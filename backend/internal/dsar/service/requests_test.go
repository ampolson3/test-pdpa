package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dsarservice "pdpa-platform/internal/dsar/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/events"
	"pdpa-platform/internal/platform/jobs"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

type env struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *dsarservice.Service
	org    *orgservice.Service
	ropa   *ropaservice.Service
	docs   *docs.Service
}

var dsarPermissions = []string{"dsar.request.read", "dsar.request.create", "dsar.request.execute", "dsar.request.update",
	"dsar.request.approve", "org.structure.read", "org.structure.update", "ropa.activity.read", "ropa.activity.create", "ropa.activity.update"}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM dsar.requests`, `DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM ropa.processing_activities`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	orgSvc := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc}
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, nil, audit.New(), nil)
	docsSvc.RegisterVersioning()
	keyring := &crypto.Keyring{KEK: crypto.NewLocalKEK()}
	riverClient, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	svc := &dsarservice.Service{Audit: audit.New(), Org: orgSvc, Docs: docsSvc, Ropa: ropaSvc, Keyring: keyring, Events: &events.Publisher{River: riverClient}}
	return env{app: app, tenant: tenant, svc: svc, org: orgSvc, ropa: ropaSvc, docs: docsSvc}
}

func (e env) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	e.inAs(t, dsarPermissions, fn)
}

func (e env) inAs(t *testing.T, permissions []string, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: permissions}))
	}); err != nil {
		t.Fatal(err)
	}
}

func plainText(n render.Node, sb *strings.Builder) {
	if n.Type == "text" {
		sb.WriteString(n.Text)
	}
	for _, c := range n.Content {
		plainText(c, sb)
	}
}

func seedLegalEntityAndType(t *testing.T, ctx context.Context, e env) (uuid.UUID, uuid.UUID) {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	types, err := e.svc.ListRequestTypes(ctx)
	if err != nil || len(types) == 0 {
		t.Fatal(err, "expected seeded request types (migration 00043)")
	}
	return le.ID, types[0].ID
}

// TestCreateRequest_ValidatesAndNumbers: a valid request is received with a real DSAR-<year>-NNNN number and
// its SLA due date set from the type's own sla_days.
func TestCreateRequest_ValidatesAndNumbers(t *testing.T) {
	e := setup(t, "dsarreq")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})

	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "สมชาย ใจดี", RequesterContact: "somchai@example.com", ContactKind: crypto.KindEmail})
		return err
	})
	if r.Status != "received" {
		t.Fatalf("status = %q, want received", r.Status)
	}
	if !strings.HasPrefix(r.RequestNo, "DSAR-") {
		t.Fatalf("request_no = %q", r.RequestNo)
	}
	if !r.DueAt.After(r.ReceivedAt) {
		t.Fatal("expected due_at after received_at")
	}

	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "bogus", RequesterName: "x", RequesterContact: "y@example.com", ContactKind: crypto.KindEmail})
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("bad channel: err = %v, want ErrInvalid", err)
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		_, err := e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: uuid.New(), LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "y@example.com", ContactKind: crypto.KindEmail})
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("unknown request type: err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestListRequests_SearchesByRequestNoOrEmail is DSAR-17's acceptance criterion: a past request can be found
// again by its request number (a substring) or by the requester's e-mail (exact blind-index match, never
// decrypted to search — rule 3), and a search for one requester's e-mail never returns another's request.
func TestListRequests_SearchesByRequestNoOrEmail(t *testing.T) {
	e := setup(t, "dsarsearch")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var r1, r2 dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		r1, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "หนึ่ง", RequesterContact: "one@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		r2, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "สอง", RequesterContact: "two@example.com", ContactKind: crypto.KindEmail})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListRequests(ctx, dsarservice.RequestFilter{Search: r1.RequestNo[len(r1.RequestNo)-6:]})
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != r1.ID {
			t.Fatalf("search by request_no fragment: got %d results, want r1", len(list))
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListRequests(ctx, dsarservice.RequestFilter{Search: "ONE@example.com"})
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != r1.ID {
			t.Fatalf("search by email (case-insensitive): got %d results, want r1", len(list))
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListRequests(ctx, dsarservice.RequestFilter{Search: "two@example.com"})
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != r2.ID {
			t.Fatalf("search by email must not return the other requester's request: got %d results", len(list))
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListRequests(ctx, dsarservice.RequestFilter{Search: "nobody@example.com"})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Fatalf("search by unknown email: got %d results, want 0", len(list))
		}
		return nil
	})
}

// TestTransition_ResultLetterGeneratedWithRequesterNameAndType is DSAR-13's acceptance criterion: completing
// a request immediately produces a response-letter draft whose text carries the request's own type name and
// the (decrypted) requester's name — both languages.
func TestTransition_ResultLetterGeneratedWithRequesterNameAndType(t *testing.T) {
	e := setup(t, "dsarletter")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var r dsarservice.Request
	var rt dsarservice.RequestType
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "สมชาย ใจดี", RequesterContact: "somchai@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		rt, err = e.svc.GetRequestType(ctx, typeID)
		return err
	})

	var docID *uuid.UUID
	e.in(t, func(ctx context.Context) error {
		var err error
		outcome := "fulfilled"
		_, docID, err = e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "verifying"})
		if err != nil {
			return err
		}
		_, docID, err = e.svc.Transition(ctx, r.ID, r.RowVersion+1, dsarservice.TransitionInput{To: "in_review"})
		if err != nil {
			return err
		}
		_, docID, err = e.svc.Transition(ctx, r.ID, r.RowVersion+2, dsarservice.TransitionInput{To: "in_progress"})
		if err != nil {
			return err
		}
		_, docID, err = e.svc.Transition(ctx, r.ID, r.RowVersion+3, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		return err
	})
	if docID == nil {
		t.Fatal("expected a generated response-letter document id")
	}

	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, *docID)
		if err != nil {
			return err
		}
		if doc.Draft == nil {
			t.Fatal("expected a draft")
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		text := sb.String()
		if !strings.Contains(text, "สมชาย ใจดี") {
			t.Error("expected the decrypted requester name in the letter")
		}
		if !strings.Contains(text, rt.NameTh) {
			t.Error("expected the request type's own name in the letter")
		}
		if !strings.Contains(text, r.RequestNo) {
			t.Error("expected the request number in the letter")
		}
		var sbEn strings.Builder
		plainText(doc.Draft.Content["en"], &sbEn)
		if !strings.Contains(sbEn.String(), rt.NameEn) {
			t.Error("expected the English request-type name in the English letter")
		}
		return nil
	})
}

// TestTransition_RejectedRequiresReasonApproverAndGeneratesLetter is DSAR-11's acceptance criterion: a
// rejection needs both a reason and an approver (dsar.request.approve, not just dsar.request.execute) —
// with those it generates the rejection letter and publishes dsar.rejected (for ROPA-10 to log against any
// referenced processing activities, which this test also links).
func TestTransition_RejectedRequiresReasonApproverAndGeneratesLetter(t *testing.T) {
	e := setup(t, "dsarreject")
	var leID, typeID uuid.UUID
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: leID, Code: "HR", NameTh: "HR", UnitType: "department"})
		if err != nil {
			return err
		}
		a, err := e.ropa.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: leID, OrgUnitID: unit.ID, Code: "HR-REJ", Name: "กิจกรรมทดสอบ", Role: "controller"}, 0)
		activityID = a.ID
		return err
	})
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "email", RequesterName: "วิไล สุขใจ", RequesterContact: "wilai@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		if _, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "verifying"}); err != nil {
			return err
		}
		_, _, err = e.svc.Transition(ctx, r.ID, r.RowVersion+1, dsarservice.TransitionInput{To: "in_review"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		_, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion+2, dsarservice.TransitionInput{To: "rejected"})
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("no reason: err = %v, want ErrInvalid", err)
		}
		return nil
	})
	noApprove := []string{"dsar.request.read", "dsar.request.create", "dsar.request.execute", "dsar.request.update"}
	e.inAs(t, noApprove, func(ctx context.Context) error {
		reason := "ไม่พบข้อมูลของท่านในระบบ"
		_, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion+2, dsarservice.TransitionInput{To: "rejected", RejectionReasonCode: &reason})
		if !errors.Is(err, dsarservice.ErrForbidden) {
			t.Errorf("no approve permission: err = %v, want ErrForbidden", err)
		}
		return nil
	})
	var docID *uuid.UUID
	e.in(t, func(ctx context.Context) error {
		reason := "ไม่พบข้อมูลของท่านในระบบ"
		var err error
		_, docID, err = e.svc.Transition(ctx, r.ID, r.RowVersion+2, dsarservice.TransitionInput{To: "rejected", RejectionReasonCode: &reason, ActivityIDs: []uuid.UUID{activityID}})
		return err
	})
	if docID == nil {
		t.Fatal("expected a rejection letter document")
	}
	e.in(t, func(ctx context.Context) error {
		doc, err := e.docs.Get(ctx, *docID)
		if err != nil {
			return err
		}
		var sb strings.Builder
		plainText(doc.Draft.Content["th"], &sb)
		if !strings.Contains(sb.String(), "ไม่พบข้อมูลของท่านในระบบ") {
			t.Error("expected the rejection reason in the letter")
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		var n int
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx,
			`SELECT count(*)::int FROM platform.outbox_events WHERE event_type = 'dsar.rejected' AND aggregate_id = $1
			 AND payload->'data'->>'reason_code' = 'ไม่พบข้อมูลของท่านในระบบ'
			 AND payload->'data'->'activity_refs' @> to_jsonb($2::text)`, r.ID, activityID.String()).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("dsar.rejected outbox rows for the activity = %d, want 1", n)
		}
		return nil
	})
}

// TestTransition_RejectedUnknownActivityRefused: an activity_id from another tenant (or that doesn't exist)
// is refused rather than silently published in the event.
func TestTransition_RejectedUnknownActivityRefused(t *testing.T) {
	e := setup(t, "dsarrejectbadact")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "email", RequesterName: "x", RequesterContact: "z@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		if _, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "verifying"}); err != nil {
			return err
		}
		_, _, err = e.svc.Transition(ctx, r.ID, r.RowVersion+1, dsarservice.TransitionInput{To: "in_review"})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		reason := "เหตุผล"
		_, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion+2, dsarservice.TransitionInput{To: "rejected", RejectionReasonCode: &reason, ActivityIDs: []uuid.UUID{uuid.New()}})
		if !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("err = %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestTransition_InvalidEdgeRefused proves ST-02's real graph is enforced: received can't jump straight to
// completed.
func TestTransition_InvalidEdgeRefused(t *testing.T) {
	e := setup(t, "dsaredge")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})
	var r dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		r, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "z@example.com", ContactKind: crypto.KindEmail})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		outcome := "fulfilled"
		_, _, err := e.svc.Transition(ctx, r.ID, r.RowVersion, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		if !errors.Is(err, dsarservice.ErrInvalidTransition) {
			t.Errorf("err = %v, want ErrInvalidTransition", err)
		}
		return nil
	})
}

// TestRequests_TenantIsolation: tenant B's transaction cannot see tenant A's request.
func TestRequests_TenantIsolation(t *testing.T) {
	a := setup(t, "dsariso-a")
	b := setup(t, "dsariso-b")
	var leID, typeID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, a)
		return nil
	})
	var r dsarservice.Request
	a.in(t, func(ctx context.Context) error {
		var err error
		r, err = a.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "x", RequesterContact: "iso@example.com", ContactKind: crypto.KindEmail})
		return err
	})
	b.in(t, func(ctx context.Context) error {
		_, err := b.svc.GetRequest(ctx, r.ID)
		if !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("cross-tenant get: err = %v, want ErrNotFound", err)
		}
		return nil
	})
}

