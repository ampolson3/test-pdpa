package noticepublichttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	noticeservice "pdpa-platform/internal/notice/service"
	noticepublichttp "pdpa-platform/internal/notice/publichttp"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/pkg/idempotency"
	"pdpa-platform/internal/pkg/validate"
	audit "pdpa-platform/internal/platform/audit/service"
	docsservice "pdpa-platform/internal/platform/docs"
	"pdpa-platform/internal/platform/docs/render"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/publickeys"
	"pdpa-platform/internal/platform/versioning"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"

	"github.com/redis/go-redis/v9"
)

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func heading(topic, text string) render.Node {
	attrs := map[string]any{"level": float64(2)}
	if topic != "" {
		attrs["topic"] = topic
	}
	return render.Node{Type: "heading", Attrs: attrs, Content: []render.Node{{Type: "text", Text: text}}}
}

func para(text string) render.Node {
	return render.Node{Type: "paragraph", Content: []render.Node{{Type: "text", Text: text}}}
}

func completeContent() render.Content {
	return render.Content{"th": {Type: "doc", Content: []render.Node{
		heading(noticeservice.TopicPurposeBasis, "วัตถุประสงค์"), para("จ่ายเงินเดือน (ฐาน: สัญญา)"),
		heading(noticeservice.TopicConsequence, "ผลกระทบ"), para("ท่านจะไม่ได้รับเงินเดือน"),
		heading(noticeservice.TopicData, "ข้อมูล"), para("ข้อมูลเงินเดือน"),
		heading(noticeservice.TopicRetention, "ระยะเวลา"), para("10 ปี"),
		heading(noticeservice.TopicRecipients, "ผู้รับ"), para("กรมสรรพากร"),
		heading(noticeservice.TopicContact, "ติดต่อ"), para("บริษัท ทดสอบ จำกัด อีเมล dpo@test.example"),
		heading(noticeservice.TopicRights, "สิทธิ"), para("ท่านมีสิทธิขอเข้าถึงข้อมูลของท่าน"),
	}}}
}

// Contract test of PNG-06's public notice endpoints behind the real OpenAPI validator, the public-key
// middleware and the Tx + audit chain — the same chain cmd/api mounts them on.
func TestNoticePublicEndpoints_Contract(t *testing.T) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: envOr("TEST_REDIS_ADDR", "localhost:6379")})
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("no Redis: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })

	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), "noticepublic")
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM notice.notice_versions`, `DELETE FROM platform.public_keys`, `DELETE FROM notice.notices`,
				`DELETE FROM platform.document_versions`, `DELETE FROM platform.documents`,
				`DELETE FROM org.legal_entities`, `DELETE FROM iam.role_assignments`, `DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})

	client, err := jobs.NewInsertClient(app)
	if err != nil {
		t.Fatal(err)
	}
	orgSvc := &orgservice.Service{Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: orgSvc}
	versioningSvc := wiring.Versioning(nil, audit.New())
	docsSvc := wiring.Docs(versioningSvc, nil, client, audit.New(), nil)
	docsSvc.RegisterVersioning()
	svc := &noticeservice.Service{Audit: audit.New(), Org: orgSvc, Ropa: ropaSvc, Docs: docsSvc, EnforceChecklist: true}
	docsSvc.SetValidate("notice", svc.CheckPublishable)
	docsSvc.SetOnPublished("notice", svc.OnDocumentPublished)

	var dpo uuid.UUID
	if err := pdb.WithTenantTx(ctx, owner, tenant.ID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		if err := tx.QueryRow(ctx,
			`INSERT INTO iam.users (tenant_id, email, display_name, status) VALUES ($1, $2, 'Dee DPO', 'active') RETURNING id`,
			tenant.ID, uuid.NewString()[:8]+"@notice.example").Scan(&dpo); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
			SELECT $1, $2, id, 'tenant' FROM iam.roles WHERE code = 'DPO' AND tenant_id IS NULL`, tenant.ID, dpo)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var le orgservice.LegalEntity
	var n noticeservice.Notice
	authorPerms := []string{"notice.document.read", "notice.document.create", "notice.document.update", "notice.document.publish"}
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: authorPerms})
		var err error
		le, err = orgSvc.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ทดสอบ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		n, err = svc.CreateWizard(ctx, noticeservice.WizardInput{LegalEntityID: le.ID, NoticeType: "privacy_notice", Title: "ประกาศสาธารณะ", Slug: "public-contract"})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Drive one real publish (submit -> DPO approve -> publish) so OnDocumentPublished fires for real.
	dpoPerms := []string{"notice.document.read", "notice.document.update", "notice.document.approve", "notice.document.publish"}
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: authorPerms})
		doc, err := docsSvc.Get(ctx, n.DocumentID)
		if err != nil {
			return err
		}
		_, err = docsSvc.SaveDraft(ctx, n.DocumentID, doc.RowVersion, docsservice.Draft{Title: doc.Title, LegalEntityID: &le.ID, Content: completeContent()})
		return err
	}); err != nil {
		t.Fatalf("save draft: %v", err)
	}
	var v versioning.Version
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: tenant.UserID.String(), Permissions: authorPerms})
		doc, err := docsSvc.Get(ctx, n.DocumentID)
		if err != nil {
			return err
		}
		v, err = versioningSvc.Submit(ctx, doc.Latest.ID, doc.Latest.RowVersion)
		return err
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), dpo.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: dpo.String(), Permissions: dpoPerms, Roles: []string{"DPO"}})
		inbox, err := versioningSvc.Inbox(ctx)
		if err != nil {
			return err
		}
		for _, it := range inbox {
			if it.VersionID == v.ID {
				v, err = versioningSvc.Decide(ctx, it.ID, it.RowVersion, "approved", "")
				return err
			}
		}
		t.Fatal("submitted version not in the DPO's inbox")
		return nil
	}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), dpo.String(), func(ctx context.Context) error {
		ctx = authz.WithGrants(ctx, authz.Grants{TenantID: tenant.ID.String(), UserID: dpo.String(), Permissions: dpoPerms, Roles: []string{"DPO"}})
		_, err := versioningSvc.Publish(ctx, v.ID, v.RowVersion)
		return err
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	var publicKey string
	if err := pdb.WithTenantTx(ctx, app, tenant.ID.String(), tenant.UserID.String(), func(ctx context.Context) error {
		got, err := svc.GetNotice(ctx, n.ID)
		if err != nil {
			return err
		}
		if got.PublicKey == nil {
			t.Fatal("expected a public key after publish")
		}
		publicKey = *got.PublicKey
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	spec, err := openapi3.NewLoader().LoadFromFile("../../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	validateMw, _ := validate.Middleware(spec, nil)
	requestError := func(w http.ResponseWriter, r *http.Request, err error) { httpx.WriteProblem(w, r, httpx.RequestInvalid(err.Error())) }
	responseError := func(w http.ResponseWriter, r *http.Request, err error) {
		if p, ok := err.(httpx.Problem); ok {
			httpx.WriteProblem(w, r, p)
			return
		}
		httpx.WriteProblem(w, r, httpx.Internal())
	}
	idem := idempotency.New(rdb)
	auditSvc := audit.New()

	r := chi.NewRouter()
	r.Use(validateMw)
	r.Group(func(g chi.Router) {
		g.Use(publickeys.Middleware(app), idem.Handler, auditSvc.TxMiddleware(app))
		strict := noticepublichttp.NewStrictHandlerWithOptions(noticepublichttp.NewStrict(svc), nil,
			noticepublichttp.StrictHTTPServerOptions{RequestErrorHandlerFunc: requestError, ResponseErrorHandlerFunc: responseError})
		noticepublichttp.HandlerWithOptions(strict, noticepublichttp.ChiServerOptions{BaseRouter: g, ErrorHandlerFunc: requestError})
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	do := func(path string) (int, string) {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(res.Body)
		return res.StatusCode, buf.String()
	}

	if code, _ := do("/public/v1/notices/" + uuid.NewString()); code != 404 {
		t.Errorf("unknown key: %d, want 404", code)
	}
	code, body := do("/public/v1/notices/" + publicKey)
	if code != 200 {
		t.Fatalf("current version: %d %s", code, body)
	}
	var pub map[string]any
	if err := json.Unmarshal([]byte(body), &pub); err != nil {
		t.Fatal(err)
	}
	if pub["title"] != n.Title {
		t.Errorf("title = %v, want %q", pub["title"], n.Title)
	}
	content, _ := pub["content"].(map[string]any)
	if html, _ := content["th"].(string); !strings.Contains(html, "จ่ายเงินเดือน") {
		t.Errorf("expected the published Thai text in the rendered HTML, got %q", html)
	}

	code, body = do("/public/v1/notices/" + publicKey + "/versions")
	if code != 200 || !strings.Contains(body, `"version_no":1`) {
		t.Errorf("versions: %d %s", code, body)
	}

	code, body = do("/public/v1/notices/" + publicKey + "/versions/1")
	if code != 200 || !strings.Contains(body, `"version_no":1`) {
		t.Errorf("get version 1: %d %s", code, body)
	}
	if code, _ := do("/public/v1/notices/" + publicKey + "/versions/99"); code != 404 {
		t.Errorf("unknown version: %d, want 404", code)
	}
}
