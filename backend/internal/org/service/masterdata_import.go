package service

import (
	"context"
	"strings"

	orgstore "pdpa-platform/internal/org/store"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/importer"
)

// MasterDataImportType is ORG-08's own PLT-14 import type: one row per tenant master-data entry
// (data_categories / data_subject_types / processing_purposes — the three editable kinds; lawful_bases
// and countries stay read-only, same as through the ordinary create endpoint). The acceptance criterion
// itself ("นำเข้าแล้วรายการซ้ำถูกตรวจพบก่อนบันทึก"): Validate checks the code against every row that
// already uses it — global default or this tenant's own — via the new MasterCodeExists query, so a
// duplicate is reported in the dry-run error report before Apply ever writes anything; CreateMaster's own
// equivalent check only catches a global-default shadow pre-insert, leaving a duplicate against the
// tenant's own existing code to the real unique constraint at insert time — fine for a single create, too
// late for "detect before saving" on a bulk import.
const MasterDataImportType = "org.master_data"

func (s *Service) MasterDataImport() importer.Type {
	return importer.Type{
		Permission: "org.masterdata.create",
		Columns: []importer.Column{
			{Key: "kind", Required: true, Label: map[string]string{"th": "ประเภท (data_categories/data_subject_types/processing_purposes)", "en": "Kind"}},
			{Key: "code", Required: true, Label: map[string]string{"th": "โค้ด", "en": "Code"}},
			{Key: "name_th", Required: true, Label: map[string]string{"th": "ชื่อ (ไทย)", "en": "Name (Thai)"}},
			{Key: "name_en", Label: map[string]string{"th": "ชื่อ (อังกฤษ)", "en": "Name (English)"}},
		},
		Validate: func(ctx context.Context, _ int, r importer.Row) []importer.FieldError {
			var errs []importer.FieldError
			kind := strings.TrimSpace(r["kind"])
			if !editableKind(kind) {
				errs = append(errs, importer.FieldError{Column: "kind", Message: "invalid_kind"})
				return errs
			}
			code := strings.ToLower(strings.TrimSpace(r["code"]))
			if !masterCode.MatchString(code) {
				errs = append(errs, importer.FieldError{Column: "code", Message: "invalid_code"})
				return errs
			}
			q := orgstore.New(pdb.MustTxFromContext(ctx))
			taken, err := q.MasterCodeExists(ctx, orgstore.MasterCodeExistsParams{Kind: kind, Code: code})
			if err != nil {
				errs = append(errs, importer.FieldError{Column: "code", Message: "lookup_failed"})
				return errs
			}
			if taken {
				errs = append(errs, importer.FieldError{Column: "code", Message: "duplicate_code"})
			}
			return errs
		},
		Apply: func(ctx context.Context, _ int, r importer.Row) error {
			kind := strings.TrimSpace(r["kind"])
			it := MasterItem{Code: strings.ToLower(strings.TrimSpace(r["code"])), NameTh: r["name_th"], NameEn: r["name_en"]}
			_, err := s.CreateMaster(ctx, kind, it)
			return err
		},
	}
}
