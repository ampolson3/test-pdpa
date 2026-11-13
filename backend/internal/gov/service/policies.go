// DPX-02: gov.policies/gov.policy_attestations were already fully specified in the baseline migrations
// (00017_gov.sql) — a published PLT-16 "policy" document the DPO tracks for mandatory employee
// attestation. No queries or service code existed against either table until now.
package service

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	govstore "pdpa-platform/internal/gov/store"
	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/clientip"
	pdb "pdpa-platform/internal/pkg/db"
)

// PolicyType is the audit entity type for this feature's two entities (gov's own audit() helper always
// takes an explicit entityType, unlike some other modules' package-wide constant).
const PolicyType = "gov_policy"

// Policy is one published document the organization requires employees to attest to.
type Policy struct {
	ID                  uuid.UUID
	Title               string
	DocumentID          uuid.UUID
	VersionNo           int32
	RequiresAttestation bool
	PublishedAt         *time.Time
	RowVersion          int32
}

// Attestation is one employee's own acknowledgement of a policy's current version.
type Attestation struct {
	ID         uuid.UUID
	PolicyID   uuid.UUID
	VersionNo  int32
	UserID     uuid.UUID
	UserName   string
	AttestedAt time.Time
}

// MissingAttestation is DPX-02's own acceptance criterion row: an active user who has not yet attested
// the policy's current version.
type MissingAttestation struct {
	UserID   uuid.UUID
	UserName string
}

func toPolicy(r govstore.GovPolicy) Policy {
	p := Policy{ID: r.ID, Title: r.Title, DocumentID: r.DocumentID, VersionNo: r.VersionNo,
		RequiresAttestation: r.RequiresAttestation, RowVersion: r.RowVersion}
	if r.PublishedAt.Valid {
		t := r.PublishedAt.Time
		p.PublishedAt = &t
	}
	return p
}

func toAttestation(r govstore.GovPolicyAttestation) Attestation {
	return Attestation{ID: r.ID, PolicyID: r.PolicyID, VersionNo: r.PolicyVersionNo, UserID: r.UserID, AttestedAt: r.AttestedAt.Time}
}

// PublishPolicy is "เผยแพร่นโยบาย": the caller names an already-published PLT-16 document (any type —
// most naturally the generic "policy" doc type, but this feature doesn't restrict it) and this feature
// records that document's current published version as the policy employees must attest to. Publishing
// the same document version again is a harmless no-op (its own real unique constraint, documented in
// uq_policies_document_id_version_no) — only a publish of a *new* version opens a fresh tracked round.
func (s *Service) PublishPolicy(ctx context.Context, documentID uuid.UUID, title string, requiresAttestation bool) (Policy, error) {
	if !has(ctx, PolicyPermPublish) {
		return Policy{}, ErrForbidden
	}
	doc, err := s.Docs.Get(ctx, documentID)
	if err != nil {
		return Policy{}, fmt.Errorf("%w: document_id", ErrInvalid)
	}
	if doc.Status != "published" || doc.CurrentVersionID == nil {
		return Policy{}, fmt.Errorf("%w: document is not published", ErrInvalid)
	}
	pv, _, err := s.Docs.PublishedVersionOf(ctx, *doc.CurrentVersionID)
	if err != nil {
		return Policy{}, err
	}
	if title == "" {
		title = doc.Title
	}
	row, err := govstore.New(pdb.MustTxFromContext(ctx)).InsertPolicy(ctx, govstore.InsertPolicyParams{
		Title: title, DocumentID: documentID, VersionNo: pv.VersionNo, RequiresAttestation: requiresAttestation, Actor: pgUUIDPtr(currentUser(ctx)),
	})
	if err != nil {
		return Policy{}, err
	}
	out := toPolicy(row)
	return out, s.audit(ctx, "gov.policy.publish", PolicyType, out.ID, nil, map[string]any{"document_id": documentID, "version_no": out.VersionNo})
}

// getPolicy is the unexported, no-permission-check read — used internally by AttestPolicy (an EMP
// holds only PolicyPermAttest, never PolicyPermRead) and MissingAttestations, both of which already
// gate themselves on a different permission before calling this.
func (s *Service) getPolicy(ctx context.Context, id uuid.UUID) (Policy, error) {
	r, err := govstore.New(pdb.MustTxFromContext(ctx)).GetPolicy(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Policy{}, ErrNotFound
	}
	if err != nil {
		return Policy{}, err
	}
	return toPolicy(r), nil
}

// GetPolicy reads one tracked policy by id — RLS already scopes it to this tenant.
func (s *Service) GetPolicy(ctx context.Context, id uuid.UUID) (Policy, error) {
	if !has(ctx, PolicyPermRead) {
		return Policy{}, ErrForbidden
	}
	return s.getPolicy(ctx, id)
}

// ListPolicies returns every tracked policy, newest published first.
func (s *Service) ListPolicies(ctx context.Context) ([]Policy, error) {
	if !has(ctx, PolicyPermRead) {
		return nil, ErrForbidden
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListPolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Policy, 0, len(rows))
	for _, r := range rows {
		out = append(out, toPolicy(r))
	}
	return out, nil
}

// AttestPolicy is "พนักงานกดรับทราบ + หลักฐาน": any employee records their own attestation of the
// policy's current version, with the request's own client IP as evidence (clientip.From(ctx), already
// resolved by the request's own middleware chain the same way the audit log already uses it). A repeat
// attestation of the same already-attested version is a harmless no-op, not a duplicate row.
func (s *Service) AttestPolicy(ctx context.Context, policyID uuid.UUID) (Attestation, error) {
	if !has(ctx, PolicyPermAttest) {
		return Attestation{}, ErrForbidden
	}
	policy, err := s.getPolicy(ctx, policyID)
	if err != nil {
		return Attestation{}, err
	}
	me := currentUser(ctx)
	if me == nil {
		return Attestation{}, fmt.Errorf("%w: no caller in context", ErrInvalid)
	}
	q := govstore.New(pdb.MustTxFromContext(ctx))
	existing, err := q.GetPolicyAttestationForUser(ctx, govstore.GetPolicyAttestationForUserParams{PolicyID: policyID, UserID: *me})
	if err == nil && existing.PolicyVersionNo == policy.VersionNo {
		return toAttestation(existing), nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Attestation{}, err
	}
	var ipPtr *netip.Addr
	if ip, ok := clientip.From(ctx); ok {
		ipPtr = &ip
	}
	row, err := q.InsertPolicyAttestation(ctx, govstore.InsertPolicyAttestationParams{
		PolicyID: policyID, PolicyVersionNo: policy.VersionNo, UserID: *me, Ip: ipPtr,
	})
	if err != nil {
		return Attestation{}, err
	}
	out := toAttestation(row)
	return out, s.audit(ctx, "gov.policy.attest", PolicyType, policyID, nil, map[string]any{"version_no": out.VersionNo, "user_id": *me})
}

// ListAttestations is the admin-facing read side, newest first.
func (s *Service) ListAttestations(ctx context.Context, policyID uuid.UUID) ([]Attestation, error) {
	if !has(ctx, PolicyPermRead) {
		return nil, ErrForbidden
	}
	rows, err := govstore.New(pdb.MustTxFromContext(ctx)).ListPolicyAttestations(ctx, policyID)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	out := make([]Attestation, 0, len(rows))
	for _, r := range rows {
		a := toAttestation(r)
		out = append(out, a)
		ids = append(ids, a.UserID)
	}
	names, err := iamservice.AllNames(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].UserName = names[out[i].UserID]
	}
	return out, nil
}

// MissingAttestations is DPX-02's own acceptance criterion: every active user who has not yet attested
// the policy's current version — the whole tenant roster (iamservice.AllActiveUsers) minus whoever has
// already attested that exact version_no (a stale attestation of a superseded version doesn't count).
func (s *Service) MissingAttestations(ctx context.Context, policyID uuid.UUID) ([]MissingAttestation, error) {
	if !has(ctx, PolicyPermRead) {
		return nil, ErrForbidden
	}
	policy, err := s.getPolicy(ctx, policyID)
	if err != nil {
		return nil, err
	}
	attested, err := govstore.New(pdb.MustTxFromContext(ctx)).ListPolicyAttestations(ctx, policyID)
	if err != nil {
		return nil, err
	}
	done := map[uuid.UUID]bool{}
	for _, a := range attested {
		if a.PolicyVersionNo == policy.VersionNo {
			done[a.UserID] = true
		}
	}
	all, err := iamservice.AllActiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MissingAttestation, 0)
	for _, u := range all {
		if !done[u.ID] {
			out = append(out, MissingAttestation{UserID: u.ID, UserName: u.DisplayName})
		}
	}
	return out, nil
}
