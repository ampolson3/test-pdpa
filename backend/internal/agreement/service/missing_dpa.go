// DPA-12 (ม.40): the same "sweep lists tenants, per-tenant job reads its own rows" pattern PNG-15/ROPA-15/
// VEN-10 already established (CLAUDE.md rule 1), reused here to find every processor vendor (VEN-01) with
// no agreement "ที่มีผลบังคับ" (ST-04 status 'active') on record — VEN-11 already built the per-vendor read
// (VendorContractStatus) but never the cross-vendor alert its own acceptance criterion also names; this
// closes that gap as DPA-12's own feature.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	agreementstore "pdpa-platform/internal/agreement/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	vendorservice "pdpa-platform/internal/vendormgmt/service"
)

// Dpo is what agreement reads from the dpo module (rule 9) — a vendor newly found missing a DPA gets a
// real tracked task, deduplicated across sweeps so the same vendor never gets a second one while the first
// is still open.
type Dpo interface {
	HasTaskForVendorAgreement(ctx context.Context, vendorID uuid.UUID) (bool, error)
	OpenMissingDPATask(ctx context.Context, vendorID uuid.UUID, title, description string) (uuid.UUID, error)
}

// MissingDPAVendor is one row of DPA-12's own acceptance criterion list: a processor vendor with no
// agreement of type "dpa" that has actually taken effect.
type MissingDPAVendor struct {
	VendorID            uuid.UUID
	ServiceDescription  string
	RelationshipOwnerID *uuid.UUID
}

// ListVendorsMissingDPA is the acceptance criterion itself ("คู่ค้าที่ขาด DPA ถูกพบทุกราย") computed live
// on every call — never persisted, so it can never go stale (the same rule VEN-11's own VendorContractStatus
// already uses, just across every vendor instead of one).
func (s *Service) ListVendorsMissingDPA(ctx context.Context) ([]MissingDPAVendor, error) {
	vendors, err := s.Vendor.ListActiveProcessorVendors(ctx)
	if err != nil {
		return nil, err
	}
	q := agreementstore.New(pdb.MustTxFromContext(ctx))
	out := make([]MissingDPAVendor, 0, len(vendors))
	for _, v := range vendors {
		count, err := q.CountActiveAgreementsForVendorByType(ctx,
			agreementstore.CountActiveAgreementsForVendorByTypeParams{VendorID: pgUUID(&v.ID), AgreementType: "dpa"})
		if err != nil {
			return nil, err
		}
		if count == 0 {
			out = append(out, MissingDPAVendor{VendorID: v.ID, ServiceDescription: v.ServiceDescription, RelationshipOwnerID: v.RelationshipOwnerID})
		}
	}
	return out, nil
}

// MissingDPASweepArgs is agreement.missing_dpa_sweep: platform-wide (jobs.GlobalKinds), no tenant
// transaction — it only enqueues one agreement.missing_dpa_check per tenant, which does the real
// (RLS-scoped) read.
type MissingDPASweepArgs struct{}

func (MissingDPASweepArgs) Kind() string { return "agreement.missing_dpa_sweep" }

// MissingDPASweeper enqueues agreement.missing_dpa_check for every trial/active tenant.
type MissingDPASweeper struct {
	river.WorkerDefaults[MissingDPASweepArgs]

	Pool  *pgxpool.Pool
	River *river.Client[pgx.Tx]
}

func (w *MissingDPASweeper) Work(ctx context.Context, _ *river.Job[MissingDPASweepArgs]) error {
	tenants, err := agreementstore.New(w.Pool).ListActiveTenants(ctx)
	if err != nil {
		return fmt.Errorf("agreement.missing_dpa_sweep: list tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil
	}
	params := make([]river.InsertManyParams, len(tenants))
	for i, t := range tenants {
		params[i] = river.InsertManyParams{Args: MissingDPACheckArgs{TenantArgs: jobs.TenantArgs{TenantID: t.String()}}}
	}
	client := w.River
	if client == nil {
		client = river.ClientFromContext[pgx.Tx](ctx)
	}
	if _, err := client.InsertMany(ctx, params); err != nil {
		return fmt.Errorf("agreement.missing_dpa_sweep: enqueue: %w", err)
	}
	return nil
}

// MissingDPASweepPeriodicJob runs agreement.missing_dpa_sweep once a day — the same cadence VEN-10's own
// periodic re-check already uses.
func MissingDPASweepPeriodicJob() *river.PeriodicJob {
	return river.NewPeriodicJob(
		river.PeriodicInterval(24*time.Hour),
		func() (river.JobArgs, *river.InsertOpts) {
			return MissingDPASweepArgs{}, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByPeriod: 24 * time.Hour}}
		},
		&river.PeriodicJobOpts{ID: "agreement-missing-dpa-sweep", RunOnStart: false},
	)
}

// MissingDPACheckArgs is agreement.missing_dpa_check: one tenant's own turn to look at its vendors.
type MissingDPACheckArgs struct {
	jobs.TenantArgs
}

func (MissingDPACheckArgs) Kind() string { return "agreement.missing_dpa_check" }

// MissingDPAChecker works agreement.missing_dpa_check inside the tenant's own transaction.
type MissingDPAChecker struct {
	river.WorkerDefaults[MissingDPACheckArgs]
	Service *Service
}

func (w *MissingDPAChecker) Work(ctx context.Context, _ *river.Job[MissingDPACheckArgs]) error {
	_, err := w.Service.CheckMissingDPA(ctx)
	return err
}

// CheckMissingDPA opens one dpo.tasks job and notifies PROC (falling back from the vendor's own
// relationship owner, the same "default recipients until real per-record routing exists" pattern BRE-07/
// PNG-04/VEN-06/VEN-10 already use) for every vendor newly found missing a DPA — "newly" because
// Dpo.HasTaskForVendorAgreement dedupes against a task already open for that vendor, so a vendor still
// missing one on the next sweep is a harmless no-op rather than a second task every day.
func (s *Service) CheckMissingDPA(ctx context.Context) (int, error) {
	if s.Dpo == nil {
		return 0, nil
	}
	missing, err := s.ListVendorsMissingDPA(ctx)
	if err != nil {
		return 0, err
	}
	opened := 0
	for _, v := range missing {
		has, err := s.Dpo.HasTaskForVendorAgreement(ctx, v.VendorID)
		if err != nil {
			return opened, err
		}
		if has {
			continue
		}
		title := fmt.Sprintf("ทำ DPA กับคู่ค้า: %s", v.ServiceDescription)
		if _, err := s.Dpo.OpenMissingDPATask(ctx, v.VendorID, title,
			"คู่ค้ารายนี้เป็นผู้ประมวลผลข้อมูลแต่ยังไม่มีข้อตกลงประมวลผลข้อมูล (DPA) ที่มีผลบังคับตามมาตรา 40"); err != nil {
			return opened, err
		}
		opened++
		if s.Notify != nil {
			recipients, err := s.missingDPARecipients(ctx, v.RelationshipOwnerID)
			if err != nil {
				return opened, err
			}
			vars := map[string]any{"vendor": v.ServiceDescription}
			for _, uid := range recipients {
				uid := uid
				for _, ch := range []string{"in_app", "email"} {
					err := pdb.Savepoint(ctx, func(ctx context.Context) error {
						_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "agreement.missing_dpa", Channel: ch,
							RecipientUserID: &uid, Vars: vars, EntityType: vendorservice.VendorEntityType, EntityID: &v.VendorID, Urgent: false})
						return err
					})
					if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
						return opened, err
					}
				}
			}
		}
	}
	return opened, nil
}

func (s *Service) missingDPARecipients(ctx context.Context, owner *uuid.UUID) ([]uuid.UUID, error) {
	if owner != nil {
		return []uuid.UUID{*owner}, nil
	}
	return iamservice.UsersWithRole(ctx, "PROC")
}
