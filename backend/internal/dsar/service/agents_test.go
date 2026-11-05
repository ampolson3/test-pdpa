package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	pdb "pdpa-platform/internal/pkg/db"
)

// attachableUpload is the caller's own clean, still-unattached PLT-09 upload — the same fixture DSAR-06's and
// DSAR-08's own tests already use.
func attachableUpload(t *testing.T, ctx context.Context, ff *fakeFiles) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ff.raw[id] = []byte("authority-document")
	_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `INSERT INTO platform.files (id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256, av_status)
		VALUES ($1, current_setting('app.tenant_id')::uuid, 'test', $2, 'poa.pdf', 'application/pdf', 19, 'x', 'clean')`,
		id, id.String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRecordAgent_ValidatesAndAttachesDocument(t *testing.T) {
	e, _, ff := identityEnv(t, "dsaragentrecord")
	legalEntity := createTestLegalEntity(t, e, "บริษัท เอเจนต์ จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	var fileID uuid.UUID
	var agent dsarservice.Agent
	e.in(t, func(ctx context.Context) error {
		fileID = attachableUpload(t, ctx, ff)
		var err error
		agent, err = e.svc.RecordAgent(ctx, req.ID, dsarservice.RecordAgentInput{
			AgentName: "สมศักดิ์ ผู้รับมอบอำนาจ", AuthorityType: "power_of_attorney", AuthorityFileID: fileID,
		})
		return err
	})
	if agent.AuthorityType != "power_of_attorney" || agent.AuthorityFileID != fileID || agent.VerifiedAt != nil {
		t.Fatalf("recorded agent = %+v", agent)
	}

	// Validation: bad authority_type, empty name.
	e.in(t, func(ctx context.Context) error {
		other := attachableUpload(t, ctx, ff)
		if _, err := e.svc.RecordAgent(ctx, req.ID, dsarservice.RecordAgentInput{AgentName: "x", AuthorityType: "bogus", AuthorityFileID: other}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("bad authority_type: err = %v, want ErrInvalid", err)
		}
		if _, err := e.svc.RecordAgent(ctx, req.ID, dsarservice.RecordAgentInput{AgentName: "  ", AuthorityType: "parent", AuthorityFileID: other}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("empty agent_name: err = %v, want ErrInvalid", err)
		}
		return nil
	})

	// The agent's own document is now attached to the request — a second record can't reuse the same file id.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordAgent(ctx, req.ID, dsarservice.RecordAgentInput{AgentName: "y", AuthorityType: "parent", AuthorityFileID: fileID}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("reusing an already-attached file id: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestTransition_InReview_RequiresAgentDocumentWhenOnBehalf(t *testing.T) {
	e, _, ff := identityEnv(t, "dsaragenttransition")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ทรานซิชัน จำกัด")

	var req dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		req, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{
			RequestTypeID: requestTypeID(t, e), LegalEntityID: legalEntity, Channel: "phone", OnBehalf: true,
			RequesterName: "เจ้าของข้อมูล", RequesterContact: "onbehalf@example.com", ContactKind: "email",
		})
		return err
	})

	// First move to verifying (the normal path) — entering in_review without an agent document is refused.
	e.in(t, func(ctx context.Context) error {
		var err error
		req, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "verifying"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "in_review"}); !errors.Is(err, dsarservice.ErrAgentRequired) {
			t.Errorf("in_review without an agent document: err = %v, want ErrAgentRequired", err)
		}
		return nil
	})

	// Recording the agent's authority document clears the gate — the acceptance criterion itself.
	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		_, err := e.svc.RecordAgent(ctx, req.ID, dsarservice.RecordAgentInput{AgentName: "ผู้ปกครอง", AuthorityType: "guardian", AuthorityFileID: fileID})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		var err error
		req, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "in_review"})
		return err
	})
	if req.Status != "in_review" {
		t.Fatalf("status after recording the agent document = %q, want in_review", req.Status)
	}
}

func TestTransition_InReview_NoGateWhenNotOnBehalf(t *testing.T) {
	e := setup(t, "dsaragentnotonbehalf")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ไม่มีตัวแทน จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	e.in(t, func(ctx context.Context) error {
		var err error
		req, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "verifying"})
		if err != nil {
			return err
		}
		req, _, err = e.svc.Transition(ctx, req.ID, req.RowVersion, dsarservice.TransitionInput{To: "in_review"})
		return err
	})
	if req.Status != "in_review" {
		t.Fatalf("status for a direct (not on_behalf) request = %q, want in_review", req.Status)
	}
}

func TestVerifyAgent_ConfirmsAndIsIdempotent(t *testing.T) {
	e, _, ff := identityEnv(t, "dsaragentverify")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ยืนยัน จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	var agent dsarservice.Agent
	e.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ff)
		var err error
		agent, err = e.svc.RecordAgent(ctx, req.ID, dsarservice.RecordAgentInput{AgentName: "ผู้อนุบาล", AuthorityType: "curator", AuthorityFileID: fileID})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		var err error
		agent, err = e.svc.VerifyAgent(ctx, req.ID, agent.ID)
		return err
	})
	if agent.VerifiedAt == nil {
		t.Fatal("expected verified_at to be set")
	}
	verifiedAt := *agent.VerifiedAt

	// A redelivered/double-click verify is a harmless no-op — the current row, unchanged.
	e.in(t, func(ctx context.Context) error {
		var err error
		agent, err = e.svc.VerifyAgent(ctx, req.ID, agent.ID)
		return err
	})
	if agent.VerifiedAt == nil || !agent.VerifiedAt.Equal(verifiedAt) {
		t.Errorf("re-verifying changed verified_at: got %v, want unchanged %v", agent.VerifiedAt, verifiedAt)
	}

	// An agent id belonging to a different request is not found.
	e.in(t, func(ctx context.Context) error {
		other := createTestRequest(t, e, legalEntity, requestTypeID(t, e))
		if _, err := e.svc.VerifyAgent(ctx, other.ID, agent.ID); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("verify against the wrong request: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}

func TestAgents_TwoTenantIsolation(t *testing.T) {
	eA, _, ffA := identityEnv(t, "dsaragentisoa")
	eB, _, ffB := identityEnv(t, "dsaragentisob")
	leA := createTestLegalEntity(t, eA, "บริษัท เอ จำกัด")
	leB := createTestLegalEntity(t, eB, "บริษัท บี จำกัด")
	reqA := createTestRequest(t, eA, leA, requestTypeID(t, eA))
	reqB := createTestRequest(t, eB, leB, requestTypeID(t, eB))

	var agentB dsarservice.Agent
	eB.in(t, func(ctx context.Context) error {
		fileID := attachableUpload(t, ctx, ffB)
		var err error
		agentB, err = eB.svc.RecordAgent(ctx, reqB.ID, dsarservice.RecordAgentInput{AgentName: "B's agent", AuthorityType: "parent", AuthorityFileID: fileID})
		return err
	})

	eA.in(t, func(ctx context.Context) error {
		agents, err := eA.svc.ListAgents(ctx, reqA.ID)
		if err != nil {
			return err
		}
		for _, a := range agents {
			if a.ID == agentB.ID {
				t.Error("tenant A's agent list includes tenant B's agent")
			}
		}
		if _, err := eA.svc.VerifyAgent(ctx, reqA.ID, agentB.ID); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("tenant A verifying tenant B's agent: err = %v, want ErrNotFound", err)
		}
		return nil
	})
	_ = ffA
}

func requestTypeID(t *testing.T, e env) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	e.in(t, func(ctx context.Context) error {
		types, err := e.svc.ListRequestTypes(ctx)
		if err != nil {
			return err
		}
		id = types[0].ID
		return nil
	})
	return id
}
