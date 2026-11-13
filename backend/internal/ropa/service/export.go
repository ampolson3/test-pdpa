package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	audit "pdpa-platform/internal/platform/audit/service"
	ropastore "pdpa-platform/internal/ropa/store"
)

const ActivityExportEntityType = "activity_export"

// ExportProcessorActivities is ROPA-04's acceptance criterion: every processor-role activity (ม.40(3))
// as one CSV row, code order, following ORG-19's export shape (UTF-8 BOM, the export itself audited).
// The PDPC's actual "ประกาศ RoPA ผู้ประมวลผล พ.ศ. 2565" form text isn't available here to copy verbatim
// (see the module doc), so the column set is a best-effort draft built only from fields ROPA-03/06/08
// already model — controller identity, data categories/subjects, retention, recipients, cross-border
// transfers — flagged for legal review the same way ORG-07's Q-20 seed data was, rather than inventing
// new legally-mandated fields. It deliberately excludes lawful_basis_code: that documents the
// controller's own basis for processing, not something the processor's ม.40(3) record reports.
func (s *Service) ExportProcessorActivities(ctx context.Context) (*bytes.Buffer, int, error) {
	q := ropastore.New(pdb.MustTxFromContext(ctx))
	rows, err := q.ListProcessorActivities(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("ropa: export: %w", err)
	}

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"code", "name", "description", "controller", "org_unit", "owner",
		"data_categories", "data_subjects", "retention", "recipients", "transfers"})

	for _, r := range rows {
		a := toActivity(ropastore.GetActivityRow(r))

		controller := ""
		if a.ControllerPartyID != nil {
			if p, err := s.Org.GetExternalParty(ctx, *a.ControllerPartyID); err == nil {
				controller = displayName(p.NameTh, p.NameEn)
			}
		}
		orgUnit := ""
		if u, err := s.Org.GetOrgUnit(ctx, a.OrgUnitID); err == nil {
			orgUnit = displayName(u.NameTh, u.NameEn)
		}
		owner := ""
		if a.OwnerUserID != nil {
			if names, err := iamservice.Names(ctx, []uuid.UUID{*a.OwnerUserID}); err == nil {
				owner = names[*a.OwnerUserID]
			}
		}

		data, err := s.ListActivityData(ctx, a.ID)
		if err != nil {
			return nil, 0, err
		}
		var categories, subjects []string
		seenCat, seenSubj := map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
		for _, d := range data {
			if !seenCat[d.DataCategoryID] {
				seenCat[d.DataCategoryID] = true
				if m, err := s.Org.GetMaster(ctx, "data_categories", d.DataCategoryID); err == nil {
					categories = append(categories, displayName(m.NameTh, m.NameEn))
				}
			}
			if !seenSubj[d.SubjectTypeID] {
				seenSubj[d.SubjectTypeID] = true
				if m, err := s.Org.GetMaster(ctx, "data_subject_types", d.SubjectTypeID); err == nil {
					subjects = append(subjects, displayName(m.NameTh, m.NameEn))
				}
			}
		}

		retention, err := s.ListRetentionRules(ctx, a.ID)
		if err != nil {
			return nil, 0, err
		}
		var retentionCells []string
		for _, rr := range retention {
			period := "ไม่ระบุระยะเวลา"
			if rr.RetentionMonths != nil {
				period = strconv.Itoa(*rr.RetentionMonths) + " เดือน"
			}
			retentionCells = append(retentionCells, fmt.Sprintf("%s (%s, ทำลายโดย %s)", period, rr.TriggerEvent, rr.DisposalMethod))
		}

		recipients, err := s.ListActivityRecipients(ctx, a.ID)
		if err != nil {
			return nil, 0, err
		}
		var recipientCells []string
		for _, rc := range recipients {
			name := rc.PartyID.String()
			if p, err := s.Org.GetExternalParty(ctx, rc.PartyID); err == nil {
				name = displayName(p.NameTh, p.NameEn)
			}
			recipientCells = append(recipientCells, fmt.Sprintf("%s (%s)", name, rc.RecipientRole))
		}

		transfers, err := s.ListActivityTransfers(ctx, a.ID)
		if err != nil {
			return nil, 0, err
		}
		var transferCells []string
		for _, tr := range transfers {
			transferCells = append(transferCells, fmt.Sprintf("%s: %s", tr.CountryCode, tr.TransferBasis))
		}

		_ = w.Write([]string{a.Code, a.Name, a.Description, controller, orgUnit, owner,
			strings.Join(categories, "; "), strings.Join(subjects, "; "),
			strings.Join(retentionCells, "; "), strings.Join(recipientCells, "; "), strings.Join(transferCells, "; ")})
	}
	w.Flush()

	if err := s.writeExportAudit(ctx, len(rows)); err != nil {
		return nil, 0, err
	}
	return &buf, len(rows), nil
}

func displayName(th, en string) string {
	if th != "" {
		return th
	}
	return en
}

func (s *Service) writeExportAudit(ctx context.Context, rows int) error {
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
			return fmt.Errorf("ropa: audit without a tenant: %w", err)
		}
	}
	e := audit.Entry{TenantID: tenant, ActorType: "system", Action: "ropa.activity.export", EntityType: ActivityExportEntityType,
		After: map[string]any{"role": "processor", "rows": rows}}
	if actor, err := uuid.Parse(g.UserID); err == nil {
		e.ActorType, e.ActorID = "user", &actor
	}
	return s.Audit.Write(ctx, e)
}
