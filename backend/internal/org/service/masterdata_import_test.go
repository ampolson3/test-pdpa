package service_test

import (
	"context"
	"strings"
	"testing"

	orgservice "pdpa-platform/internal/org/service"
	"pdpa-platform/internal/platform/importer"
)

// ORG-08 (Master data import/export) — the acceptance criterion directly: a duplicate code is detected
// in the dry-run Validate, before Apply ever writes anything. Exercised against the import type's own
// Validate/Apply functions, the same style ROPA-18's own import test already established.
func TestMasterDataImport_DuplicateDetectedBeforeSaving(t *testing.T) {
	e := setup(t, "orgmdimport")
	imp := e.svc.MasterDataImport()

	e.in(t, func(ctx context.Context) error {
		// A code already used by a global default is caught before anything is written.
		cats, err := e.svc.ListMaster(ctx, orgservice.KindDataCategories)
		if err != nil {
			return err
		}
		existingCode := cats[0].Code
		row := importer.Row{"kind": orgservice.KindDataCategories, "code": existingCode, "name_th": "ซ้ำกับค่าเริ่มต้น"}
		errs := imp.Validate(ctx, 1, row)
		if len(errs) != 1 || errs[0].Column != "code" || errs[0].Message != "duplicate_code" {
			t.Fatalf("duplicate of a global default: %+v", errs)
		}

		// A genuinely new code validates clean and applies.
		newRow := importer.Row{"kind": orgservice.KindDataCategories, "code": "org08_import_cat", "name_th": "หมวดนำเข้าใหม่", "name_en": "Imported category"}
		if errs := imp.Validate(ctx, 2, newRow); len(errs) != 0 {
			t.Fatalf("Validate on a new code: %+v", errs)
		}
		if err := imp.Apply(ctx, 2, newRow); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		// The same code, imported again in a second batch, is now a duplicate against the tenant's own
		// row too (not just a global default) — caught the same way, before Apply runs again.
		errs = imp.Validate(ctx, 3, newRow)
		if len(errs) != 1 || errs[0].Column != "code" || errs[0].Message != "duplicate_code" {
			t.Fatalf("duplicate of the tenant's own just-imported row: %+v", errs)
		}

		list, err := e.svc.ListMaster(ctx, orgservice.KindDataCategories)
		if err != nil {
			return err
		}
		found := false
		for _, it := range list {
			if it.Code == "org08_import_cat" {
				found = true
				if it.Global {
					t.Error("imported row should not be a global default")
				}
			}
		}
		if !found {
			t.Error("imported category not found after Apply")
		}
		return nil
	})
}

func TestMasterDataImport_InvalidKindAndCode(t *testing.T) {
	e := setup(t, "orgmdimport2")
	imp := e.svc.MasterDataImport()

	e.in(t, func(ctx context.Context) error {
		if errs := imp.Validate(ctx, 1, importer.Row{"kind": "lawful_bases", "code": "x", "name_th": "x"}); len(errs) != 1 || errs[0].Column != "kind" {
			t.Errorf("read-only kind: %+v", errs)
		}
		if errs := imp.Validate(ctx, 2, importer.Row{"kind": "nope", "code": "x", "name_th": "x"}); len(errs) != 1 || errs[0].Column != "kind" {
			t.Errorf("unknown kind: %+v", errs)
		}
		if errs := imp.Validate(ctx, 3, importer.Row{"kind": orgservice.KindPurposes, "code": "Not-A-Valid-Code!", "name_th": "x"}); len(errs) != 1 || errs[0].Column != "code" || errs[0].Message != "invalid_code" {
			t.Errorf("bad code format: %+v", errs)
		}
		return nil
	})
}

// ExportMaster is ORG-08's export half — every default plus the tenant's own rows, as CSV.
func TestExportMaster_IncludesDefaultsAndTenantRows(t *testing.T) {
	e := setup(t, "orgmdexport")
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.CreateMaster(ctx, orgservice.KindSubjectTypes, orgservice.MasterItem{Code: "export_test_subject", NameTh: "กลุ่มทดสอบส่งออก"}); err != nil {
			return err
		}
		buf, n, err := e.svc.ExportMaster(ctx, orgservice.KindSubjectTypes)
		if err != nil {
			return err
		}
		if n < 10 {
			t.Errorf("expected at least the 9 defaults plus the new row, got %d", n)
		}
		csv := buf.String()
		if !strings.Contains(csv, "code,name_th,name_en") {
			t.Errorf("missing header: %q", csv[:min(len(csv), 80)])
		}
		if !strings.Contains(csv, "export_test_subject") {
			t.Error("tenant's own row missing from export")
		}
		return nil
	})
}

