package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dpiaservice "pdpa-platform/internal/dpia/service"
	orgservice "pdpa-platform/internal/org/service"
	ropaservice "pdpa-platform/internal/ropa/service"
)

// categories returns one non-sensitive and one sensitive ORG-07 data category (the seeded defaults always
// have both — see ORG-07's own migration).
func categories(t *testing.T, ctx context.Context, org *orgservice.Service) (plain, sensitive orgservice.MasterItem) {
	t.Helper()
	items, err := org.ListMaster(ctx, "data_categories")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.IsSensitive && sensitive.ID == nil {
			sensitive = it
		}
		if !it.IsSensitive && plain.ID == nil {
			plain = it
		}
	}
	if plain.ID == nil || sensitive.ID == nil {
		t.Fatal("expected both a sensitive and a non-sensitive data category to be seeded (ORG-07)")
	}
	return plain, sensitive
}

// TestActivityDescription_MatchesSourceActivity is DPIA-04's own acceptance criterion: every field the
// description composes from RoPA is exactly what was saved on the source activity — purposes (with the
// lawful basis's own name), data (with category/subject-type names and the sensitive flag), recipients,
// cross-border transfers and retention rules.
func TestActivityDescription_MatchesSourceActivity(t *testing.T) {
	e := setup(t, "dpiadesc")
	var act ropaservice.Activity
	var basis orgservice.MasterItem
	var plain, sensitive orgservice.MasterItem
	var subjectType orgservice.MasterItem
	var party orgservice.ExternalParty
	var assessmentID uuid.UUID

	e.in(t, func(ctx context.Context) error {
		act = e.activity(t, ctx, "DESC-01")

		bases, err := e.org.ListMaster(ctx, "lawful_bases")
		if err != nil {
			return err
		}
		for _, b := range bases {
			if !b.RequiresConsent {
				basis = b
				break
			}
		}
		plain, sensitive = categories(t, ctx, e.org)
		subjects, err := e.org.ListMaster(ctx, "data_subject_types")
		if err != nil {
			return err
		}
		subjectType = subjects[0]
		if party, err = e.org.SaveExternalParty(ctx, orgservice.ExternalParty{PartyType: "processor", NameTh: "ผู้ให้บริการทดสอบ", CountryCode: "US"}, 0); err != nil {
			return err
		}

		if _, err := e.ropa.AddActivityPurpose(ctx, ropaservice.ActivityPurpose{ActivityID: act.ID, PurposeText: "ทดสอบวัตถุประสงค์", LawfulBasisCode: basis.Code}); err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityData(ctx, ropaservice.ActivityData{ActivityID: act.ID, DataCategoryID: *sensitive.ID, SubjectTypeID: *subjectType.ID, Source: "direct", IsSensitive: true}); err != nil {
			return err
		}
		recipient, err := e.ropa.AddActivityRecipient(ctx, ropaservice.ActivityRecipient{ActivityID: act.ID, PartyID: party.ID, RecipientRole: "processor", DisclosureBasis: "สัญญา"})
		if err != nil {
			return err
		}
		if _, err := e.ropa.AddActivityTransfer(ctx, ropaservice.ActivityTransfer{ActivityID: act.ID, RecipientID: &recipient.ID, CountryCode: "US", TransferBasis: "standard_clauses", Safeguards: "SCC 2021"}); err != nil {
			return err
		}
		months := 12
		if _, err := e.ropa.AddRetentionRule(ctx, ropaservice.RetentionRule{ActivityID: act.ID, DataCategoryID: sensitive.ID, RetentionMonths: &months, RetentionBasis: "กฎหมายแรงงาน", TriggerEvent: "สิ้นสุดสัญญา", DisposalMethod: "destroy"}); err != nil {
			return err
		}

		// Two or more high-risk factors → required, so the round lands in_progress with ActivityID set.
		answers := allAnswers("no")
		answers["sensitive_data"] = "yes"
		answers["large_scale"] = "yes"
		a, err := e.svc.Screen(ctx, act.ID, answers)
		if err != nil {
			return err
		}
		assessmentID = a.ID
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		d, err := e.svc.ActivityDescription(ctx, assessmentID)
		if err != nil {
			return err
		}
		if d.ActivityID != act.ID || d.ActivityCode != act.Code || d.ActivityName != act.Name || d.Role != act.Role {
			t.Errorf("activity fields don't match source: %+v vs %+v", d, act)
		}
		if len(d.Purposes) != 1 || d.Purposes[0].Text != "ทดสอบวัตถุประสงค์" || d.Purposes[0].LawfulBasisCode != basis.Code || d.Purposes[0].LawfulBasisNameTh != basis.NameTh {
			t.Errorf("purpose mismatch: %+v", d.Purposes)
		}
		if len(d.Data) != 1 || d.Data[0].CategoryNameTh != sensitive.NameTh || d.Data[0].SubjectTypeNameTh != subjectType.NameTh || !d.Data[0].IsSensitive || d.Data[0].Source != "direct" {
			t.Errorf("data mismatch: %+v", d.Data)
		}
		if len(d.Recipients) != 1 || d.Recipients[0].PartyNameTh != party.NameTh || d.Recipients[0].RecipientRole != "processor" {
			t.Errorf("recipient mismatch: %+v", d.Recipients)
		}
		if len(d.Transfers) != 1 || d.Transfers[0].TransferBasis != "standard_clauses" || d.Transfers[0].Safeguards != "SCC 2021" {
			t.Errorf("transfer mismatch: %+v", d.Transfers)
		}
		if len(d.Retention) != 1 || d.Retention[0].RetentionMonths == nil || *d.Retention[0].RetentionMonths != 12 || d.Retention[0].DisposalMethod != "destroy" {
			t.Errorf("retention mismatch: %+v", d.Retention)
		}
		if plain.ID == nil { // silence "declared and not used" if the plain category is only for contrast
			t.Fatal("unreachable")
		}
		return nil
	})
}

// TestActivityDescription_TwoTenantIsolation proves tenant B can't read tenant A's assessment description.
func TestActivityDescription_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpiadescisoA")
	b := setup(t, "dpiadescisoB")
	var assessmentID uuid.UUID
	a.in(t, func(ctx context.Context) error {
		act := a.activity(t, ctx, "DESC-ISO-01")
		answers := allAnswers("no")
		answers["sensitive_data"] = "yes"
		answers["large_scale"] = "yes"
		got, err := a.svc.Screen(ctx, act.ID, answers)
		if err != nil {
			return err
		}
		assessmentID = got.ID
		return nil
	})
	b.in(t, func(ctx context.Context) error {
		if _, err := b.svc.ActivityDescription(ctx, assessmentID); !errors.Is(err, dpiaservice.ErrNotFound) {
			t.Errorf("cross-tenant ActivityDescription: %v, want ErrNotFound", err)
		}
		return nil
	})
}
