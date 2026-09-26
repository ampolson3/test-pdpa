package forms_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/crypto"
	"pdpa-platform/internal/platform/forms"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	"pdpa-platform/internal/wiring"
)

type fixture struct {
	app        *pgxpool.Pool
	a, b       dbtest.Tenant
	alice, bob uuid.UUID
	svc        *forms.Service
	draft      forms.Draft
}

// designer may build and answer assessments.
var designer = []string{"assessment.template.read", "assessment.template.create", "assessment.template.update", "assessment.template.publish", "assessment.dpia.create"}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	f := &fixture{app: dbtest.Pool(t)}
	owner, platform := dbtest.OwnerPool(t), dbtest.PlatformPool(t)
	f.a = dbtest.SeedTenant(t, ctx, f.app, platform, "forms-a")
	f.b = dbtest.SeedTenant(t, ctx, f.app, platform, "forms-b")
	f.alice = f.a.UserID
	if err := pdb.WithTenantTx(ctx, f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = 'active', display_name = 'Alice' WHERE id = $1`, f.alice); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, 'bob@forms.example', 'Bob', 'active') RETURNING id`,
			f.a.ID).Scan(&f.bob)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, tenant := range []dbtest.Tenant{f.a, f.b} {
			_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
				tx := pdb.MustTxFromContext(ctx)
				for _, q := range []string{`DELETE FROM platform.form_section_assignments`, `DELETE FROM platform.form_submissions`,
					`UPDATE platform.form_definitions SET current_version_id = NULL WHERE tenant_id IS NOT NULL`,
					`DELETE FROM platform.form_versions WHERE tenant_id IS NOT NULL`, `DELETE FROM platform.form_definitions WHERE tenant_id IS NOT NULL`,
					`DELETE FROM platform.notifications`, `DELETE FROM platform.audit_log`, `DELETE FROM platform.tenant_keys`} {
					if _, err := tx.Exec(ctx, q); err != nil {
						t.Logf("cleanup %q: %v", q, err)
					}
				}
				_, err := tx.Exec(ctx, `DELETE FROM iam.users WHERE id <> $1`, tenant.UserID)
				return err
			})
			_, _ = f.app.Exec(context.Background(), `DELETE FROM river_job WHERE args->>'tenant_id' = $1`, tenant.ID.String())
		}
	})
	client, _ := jobs.NewInsertClient(f.app)
	f.svc = wiring.Forms(&notify.Service{Keyring: &crypto.Keyring{KEK: crypto.NewLocalKEK()}, River: client}, audit.New())

	raw, err := os.ReadFile("../../../../packages/form-renderer/src/fixtures/engine-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Schema  forms.Schema   `json:"schema"`
		Scoring *forms.Scoring `json:"scoring"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	f.draft = forms.Draft{Schema: fx.Schema, Scoring: fx.Scoring}
	return f
}

// as runs fn in one transaction of tenant as user with permissions.
func (f *fixture) as(t *testing.T, tenant dbtest.Tenant, user uuid.UUID, perms []string, fn func(ctx context.Context) error) {
	t.Helper()
	err := pdb.WithTenantTx(context.Background(), f.app, tenant.ID.String(), user.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: user.String(), Permissions: perms}))
	})
	if err != nil {
		t.Fatal(err)
	}
}

// published builds and publishes the fixture assessment as Alice.
func (f *fixture) published(t *testing.T) forms.Form {
	t.Helper()
	var form forms.Form
	f.as(t, f.a, f.alice, designer, func(ctx context.Context) error {
		var err error
		if form, err = f.svc.CreateForm(ctx, "dpia_screening", "คัดกรอง DPIA", "assessment", f.draft); err != nil {
			return err
		}
		form, err = f.svc.Publish(ctx, form.ID, form.Versions[0].RowVersion)
		return err
	})
	return form
}

// Acceptance (PLT-06): an assessment with show/hide conditions and scoring is built from data alone, and
// responses are validated, filtered and scored on the server.
func TestAssessment_ConditionsAndScoring(t *testing.T) {
	f := setup(t)
	form := f.published(t)
	if form.Status != "published" || len(form.Versions) != 1 || form.Versions[0].PublishedAt == nil || form.CurrentVersionID == nil {
		t.Fatalf("published form: %+v", form)
	}
	f.as(t, f.a, f.alice, designer, func(ctx context.Context) error {
		resp, err := f.svc.StartResponse(ctx, form.ID, "", nil)
		if err != nil {
			return err
		}
		if resp.Status != "draft" || !resp.IsOwner || len(resp.CanAnswer) != 3 {
			t.Errorf("new response: %+v", resp)
		}
		// Drafts may be incomplete but not wrong.
		_, err = f.svc.SaveAnswers(ctx, resp.ID, resp.RowVersion, forms.Answers{"subjects": -5})
		var ae *forms.AnswerErrors
		if !errors.As(err, &ae) || len(ae.Fields) != 1 || ae.Fields[0] != (forms.FieldError{Question: "subjects", Code: "out_of_range"}) {
			t.Errorf("bad answer: %v", err)
		}
		if _, err := f.svc.SaveAnswers(ctx, resp.ID, resp.RowVersion, forms.Answers{"nope": 1}); !errors.As(err, &ae) {
			t.Errorf("unknown question: %v", err)
		}
		resp, err = f.svc.SaveAnswers(ctx, resp.ID, resp.RowVersion, forms.Answers{"org_name": "ACME", "processes_sensitive": "yes", "sensitive_types": []any{"health", "biometric"}, "subjects": 20000})
		if err != nil {
			return err
		}
		if !slices.Contains(resp.Result.Visible, "large_scale_reason") {
			t.Errorf("conditional section should show: %v", resp.Result.Visible)
		}
		if _, err := f.svc.Submit(ctx, resp.ID, resp.RowVersion); !errors.As(err, &ae) || ae.Fields[0].Question != "large_scale_reason" {
			t.Errorf("submit without the now-required answer: %v", err)
		}
		if _, err := f.svc.SaveAnswers(ctx, resp.ID, resp.RowVersion-1, forms.Answers{"org_name": "x"}); !errors.Is(err, forms.ErrVersionMismatch) {
			t.Errorf("stale version: %v", err)
		}
		// Changing the answer the section depends on hides it, and its answers are dropped.
		resp, err = f.svc.SaveAnswers(ctx, resp.ID, resp.RowVersion, forms.Answers{"large_scale_reason": "many", "vendor": "offshore", "notes": "n"})
		if err != nil {
			return err
		}
		resp, err = f.svc.SaveAnswers(ctx, resp.ID, resp.RowVersion, forms.Answers{"subjects": 50})
		if err != nil {
			return err
		}
		if _, ok := resp.Answers["large_scale_reason"]; ok {
			t.Errorf("hidden answer kept: %v", resp.Answers)
		}
		resp, err = f.svc.Submit(ctx, resp.ID, resp.RowVersion)
		if err != nil {
			return err
		}
		// processes_sensitive yes 5×2 + health 3 + biometric 4 + offshore 4 = 21 → high.
		if resp.Status != "submitted" || resp.SubmittedAt == nil || resp.Result.Score != 21 || resp.Result.Band != "high" {
			t.Errorf("submitted: status %s score %v band %s", resp.Status, resp.Result.Score, resp.Result.Band)
		}
		if _, err := f.svc.SaveAnswers(ctx, resp.ID, resp.RowVersion, forms.Answers{"org_name": "x"}); !errors.Is(err, forms.ErrInvalidState) {
			t.Errorf("submitted responses are frozen: %v", err)
		}
		list, err := f.svc.ListResponses(ctx, form.ID)
		if err != nil || len(list) != 1 || list[0].Result.Band != "high" || list[0].OwnerName != "Alice" {
			t.Errorf("responses: %+v %v", list, err)
		}
		return nil
	})
}

// Published versions never change: a change is a new draft, published as a new version; responses keep
// the version they started on.
func TestVersions(t *testing.T) {
	f := setup(t)
	form := f.published(t)
	f.as(t, f.a, f.alice, designer, func(ctx context.Context) error {
		old, err := f.svc.StartResponse(ctx, form.ID, "", nil)
		if err != nil {
			return err
		}
		d := f.draft
		d.Schema.Sections = d.Schema.Sections[:1]
		if _, err := f.svc.SaveDraft(ctx, form.ID, 1, d); !errors.Is(err, forms.ErrVersionMismatch) {
			t.Errorf("no draft exists, so only version 0 creates one: %v", err)
		}
		v2, err := f.svc.SaveDraft(ctx, form.ID, 0, d)
		if err != nil || v2.No != 2 || v2.PublishedAt != nil {
			t.Fatalf("new draft: %+v %v", v2, err)
		}
		if v2, err = f.svc.SaveDraft(ctx, form.ID, v2.RowVersion, d); err != nil || v2.No != 2 {
			t.Errorf("editing the draft keeps its number: %+v %v", v2, err)
		}
		if _, err := f.svc.SaveDraft(ctx, form.ID, 0, d); !errors.Is(err, forms.ErrVersionMismatch) {
			t.Errorf("second draft: %v", err)
		}
		bad := d
		bad.Schema = forms.Schema{Sections: []forms.Section{{Key: "s", Title: forms.Text{"th": "ส"}, Questions: []forms.Question{{Key: "q", Type: "nope", Label: forms.Text{"th": "ค"}}}}}}
		if _, err := f.svc.SaveDraft(ctx, form.ID, v2.RowVersion, bad); !errors.Is(err, forms.ErrInvalidSchema) {
			t.Errorf("invalid schema: %v", err)
		}
		if form, err = f.svc.Publish(ctx, form.ID, v2.RowVersion); err != nil {
			return err
		}
		if len(form.Versions) != 2 || *form.CurrentVersionID != form.Versions[0].ID || form.Versions[0].No != 2 {
			t.Errorf("after second publish: %+v", form)
		}
		if _, err := f.svc.Publish(ctx, form.ID, 1); !errors.Is(err, forms.ErrInvalidState) {
			t.Errorf("nothing to publish: %v", err)
		}
		again, err := f.svc.GetResponse(ctx, old.ID)
		if err != nil || again.Version.No != 1 || len(again.Version.Schema.Sections) != 3 {
			t.Errorf("running response moved version: %+v %v", again.Version, err)
		}
		fresh, err := f.svc.StartResponse(ctx, form.ID, "", nil)
		if err != nil || fresh.Version.No != 2 {
			t.Errorf("new response: %+v %v", fresh.Version, err)
		}
		return nil
	})
}

// A section can be given to someone else: they answer only it, get an in-app notification, and the owner
// can submit once they are done.
func TestSectionAssignment(t *testing.T) {
	f := setup(t)
	form := f.published(t)
	var respID uuid.UUID
	f.as(t, f.a, f.alice, designer, func(ctx context.Context) error {
		resp, err := f.svc.StartResponse(ctx, form.ID, "", nil)
		if err != nil {
			return err
		}
		respID = resp.ID
		resp, err = f.svc.SaveAnswers(ctx, resp.ID, resp.RowVersion, forms.Answers{"org_name": "ACME", "processes_sensitive": "no", "subjects": 20000})
		if err != nil {
			return err
		}
		if _, err := f.svc.Assign(ctx, resp.ID, resp.RowVersion, "nope", &f.bob); !errors.Is(err, forms.ErrInvalidRequest) {
			t.Errorf("unknown section: %v", err)
		}
		if resp, err = f.svc.Assign(ctx, resp.ID, resp.RowVersion, "scale", &f.bob); err != nil {
			return err
		}
		if len(resp.Assignments) != 1 || resp.Assignments[0].AssigneeName != "Bob" || slices.Contains(resp.CanAnswer, "scale") {
			t.Errorf("assigned: %+v can %v", resp.Assignments, resp.CanAnswer)
		}
		if _, err := f.svc.SaveAnswers(ctx, resp.ID, resp.RowVersion, forms.Answers{"large_scale_reason": "x"}); !errors.Is(err, forms.ErrForbidden) {
			t.Errorf("owner answering an assigned section: %v", err)
		}
		if _, err := f.svc.Submit(ctx, resp.ID, resp.RowVersion); !errors.Is(err, forms.ErrInvalidState) {
			t.Errorf("submit before the section is done: %v", err)
		}
		return nil
	})
	var n int
	if err := pdb.WithTenantTx(context.Background(), f.app, f.a.ID.String(), "", func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT count(*) FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id
			WHERE t.code = 'form.section_assigned' AND n.recipient_user_id = $1 AND n.entity_id = $2`, f.bob, respID).Scan(&n)
	}); err != nil || n != 1 {
		t.Errorf("Bob's in-app notifications: %d %v", n, err)
	}
	// Bob has no permission on assessments at all — the assignment is his access.
	f.as(t, f.a, f.bob, nil, func(ctx context.Context) error {
		mine, err := f.svc.MyAssignments(ctx)
		if err != nil || len(mine) != 1 || mine[0].ResponseID != respID || mine[0].SectionTitle["th"] != "ขนาดการประมวลผล" {
			t.Errorf("Bob's assignments: %+v %v", mine, err)
		}
		resp, err := f.svc.GetResponse(ctx, respID)
		if err != nil {
			return err
		}
		if resp.IsOwner || !slices.Equal(resp.CanAnswer, []string{"scale"}) {
			t.Errorf("Bob sees: owner %v can %v", resp.IsOwner, resp.CanAnswer)
		}
		if _, err := f.svc.SaveAnswers(ctx, respID, resp.RowVersion, forms.Answers{"org_name": "Evil"}); !errors.Is(err, forms.ErrForbidden) {
			t.Errorf("Bob answering another section: %v", err)
		}
		if _, err := f.svc.CompleteSection(ctx, respID, "scale"); err == nil {
			t.Error("completing with a required question unanswered")
		}
		if resp, err = f.svc.SaveAnswers(ctx, respID, resp.RowVersion, forms.Answers{"large_scale_reason": "many customers"}); err != nil {
			return err
		}
		if _, err := f.svc.Submit(ctx, respID, resp.RowVersion); !errors.Is(err, forms.ErrForbidden) {
			t.Errorf("Bob submitting: %v", err)
		}
		if resp, err = f.svc.CompleteSection(ctx, respID, "scale"); err != nil {
			return err
		}
		if resp.Assignments[0].Status != "done" || len(resp.CanAnswer) != 0 {
			t.Errorf("after done: %+v %v", resp.Assignments, resp.CanAnswer)
		}
		return nil
	})
	f.as(t, f.a, f.alice, designer, func(ctx context.Context) error {
		resp, err := f.svc.GetResponse(ctx, respID)
		if err != nil {
			return err
		}
		resp, err = f.svc.Submit(ctx, respID, resp.RowVersion)
		if err != nil || resp.Status != "submitted" || resp.Answers["large_scale_reason"] != "many customers" {
			t.Errorf("submit: %+v %v", resp, err)
		}
		return nil
	})
}

// Each form type uses its module's permissions; types nobody registered don't exist.
func TestPermissionsByFormType(t *testing.T) {
	f := setup(t)
	form := f.published(t)
	f.as(t, f.a, f.bob, []string{"dsar.form.read", "dsar.form.create"}, func(ctx context.Context) error {
		if _, err := f.svc.CreateForm(ctx, "x", "x", "assessment", f.draft); !errors.Is(err, forms.ErrForbidden) {
			t.Errorf("create assessment with dsar permissions: %v", err)
		}
		if _, err := f.svc.CreateForm(ctx, "x", "x", "intake", f.draft); !errors.Is(err, forms.ErrUnknownType) {
			t.Errorf("unregistered type: %v", err)
		}
		if _, err := f.svc.GetForm(ctx, form.ID); !errors.Is(err, forms.ErrNotFound) {
			t.Errorf("reading another type's form: %v", err)
		}
		list, err := f.svc.ListForms(ctx, "")
		if err != nil || len(list) != 0 {
			t.Errorf("list: %+v %v", list, err)
		}
		if types := f.svc.Types(ctx); !slices.Equal(types, []string{"dsar"}) {
			t.Errorf("types: %v", types)
		}
		if _, err := f.svc.CreateForm(ctx, "request", "คำขอ", "dsar", f.draft); err != nil {
			return err
		}
		if _, err := f.svc.CreateForm(ctx, "request", "คำขอ", "dsar", f.draft); !errors.Is(err, forms.ErrInvalidRequest) {
			t.Errorf("duplicate code: %v", err)
		}
		return nil
	})
	// Read-only: may see the form but not change, publish or answer it.
	f.as(t, f.a, f.bob, []string{"assessment.template.read"}, func(ctx context.Context) error {
		if _, err := f.svc.GetForm(ctx, form.ID); err != nil {
			t.Errorf("read: %v", err)
		}
		if _, err := f.svc.SaveDraft(ctx, form.ID, 0, f.draft); !errors.Is(err, forms.ErrForbidden) {
			t.Errorf("update: %v", err)
		}
		if _, err := f.svc.StartResponse(ctx, form.ID, "", nil); !errors.Is(err, forms.ErrForbidden) {
			t.Errorf("respond: %v", err)
		}
		return nil
	})
	// Consent forms are only answered through their module (Record), never started in the admin app.
	consent := []string{"consent.collectionpoint.read", "consent.collectionpoint.create", "consent.collectionpoint.publish"}
	f.as(t, f.a, f.alice, consent, func(ctx context.Context) error {
		c, err := f.svc.CreateForm(ctx, "newsletter", "จดหมายข่าว", "consent", f.draft)
		if err != nil {
			return err
		}
		if c, err = f.svc.Publish(ctx, c.ID, c.Versions[0].RowVersion); err != nil {
			return err
		}
		if _, err := f.svc.StartResponse(ctx, c.ID, "", nil); !errors.Is(err, forms.ErrForbidden) {
			t.Errorf("start consent response: %v", err)
		}
		if _, _, err := f.svc.Record(ctx, *c.CurrentVersionID, forms.Answers{"org_name": "x"}, "data_subject", nil, "", nil); err == nil {
			t.Error("Record accepts an incomplete response")
		}
		id, res, err := f.svc.Record(ctx, *c.CurrentVersionID, forms.Answers{"org_name": "x", "processes_sensitive": "no", "subjects": 3, "notes": "hidden"},
			"data_subject", nil, "", nil)
		if err != nil || id == uuid.Nil || res.Band != "low" || res.Answers["notes"] != nil {
			t.Errorf("record: %v %+v %v", id, res, err)
		}
		return nil
	})
}

// Tenant B sees none of tenant A's forms or responses.
func TestTwoTenantIsolation(t *testing.T) {
	f := setup(t)
	form := f.published(t)
	var respID uuid.UUID
	f.as(t, f.a, f.alice, designer, func(ctx context.Context) error {
		r, err := f.svc.StartResponse(ctx, form.ID, "", nil)
		respID = r.ID
		return err
	})
	f.as(t, f.b, f.b.UserID, designer, func(ctx context.Context) error {
		if _, err := f.svc.GetForm(ctx, form.ID); !errors.Is(err, forms.ErrNotFound) {
			t.Errorf("form: %v", err)
		}
		if _, err := f.svc.GetResponse(ctx, respID); !errors.Is(err, forms.ErrNotFound) {
			t.Errorf("response: %v", err)
		}
		if _, err := f.svc.StartResponse(ctx, form.ID, "", nil); !errors.Is(err, forms.ErrNotFound) {
			t.Errorf("start: %v", err)
		}
		if _, err := f.svc.Publish(ctx, form.ID, 1); !errors.Is(err, forms.ErrNotFound) {
			t.Errorf("publish: %v", err)
		}
		list, err := f.svc.ListForms(ctx, "")
		if err != nil || len(list) != 0 {
			t.Errorf("list: %+v %v", list, err)
		}
		// Same code in another tenant is fine.
		_, err = f.svc.CreateForm(ctx, "dpia_screening", "คัดกรอง DPIA", "assessment", f.draft)
		return err
	})
}
