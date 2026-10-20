package service_test

import (
	"context"
	"errors"
	"fmt"
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
	"dsar.request.approve", "org.structure.read", "org.structure.update", "ropa.activity.read", "ropa.activity.create", "ropa.activity.update",
	"dsar.subtask.read", "dsar.subtask.create", "dsar.subtask.update", "dsar.subtask.execute", "dsar.subtask.delete",
	"dsar.package.read", "dsar.package.create"}

func setup(t *testing.T, suffix string) env {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM dsar.verifications`, `DELETE FROM iam.subject_verifications`, `DELETE FROM dsar.subtasks`,
				`DELETE FROM platform.notifications`, `DELETE FROM river_job WHERE kind = 'notify.deliver'`,
				`DELETE FROM iam.group_members`, `DELETE FROM iam.groups`, `DELETE FROM platform.files`,
				`DELETE FROM platform.public_keys`, `DELETE FROM dsar.intake_forms`, `DELETE FROM dsar.agents`,
				`DELETE FROM dsar.requests`, `DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM ropa.processing_activities`, `DELETE FROM org.org_units`,
				`UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`, `DELETE FROM platform.audit_log`,
				`DELETE FROM iam.users WHERE email LIKE 'assignee-%@dbtest.example'`,
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
	svc := &dsarservice.Service{Audit: audit.New(), Org: orgSvc, Docs: docsSvc, Ropa: ropaSvc, Keyring: keyring,
		Events: &events.Publisher{River: riverClient}, River: riverClient}
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

// TestCreateRequest_EveryChannelLandsInTheSameQueueWithOnBehalf is DSAR-02's acceptance criterion directly:
// a request intaken by staff on any channel (phone, branch, letter, email, LINE — not just the self-service
// "web" default) lands in the same dsar.requests queue, visible through the same ListRequests, and records
// both its origin channel and whether staff recorded it on the data subject's own behalf.
func TestCreateRequest_EveryChannelLandsInTheSameQueueWithOnBehalf(t *testing.T) {
	e := setup(t, "dsarchannel")
	var leID, typeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, typeID = seedLegalEntityAndType(t, ctx, e)
		return nil
	})

	channels := []string{"web", "email", "phone", "branch", "letter", "line", "api"}
	created := map[string]dsarservice.Request{}
	e.in(t, func(ctx context.Context) error {
		for i, ch := range channels {
			r, err := e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID, LegalEntityID: leID,
				Channel: ch, OnBehalf: ch != "web", RequesterName: "x", RequesterContact: fmt.Sprintf("chan%d@example.com", i), ContactKind: crypto.KindEmail})
			if err != nil {
				return fmt.Errorf("channel %s: %w", ch, err)
			}
			created[ch] = r
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListRequests(ctx, dsarservice.RequestFilter{})
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, r := range list {
			if want, ok := created[r.Channel]; ok && r.ID == want.ID {
				seen[r.Channel] = true
				if r.Channel == "web" && r.OnBehalf {
					t.Errorf("web request unexpectedly recorded as on_behalf")
				}
				if r.Channel != "web" && !r.OnBehalf {
					t.Errorf("channel %s: expected on_behalf, got false", r.Channel)
				}
			}
		}
		for _, ch := range channels {
			if !seen[ch] {
				t.Errorf("channel %s: its request was not found in the same ListRequests queue", ch)
			}
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

// TestListRequests_FiltersByRequestType is DSAR-05's acceptance criterion: a complaint (or inquiry) is just
// another right type on the same ST-02 workflow every request shares, filterable to its own queue view by
// request_type_id, and tracked through to completed exactly like any other request type.
func TestListRequests_FiltersByRequestType(t *testing.T) {
	e := setup(t, "dsarcomplaint")
	var leID, complaintTypeID, accessTypeID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		leID, _ = seedLegalEntityAndType(t, ctx, e)
		types, err := e.svc.ListRequestTypes(ctx)
		if err != nil {
			return err
		}
		for _, rt := range types {
			switch rt.Code {
			case "complaint":
				complaintTypeID = rt.ID
			case "access":
				accessTypeID = rt.ID
			}
		}
		if complaintTypeID == uuid.Nil || accessTypeID == uuid.Nil {
			t.Fatal("expected both complaint and access request types to be seeded (migration 00043)")
		}
		return nil
	})

	var complaint, access dsarservice.Request
	e.in(t, func(ctx context.Context) error {
		var err error
		complaint, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: complaintTypeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "ร้องเรียน หนึ่ง", RequesterContact: "complainant@example.com", ContactKind: crypto.KindEmail})
		if err != nil {
			return err
		}
		access, err = e.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: accessTypeID, LegalEntityID: leID,
			Channel: "web", RequesterName: "ขอสำเนา สอง", RequesterContact: "accessor@example.com", ContactKind: crypto.KindEmail})
		return err
	})

	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListRequests(ctx, dsarservice.RequestFilter{RequestTypeID: &complaintTypeID})
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != complaint.ID {
			t.Fatalf("filter by complaint request_type_id: got %d results, want exactly the complaint", len(list))
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListRequests(ctx, dsarservice.RequestFilter{RequestTypeID: &accessTypeID})
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != access.ID {
			t.Fatalf("filter by access request_type_id: got %d results, want exactly the access request", len(list))
		}
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		list, _, err := e.svc.ListRequests(ctx, dsarservice.RequestFilter{})
		if err != nil {
			return err
		}
		if len(list) != 2 {
			t.Fatalf("no filter: got %d results, want both requests in the same queue", len(list))
		}
		return nil
	})

	// The complaint walks the exact same ST-02 transitions every other right type already does — "tracked
	// through to close just like a [regular] request" is this generic machine, not special-cased logic.
	e.in(t, func(ctx context.Context) error {
		outcome := "fulfilled"
		var err error
		_, _, err = e.svc.Transition(ctx, complaint.ID, complaint.RowVersion, dsarservice.TransitionInput{To: "verifying"})
		if err != nil {
			return err
		}
		_, _, err = e.svc.Transition(ctx, complaint.ID, complaint.RowVersion+1, dsarservice.TransitionInput{To: "in_review"})
		if err != nil {
			return err
		}
		_, _, err = e.svc.Transition(ctx, complaint.ID, complaint.RowVersion+2, dsarservice.TransitionInput{To: "in_progress"})
		if err != nil {
			return err
		}
		_, _, err = e.svc.Transition(ctx, complaint.ID, complaint.RowVersion+3, dsarservice.TransitionInput{To: "completed", Outcome: &outcome})
		return err
	})
	e.in(t, func(ctx context.Context) error {
		got, err := e.svc.GetRequest(ctx, complaint.ID)
		if err != nil {
			return err
		}
		if got.Status != "completed" || got.ClosedAt == nil {
			t.Fatalf("expected the complaint to be closed, got status=%q closed_at=%v", got.Status, got.ClosedAt)
		}
		return nil
	})
}

// TestListRequests_FiltersByRequestType_TwoTenantIsolation proves the filter never leaks another tenant's
// requests of the same type.
func TestListRequests_FiltersByRequestType_TwoTenantIsolation(t *testing.T) {
	e1 := setup(t, "dsarcompa")
	e2 := setup(t, "dsarcompb")
	var leID1, typeID1 uuid.UUID
	e1.in(t, func(ctx context.Context) error {
		leID1, typeID1 = seedLegalEntityAndType(t, ctx, e1)
		return nil
	})
	var typeID2 uuid.UUID
	e2.in(t, func(ctx context.Context) error {
		_, typeID2 = seedLegalEntityAndType(t, ctx, e2)
		return nil
	})
	e1.in(t, func(ctx context.Context) error {
		_, err := e1.svc.CreateRequest(ctx, dsarservice.CreateRequestInput{RequestTypeID: typeID1, LegalEntityID: leID1,
			Channel: "web", RequesterName: "A", RequesterContact: "a@example.com", ContactKind: crypto.KindEmail})
		return err
	})
	e2.in(t, func(ctx context.Context) error {
		list, _, err := e2.svc.ListRequests(ctx, dsarservice.RequestFilter{RequestTypeID: &typeID2})
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Fatalf("tenant B must not see tenant A's requests of the same type: got %d", len(list))
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
