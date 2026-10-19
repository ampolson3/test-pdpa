package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/google/uuid"

	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
)

// MasterDataExportEntityType is ORG-08's own export audit entity — a whole-kind export has no single
// row to attribute the entry to, the same reasoning ROPA-04's own ActivityExportEntityType already uses.
const MasterDataExportEntityType = "master_data_export"

// ExportMaster is ORG-08's export half: one kind of master data (platform defaults then the tenant's
// own, ListMaster's own order) as CSV, following ORG-19/ROPA-04's own export shape (UTF-8 BOM, the
// export itself audited). Columns cover every MasterItem field any kind uses; a column that doesn't
// apply to a given kind is simply blank on that row, the same best-effort shape ROPA-04's own export
// already uses for fields that only apply to some rows.
func (s *Service) ExportMaster(ctx context.Context, kind string) (*bytes.Buffer, int, error) {
	items, err := s.ListMaster(ctx, kind)
	if err != nil {
		return nil, 0, err
	}
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"code", "name_th", "name_en", "is_global", "is_sensitive", "sensitive_type",
		"is_vulnerable", "category", "section_ref", "for_sensitive", "requires_consent", "requires_lia",
		"adequacy_status", "region"})
	for _, it := range items {
		_ = w.Write([]string{it.Code, it.NameTh, it.NameEn, strconv.FormatBool(it.Global), strconv.FormatBool(it.IsSensitive), it.SensitiveType,
			strconv.FormatBool(it.IsVulnerable), it.Category, it.SectionRef, strconv.FormatBool(it.ForSensitive), strconv.FormatBool(it.RequiresConsent), strconv.FormatBool(it.RequiresLIA),
			it.AdequacyStatus, it.Region})
	}
	w.Flush()
	if err := s.writeExportAudit(ctx, kind, len(items)); err != nil {
		return nil, 0, err
	}
	return &buf, len(items), nil
}

func (s *Service) writeExportAudit(ctx context.Context, kind string, rows int) error {
	if s.Audit == nil {
		return nil
	}
	g, _ := authz.FromContext(ctx)
	tenant, err := uuid.Parse(g.TenantID)
	if err != nil {
		var t string
		if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
			return err
		}
		if tenant, err = uuid.Parse(t); err != nil {
			return fmt.Errorf("org: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: "org.master_data.export", EntityType: MasterDataExportEntityType,
		After: map[string]any{"kind": kind, "rows": rows}}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}
