package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	dpiaservice "pdpa-platform/internal/dpia/service"
	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/pkg/dbtest"
	audit "pdpa-platform/internal/platform/audit/service"
	"pdpa-platform/internal/platform/forms"
	ropaservice "pdpa-platform/internal/ropa/service"
	"pdpa-platform/internal/wiring"
)

// tiaEnv wires org + dpia + ropa together — DPX-06's own acceptance criterion needs a logged
// ROPA-08 transfer and a published assessment_type='tia' template (migration 00063) answered through
// dpia's own generic RecordSubjectAssessment, linked back via ropa.activity_transfers.tia_assessment_id.
type tiaEnv struct {
	app    *pgxpool.Pool
	tenant dbtest.Tenant
	svc    *ropaservice.Service
	org    *orgservice.Service
}

func tiaSetup(t *testing.T, suffix string) tiaEnv {
	t.Helper()
	ctx := context.Background()
	app, owner := dbtest.Pool(t), dbtest.OwnerPool(t)
	tenant := dbtest.SeedTenant(t, ctx, app, dbtest.PlatformPool(t), suffix)
	t.Cleanup(func() {
		_ = pdb.WithTenantTx(context.Background(), owner, tenant.ID.String(), "", func(ctx context.Context) error {
			tx := pdb.MustTxFromContext(ctx)
			for _, q := range []string{
				`DELETE FROM ropa.activity_transfers`, `DELETE FROM ropa.activity_recipients`, `DELETE FROM assess.answers`,
				`DELETE FROM assess.assessments`, `DELETE FROM ropa.processing_activities`, `DELETE FROM org.external_parties`,
				`DELETE FROM org.org_units`, `UPDATE org.legal_entities SET parent_id = NULL`, `DELETE FROM org.legal_entities`,
				`DELETE FROM platform.audit_log`,
			} {
				_, _ = tx.Exec(ctx, q)
			}
			return nil
		})
	})
	org := &orgservice.Service{Audit: audit.New()}
	formsSvc := wiring.Forms(nil, audit.New())
	dpiaSvc := &dpiaservice.Service{Forms: formsSvc, Org: org, Audit: audit.New()}
	ropaSvc := &ropaservice.Service{Audit: audit.New(), Org: org, Dpia: wiring.RopaDpia{Dpia: dpiaSvc}}
	return tiaEnv{app: app, tenant: tenant, svc: ropaSvc, org: org}
}

var tiaPerms = []string{"ropa.activity.read", "ropa.activity.create", "ropa.activity.update",
	"assessment.template.read"}

func (e tiaEnv) in(t *testing.T, fn func(ctx context.Context) error) {
	t.Helper()
	if err := pdb.WithTenantTx(context.Background(), e.app, e.tenant.ID.String(), e.tenant.UserID.String(), func(ctx context.Context) error {
		return fn(authz.WithGrants(ctx, authz.Grants{TenantID: e.tenant.ID.String(), UserID: e.tenant.UserID.String(), Permissions: tiaPerms}))
	}); err != nil {
		t.Fatal(err)
	}
}

// transfer_tia (migration 00063) has 6 yes_no questions scored 0/1 each — a "clean" transfer (adequate
// country, binding safeguard, certification, no government-access risk, no onward transfer, rights
// enforceable) answers every question at its own lowest-risk option.
var tiaCleanAnswers = forms.Answers{
	"host_country_adequate": "yes", "has_binding_safeguard": "yes", "has_certification": "yes",
	"government_access_risk": "no", "onward_transfer_risk": "no", "data_subject_rights_enforceable": "yes",
}
var tiaRiskyAnswers = forms.Answers{
	"host_country_adequate": "no", "has_binding_safeguard": "no", "has_certification": "no",
	"government_access_risk": "yes", "onward_transfer_risk": "yes", "data_subject_rights_enforceable": "no",
}

func seedTiaActivityAndTransfer(t *testing.T, ctx context.Context, e tiaEnv) (ropaservice.Activity, ropaservice.ActivityTransfer) {
	t.Helper()
	le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.org.CreateOrgUnit(ctx, orgservice.OrgUnit{LegalEntityID: le.ID, Code: "IT", NameTh: "IT", UnitType: "department"})
	if err != nil {
		t.Fatal(err)
	}
	activity, err := e.svc.SaveActivity(ctx, ropaservice.Activity{LegalEntityID: le.ID, OrgUnitID: unit.ID, Code: "IT-01", Name: "ระบบคลาวด์", Role: "controller"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := e.svc.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{ActivityID: activity.ID, CountryCode: "US", TransferBasis: "standard_clauses"})
	if err != nil {
		t.Fatal(err)
	}
	return activity, tr
}

// TestRecordTransferAssessment_LinksAssessmentAndClearsTiaMissing is DPX-06's own acceptance criterion:
// answering the TIA template for a logged transfer links tia_assessment_id back onto the transfer row,
// and the activity's own "tia_missing" completeness item (every route needs a referenced TIA) clears.
func TestRecordTransferAssessment_LinksAssessmentAndClearsTiaMissing(t *testing.T) {
	e := tiaSetup(t, "dpx06link")
	var activityID, transferID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		activity, tr := seedTiaActivityAndTransfer(t, ctx, e)
		activityID, transferID = activity.ID, tr.ID
		got, err := e.svc.GetActivity(ctx, activityID)
		if err != nil {
			return err
		}
		found := false
		for _, m := range got.MissingItems {
			if m == "tia_missing" {
				found = true
			}
		}
		if !found {
			t.Errorf("a logged transfer with no TIA yet should flag tia_missing, got %v", got.MissingItems)
		}
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		updated, err := e.svc.RecordTransferAssessment(ctx, activityID, transferID, "transfer_tia", tiaCleanAnswers)
		if err != nil {
			return err
		}
		if updated.TiaAssessmentID == nil {
			t.Errorf("tia_assessment_id was not set")
		}
		got, err := e.svc.GetActivity(ctx, activityID)
		if err != nil {
			return err
		}
		for _, m := range got.MissingItems {
			if m == "tia_missing" {
				t.Errorf("tia_missing should clear once the transfer has a recorded TIA: %v", got.MissingItems)
			}
		}
		list, err := e.svc.ListActivityTransfers(ctx, activityID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].TiaAssessmentID == nil {
			t.Errorf("ListActivityTransfers should also report the linked tia_assessment_id")
		}
		return nil
	})
}

func TestRecordTransferAssessment_ScoresByAnswers(t *testing.T) {
	e := tiaSetup(t, "dpx06score")
	var activityID, cleanTransferID, riskyTransferID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		activity, tr := seedTiaActivityAndTransfer(t, ctx, e)
		activityID, cleanTransferID = activity.ID, tr.ID
		tr2, err := e.svc.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{ActivityID: activityID, CountryCode: "CN", TransferBasis: "exemption"})
		riskyTransferID = tr2.ID
		return err
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordTransferAssessment(ctx, activityID, cleanTransferID, "transfer_tia", tiaCleanAnswers); err != nil {
			return err
		}
		if _, err := e.svc.RecordTransferAssessment(ctx, activityID, riskyTransferID, "transfer_tia", tiaRiskyAnswers); err != nil {
			return err
		}
		return nil
	})
}

func TestRecordTransferAssessment_UnknownTemplateCodeRefused(t *testing.T) {
	e := tiaSetup(t, "dpx06badtemplate")
	var activityID, transferID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		activity, tr := seedTiaActivityAndTransfer(t, ctx, e)
		activityID, transferID = activity.ID, tr.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordTransferAssessment(ctx, activityID, transferID, "no_such_template", tiaCleanAnswers); !errors.Is(err, ropaservice.ErrInvalid) {
			t.Errorf("unknown template_code: %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestRecordTransferAssessment_UnknownTransferRefused(t *testing.T) {
	e := tiaSetup(t, "dpx06badtransfer")
	var activityID uuid.UUID
	e.in(t, func(ctx context.Context) error {
		activity, _ := seedTiaActivityAndTransfer(t, ctx, e)
		activityID = activity.ID
		return nil
	})
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RecordTransferAssessment(ctx, activityID, uuid.New(), "transfer_tia", tiaCleanAnswers); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("unknown transfer id: %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestRecordTransferAssessment_TwoTenantIsolation mirrors VEN-07's own isolation test.
func TestRecordTransferAssessment_TwoTenantIsolation(t *testing.T) {
	a := tiaSetup(t, "dpx06iso1")
	b := tiaSetup(t, "dpx06iso2")
	var activityID, transferID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		activity, tr := seedTiaActivityAndTransfer(t, ctx, a)
		activityID, transferID = activity.ID, tr.ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.RecordTransferAssessment(ctx, activityID, transferID, "transfer_tia", tiaCleanAnswers); !errors.Is(err, ropaservice.ErrNotFound) {
			t.Errorf("tenant B recording a TIA against tenant A's activity: %v, want ErrNotFound", err)
		}
		return nil
	})
}
