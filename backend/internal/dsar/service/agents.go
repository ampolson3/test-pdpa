package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// DSAR-04 (ม.20, ผู้รับมอบอำนาจ/ผู้ใช้อำนาจปกครอง/ผู้อนุบาล/ผู้พิทักษ์): a request submitted on the data
// subject's behalf — DSAR-02's own `on_behalf` flag — needs the agent's own identity and the document that
// proves their authority to act (a power of attorney, or a parent/guardian/curator document) before staff
// can actually start working the request. dsar.agents was already fully specified in the baseline
// migrations (authority_file_id NOT NULL — a row can never exist without a document attached), so this is
// the service and the Transition guard, no new migration.

const agentNameContext = "dsar.agents.agent_name_enc"

var agentAuthorityTypes = []string{"power_of_attorney", "parent", "guardian", "curator"}

// ErrAgentRequired is DSAR-04's own acceptance criterion: an on_behalf request can't leave verifying for
// in_review without an authority-proof document already on record.
var ErrAgentRequired = errors.New("dsar: an authority-proof document is required for a request made on behalf of the data subject")

// Agent is one dsar.agents row — the person who submitted the request on the data subject's own behalf.
type Agent struct {
	ID              uuid.UUID
	RequestID       uuid.UUID
	AuthorityType   string // power_of_attorney | parent | guardian | curator
	AuthorityFileID uuid.UUID
	VerifiedAt      *time.Time
	CreatedAt       time.Time
	RowVersion      int32
}

func toAgent(r dsarstore.GetAgentRow) Agent {
	a := Agent{ID: r.ID, RequestID: r.RequestID, AuthorityType: r.AuthorityType, AuthorityFileID: r.AuthorityFileID,
		CreatedAt: r.CreatedAt.Time, RowVersion: r.RowVersion}
	if r.VerifiedAt.Valid {
		t := r.VerifiedAt.Time
		a.VerifiedAt = &t
	}
	return a
}

type RecordAgentInput struct {
	AgentName       string
	AuthorityType   string
	AuthorityFileID uuid.UUID
}

// RecordAgent attaches the agent's own clean, still-unattached PLT-09 upload of their authority document to
// the request (the same "caller's own clean upload" check DSAR-06's own id_document path uses) and encrypts
// the agent's name (PLT-13, rule 3 — never logged, write-only here like the requester's own name).
func (s *Service) RecordAgent(ctx context.Context, requestID uuid.UUID, in RecordAgentInput) (Agent, error) {
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return Agent{}, err
	}
	in.AgentName = strings.TrimSpace(in.AgentName)
	if in.AgentName == "" {
		return Agent{}, fmt.Errorf("%w: agent_name", ErrInvalid)
	}
	if !slices.Contains(agentAuthorityTypes, in.AuthorityType) {
		return Agent{}, fmt.Errorf("%w: authority_type", ErrInvalid)
	}
	f, err := s.Files.Get(ctx, in.AuthorityFileID)
	if err != nil || f.EntityType != "" {
		return Agent{}, fmt.Errorf("%w: authority_file_id", ErrInvalid)
	}
	if err := s.Files.Attach(ctx, in.AuthorityFileID, "dsar_request", requestID); err != nil {
		return Agent{}, err
	}
	nameEnc, err := s.Keyring.Encrypt(ctx, cryptoClass, agentNameContext, []byte(in.AgentName))
	if err != nil {
		return Agent{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Agent{}, err
	}
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.InsertAgent(ctx, dsarstore.InsertAgentParams{
		ID: id, RequestID: requestID, AgentNameEnc: nameEnc, AuthorityType: in.AuthorityType, AuthorityFileID: in.AuthorityFileID,
	})
	if err != nil {
		return Agent{}, err
	}
	out := toAgent(dsarstore.GetAgentRow(row))
	return out, s.audit(ctx, "dsar.agent.record", requestID, nil, map[string]any{"authority_type": out.AuthorityType, "authority_file_id": out.AuthorityFileID})
}

func (s *Service) ListAgents(ctx context.Context, requestID uuid.UUID) ([]Agent, error) {
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListAgentsForRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAgent(dsarstore.GetAgentRow(r)))
	}
	return out, nil
}

// VerifyAgent is staff's own pass on the authority document — there is no trusted registry to check a power
// of attorney or a guardianship document against, the same human-judgement call DSAR-06's own
// ConfirmIDDocument makes for the data subject's own identity ("ยืนยันทั้งผู้ยื่นและเจ้าของข้อมูล").
func (s *Service) VerifyAgent(ctx context.Context, requestID, agentID uuid.UUID) (Agent, error) {
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	cur, err := q.GetAgent(ctx, agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	if err != nil {
		return Agent{}, err
	}
	if cur.RequestID != requestID {
		return Agent{}, ErrNotFound
	}
	row, err := q.VerifyAgent(ctx, agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already verified — redelivery/double-click, surfaced as the current row (DecideVerification's own
		// no-op pattern).
		return toAgent(cur), nil
	}
	if err != nil {
		return Agent{}, err
	}
	out := toAgent(dsarstore.GetAgentRow(row))
	return out, s.audit(ctx, "dsar.agent.verify", requestID, map[string]any{"verified_at": nil}, map[string]any{"verified_at": out.VerifiedAt})
}

// hasAgentDocument is DSAR-04's own acceptance criterion check: an on_behalf request needs at least one
// dsar.agents row (whose authority_file_id is NOT NULL by schema) before it may leave verifying.
func (s *Service) hasAgentDocument(ctx context.Context, requestID uuid.UUID) (bool, error) {
	n, err := dsarstore.New(pdb.MustTxFromContext(ctx)).CountAgentsForRequest(ctx, requestID)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
