// VEN-03 (sub-processor management, ม.40): vendor.sub_processors was already fully specified in the
// baseline migrations, unused until now — a vendor's sub-processors each start "pending" and need a DPO
// decision before use (this feature's own acceptance criterion: unapproved ones are flagged — the list
// endpoint reports every one's status, which the UI reads to show the warning). Deliberately no state
// machine entry: a flat pending -> approved | rejected has no further edges to encode.
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/notify"
	vendorstore "pdpa-platform/internal/vendormgmt/store"
)

var subProcessorDecisions = []string{"approved", "rejected"}

// SubProcessor is one vendor.sub_processors row.
type SubProcessor struct {
	ID         uuid.UUID
	VendorID   uuid.UUID
	PartyID    uuid.UUID
	Service    string
	Countries  []string
	Status     string
	ApprovedBy *uuid.UUID
	ApprovedAt *time.Time
	RowVersion int32
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SubProcessorInput is what a caller provides when recording a sub-processor.
type SubProcessorInput struct {
	PartyID   uuid.UUID
	Service   string
	Countries []string
}

// subProcessorRow mirrors every sub_processors.sql query's own shared column layout (sqlc generates a
// distinct named Row type per query rather than reusing one shared struct — the same workaround
// BRE-15/VEN-06 already established).
type subProcessorRow struct {
	ID         uuid.UUID
	VendorID   uuid.UUID
	PartyID    uuid.UUID
	Service    string
	Countries  []string
	Status     string
	ApprovedBy pgtype.UUID
	ApprovedAt pgtype.Timestamptz
	RowVersion int32
	CreatedAt  pgtype.Timestamptz
	UpdatedAt  pgtype.Timestamptz
}

func toSubProcessor(r subProcessorRow) SubProcessor {
	sp := SubProcessor{ID: r.ID, VendorID: r.VendorID, PartyID: r.PartyID, Service: r.Service, Countries: r.Countries,
		Status: r.Status, RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.ApprovedBy.Valid {
		u := uuid.UUID(r.ApprovedBy.Bytes)
		sp.ApprovedBy = &u
	}
	if r.ApprovedAt.Valid {
		t := r.ApprovedAt.Time
		sp.ApprovedAt = &t
	}
	return sp
}

// AddSubProcessor records a sub-processor the vendor uses — always "pending" to start, needing a
// vendor.vendor.approve decision before use — and alerts the DPO team at once (the acceptance criterion's
// own "แจ้งเมื่อคู่ค้าเพิ่มรายใหม่"), the same synchronous alert pattern BRE's own incident `alert` already
// uses, not a background sweep (there is nothing time-based to wait for here).
func (s *Service) AddSubProcessor(ctx context.Context, vendorID uuid.UUID, in SubProcessorInput) (SubProcessor, error) {
	if in.Service == "" {
		return SubProcessor{}, fmt.Errorf("%w: service", ErrInvalid)
	}
	if _, err := s.GetVendor(ctx, vendorID); err != nil {
		return SubProcessor{}, fmt.Errorf("%w: vendor_id", ErrInvalid)
	}
	if _, err := s.Org.GetExternalParty(ctx, in.PartyID); err != nil {
		return SubProcessor{}, fmt.Errorf("%w: party_id", ErrInvalid)
	}
	countries := in.Countries
	if countries == nil {
		countries = []string{}
	}

	id, err := uuid.NewV7()
	if err != nil {
		return SubProcessor{}, err
	}
	row, err := vendorstore.New(pdb.MustTxFromContext(ctx)).InsertSubProcessor(ctx, vendorstore.InsertSubProcessorParams{
		ID: id, VendorID: vendorID, PartyID: in.PartyID, Service: in.Service, Countries: countries,
	})
	if err != nil {
		return SubProcessor{}, err
	}
	out := toSubProcessor(subProcessorRow(row))
	if err := s.alertSubProcessorAdded(ctx, out); err != nil {
		return SubProcessor{}, err
	}
	if err := s.audit(ctx, "vendor.sub_processor.create", vendorID, nil,
		map[string]any{"sub_processor_id": out.ID, "service": out.Service}); err != nil {
		return SubProcessor{}, err
	}
	return out, nil
}

// ListSubProcessors lists one vendor's sub-processors, newest first — every row carries its own status so
// the UI can flag the unapproved ones (the acceptance criterion itself).
func (s *Service) ListSubProcessors(ctx context.Context, vendorID uuid.UUID) ([]SubProcessor, error) {
	if _, err := s.GetVendor(ctx, vendorID); err != nil {
		return nil, err
	}
	rows, err := vendorstore.New(pdb.MustTxFromContext(ctx)).ListSubProcessors(ctx, vendorID)
	if err != nil {
		return nil, err
	}
	out := make([]SubProcessor, 0, len(rows))
	for _, r := range rows {
		out = append(out, toSubProcessor(subProcessorRow(r)))
	}
	return out, nil
}

// DecideSubProcessor approves or rejects a still-pending sub-processor before it is used. Gated by
// vendor.vendor.approve beyond the endpoint's own vendor.vendor.update, the same permanent-decision
// permission VEN-14's own terminate and ORG-06/DPIA-03's own permanent status changes already reuse.
func (s *Service) DecideSubProcessor(ctx context.Context, vendorID, id uuid.UUID, decision string, rowVersion int32) (SubProcessor, error) {
	if g, _ := authz.FromContext(ctx); !g.Has("vendor.vendor.approve") {
		return SubProcessor{}, ErrForbidden
	}
	valid := false
	for _, d := range subProcessorDecisions {
		if d == decision {
			valid = true
			break
		}
	}
	if !valid {
		return SubProcessor{}, fmt.Errorf("%w: decision", ErrInvalid)
	}

	if _, err := vendorstore.New(pdb.MustTxFromContext(ctx)).GetSubProcessor(ctx, vendorstore.GetSubProcessorParams{
		ID: id, VendorID: vendorID,
	}); errors.Is(err, pgx.ErrNoRows) {
		return SubProcessor{}, ErrNotFound
	} else if err != nil {
		return SubProcessor{}, err
	}

	row, err := vendorstore.New(pdb.MustTxFromContext(ctx)).DecideSubProcessor(ctx, vendorstore.DecideSubProcessorParams{
		Status: decision, ID: id, VendorID: vendorID, RowVersion: rowVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SubProcessor{}, ErrVersionMismatch
	}
	if err != nil {
		return SubProcessor{}, err
	}
	out := toSubProcessor(subProcessorRow(row))
	if err := s.audit(ctx, "vendor.sub_processor."+decision, vendorID, nil, map[string]any{"sub_processor_id": out.ID}); err != nil {
		return SubProcessor{}, err
	}
	return out, nil
}

// alertSubProcessorAdded notifies every DPO at once — the acceptance criterion's own "แจ้งเมื่อคู่ค้าเพิ่ม
// รายใหม่" — the same synchronous, role-DPO alert BRE's own incident `alert` already uses for this exact
// shape of notification.
func (s *Service) alertSubProcessorAdded(ctx context.Context, sp SubProcessor) error {
	if s.Notify == nil {
		return nil
	}
	recipients, err := iamservice.UsersWithRole(ctx, "DPO")
	if err != nil {
		return err
	}
	vars := map[string]any{"service": sp.Service}
	for _, uid := range recipients {
		uid := uid
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "vendor.sub_processor_added", Channel: ch,
					RecipientUserID: &uid, Vars: vars, EntityType: VendorEntityType, EntityID: &sp.VendorID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}
