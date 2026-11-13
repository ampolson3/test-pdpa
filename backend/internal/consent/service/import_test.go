package service_test

import (
	"context"
	"testing"

	"pdpa-platform/internal/consent/consenttest"
	consent "pdpa-platform/internal/consent/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/importer"
)

// setRegistrationNo gives tenant A's fixture legal entity a real registration number — CON-22's own
// lookup key — the fixture itself only sets name_th (no other test needs one).
func setRegistrationNo(t *testing.T, f *consenttest.Fixture, no string) {
	t.Helper()
	f.As(t, f.A, f.Alice, nil, nil, func(ctx context.Context) error {
		_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE org.legal_entities SET registration_no = $1 WHERE id = $2`, no, f.EntityA)
		return err
	})
}

func legacyImportRow(regNo, decision, when string) importer.Row {
	return importer.Row{
		"legal_entity_registration_no": regNo,
		"identifier_type":              "email",
		"identifier_value":             "legacy+" + decision + "@example.com",
		"purpose_code":                 "MKT-LEGACY",
		"decision":                     decision,
		"occurred_at":                  when,
	}
}

// TestLegacyImport_ConsentedRowActivatesPurpose is the acceptance criterion's own happy path: a legacy
// "consented" row lands as a real ACTIVE decision, attributable to its own imported collection point.
func TestLegacyImport_ConsentedRowActivatesPurpose(t *testing.T) {
	f := consenttest.Setup(t)
	setRegistrationNo(t, f, "1234567890121")
	f.LivePurpose(t, "MKT-LEGACY", consenttest.Content("MKT-LEGACY", "ยินยอมรับข่าวสาร (เดิม)"))
	typ := f.Svc.LegacyImport()
	row := legacyImportRow("1234567890121", "consented", "2024-01-15")

	f.As(t, f.A, f.Alice, nil, []string{"consent.bulk.execute"}, func(ctx context.Context) error {
		if errs := typ.Validate(ctx, 1, row); len(errs) != 0 {
			t.Fatalf("unexpected validation errors: %+v", errs)
		}
		if err := typ.Apply(ctx, 1, row); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		subs, err := f.Svc.Subjects(ctx, "email", "legacy+consented@example.com")
		if err != nil || len(subs) != 1 {
			t.Fatalf("Subjects: %v %+v", err, subs)
		}
		profile, err := f.Svc.GetProfile(ctx, subs[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(profile.Statuses) != 1 || profile.Statuses[0].Status != consent.StatusActive || profile.Statuses[0].PurposeCode != "MKT-LEGACY" {
			t.Fatalf("expected one ACTIVE MKT-LEGACY status, got %+v", profile.Statuses)
		}
		return nil
	})
}

// TestLegacyImport_NotConsentedRow keeps the subject NOT_GIVEN.
func TestLegacyImport_NotConsentedRow(t *testing.T) {
	f := consenttest.Setup(t)
	setRegistrationNo(t, f, "1234567890121")
	f.LivePurpose(t, "MKT-LEGACY", consenttest.Content("MKT-LEGACY", "ยินยอมรับข่าวสาร (เดิม)"))
	typ := f.Svc.LegacyImport()
	row := legacyImportRow("1234567890121", "not_consented", "2024-01-15")

	f.As(t, f.A, f.Alice, nil, []string{"consent.bulk.execute"}, func(ctx context.Context) error {
		if err := typ.Apply(ctx, 1, row); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		subs, err := f.Svc.Subjects(ctx, "email", "legacy+not_consented@example.com")
		if err != nil || len(subs) != 1 {
			t.Fatalf("Subjects: %v %+v", err, subs)
		}
		profile, err := f.Svc.GetProfile(ctx, subs[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(profile.Statuses) != 1 || profile.Statuses[0].Status != consent.StatusNotGiven {
			t.Fatalf("expected NOT_GIVEN, got %+v", profile.Statuses)
		}
		return nil
	})
}

// TestLegacyImport_WithdrawnRowReplaysBothTransactions is the acceptance criterion's own trickiest case:
// ST-01 has no "" -> WITHDRAWN edge, so a "withdrawn" legacy row replays CONSENTED then WITHDRAWN at the
// same original date, landing the subject WITHDRAWN with both transactions on record.
func TestLegacyImport_WithdrawnRowReplaysBothTransactions(t *testing.T) {
	f := consenttest.Setup(t)
	setRegistrationNo(t, f, "1234567890121")
	f.LivePurpose(t, "MKT-LEGACY", consenttest.Content("MKT-LEGACY", "ยินยอมรับข่าวสาร (เดิม)"))
	typ := f.Svc.LegacyImport()
	row := legacyImportRow("1234567890121", "withdrawn", "2024-01-15")

	f.As(t, f.A, f.Alice, nil, []string{"consent.bulk.execute"}, func(ctx context.Context) error {
		if err := typ.Apply(ctx, 1, row); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		subs, err := f.Svc.Subjects(ctx, "email", "legacy+withdrawn@example.com")
		if err != nil || len(subs) != 1 {
			t.Fatalf("Subjects: %v %+v", err, subs)
		}
		profile, err := f.Svc.GetProfile(ctx, subs[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(profile.Statuses) != 1 || profile.Statuses[0].Status != consent.StatusWithdrawn {
			t.Fatalf("expected WITHDRAWN, got %+v", profile.Statuses)
		}
		// History is newest-first (ListSubjectTransactions: ORDER BY occurred_at DESC).
		var kinds []string
		for _, h := range profile.History {
			if h.PurposeCode == "MKT-LEGACY" {
				kinds = append(kinds, h.Type)
			}
		}
		if len(kinds) != 2 || kinds[0] != consent.TxWithdrawn || kinds[1] != consent.TxConsented {
			t.Fatalf("expected [WITHDRAWN CONSENTED] history, got %v", kinds)
		}
		return nil
	})
}

// TestLegacyImport_UnknownLegalEntity refuses a registration number that matches no legal entity.
func TestLegacyImport_UnknownLegalEntity(t *testing.T) {
	f := consenttest.Setup(t)
	f.LivePurpose(t, "MKT-LEGACY", consenttest.Content("MKT-LEGACY", "ยินยอมรับข่าวสาร (เดิม)"))
	typ := f.Svc.LegacyImport()
	row := legacyImportRow("0000000000000", "consented", "2024-01-15")

	f.As(t, f.A, f.Alice, nil, []string{"consent.bulk.execute"}, func(ctx context.Context) error {
		errs := typ.Validate(ctx, 1, row)
		if len(errs) != 1 || errs[0].Column != "legal_entity_registration_no" {
			t.Fatalf("expected one legal_entity_registration_no error, got %+v", errs)
		}
		if err := typ.Apply(ctx, 1, row); err == nil {
			t.Fatal("expected Apply to refuse an unknown legal entity")
		}
		return nil
	})
}

// TestLegacyImport_UnknownPurpose refuses a purpose code that isn't a currently-published purpose.
func TestLegacyImport_UnknownPurpose(t *testing.T) {
	f := consenttest.Setup(t)
	setRegistrationNo(t, f, "1234567890121")
	typ := f.Svc.LegacyImport()
	row := legacyImportRow("1234567890121", "consented", "2024-01-15")

	f.As(t, f.A, f.Alice, nil, []string{"consent.bulk.execute"}, func(ctx context.Context) error {
		errs := typ.Validate(ctx, 1, row)
		if len(errs) != 1 || errs[0].Column != "purpose_code" {
			t.Fatalf("expected one purpose_code error, got %+v", errs)
		}
		return nil
	})
}

// TestLegacyImport_InvalidIdentifierAndDecisionAndDate covers the remaining column-level validation rules.
func TestLegacyImport_InvalidIdentifierAndDecisionAndDate(t *testing.T) {
	f := consenttest.Setup(t)
	setRegistrationNo(t, f, "1234567890121")
	f.LivePurpose(t, "MKT-LEGACY", consenttest.Content("MKT-LEGACY", "ยินยอมรับข่าวสาร (เดิม)"))
	typ := f.Svc.LegacyImport()

	cases := []struct {
		name, mutateCol, mutateVal, wantCol string
	}{
		{"bad identifier", "identifier_value", "not-an-email", "identifier_value"},
		{"bad decision", "decision", "maybe", "decision"},
		{"bad date", "occurred_at", "not-a-date", "occurred_at"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := legacyImportRow("1234567890121", "consented", "2024-01-15")
			row[c.mutateCol] = c.mutateVal
			f.As(t, f.A, f.Alice, nil, []string{"consent.bulk.execute"}, func(ctx context.Context) error {
				errs := typ.Validate(ctx, 1, row)
				found := false
				for _, e := range errs {
					if e.Column == c.wantCol {
						found = true
					}
				}
				if !found {
					t.Fatalf("expected an error on column %q, got %+v", c.wantCol, errs)
				}
				return nil
			})
		})
	}
}

// TestLegacyImport_TwoTenantIsolation: tenant B's own registration number never resolves from tenant A's
// own transaction, and importing into A never creates anything visible from B.
func TestLegacyImport_TwoTenantIsolation(t *testing.T) {
	f := consenttest.Setup(t)
	setRegistrationNo(t, f, "1234567890121")
	f.LivePurpose(t, "MKT-LEGACY", consenttest.Content("MKT-LEGACY", "ยินยอมรับข่าวสาร (เดิม)"))
	typ := f.Svc.LegacyImport()
	row := legacyImportRow("1234567890121", "consented", "2024-01-15")

	f.As(t, f.A, f.Alice, nil, []string{"consent.bulk.execute"}, func(ctx context.Context) error {
		return typ.Apply(ctx, 1, row)
	})

	f.As(t, f.B, f.B.UserID, nil, []string{"consent.bulk.execute"}, func(ctx context.Context) error {
		subs, err := f.Svc.Subjects(ctx, "email", "legacy+consented@example.com")
		if err != nil {
			t.Fatal(err)
		}
		if len(subs) != 0 {
			t.Fatalf("tenant B must not see tenant A's imported subject, got %+v", subs)
		}
		errs := typ.Validate(ctx, 1, row)
		found := false
		for _, e := range errs {
			if e.Column == "legal_entity_registration_no" {
				found = true
			}
		}
		if !found {
			t.Fatalf("tenant B must not resolve tenant A's own registration number, got %+v", errs)
		}
		return nil
	})
}
