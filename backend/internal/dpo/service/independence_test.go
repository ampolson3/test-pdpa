package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	dposervice "pdpa-platform/internal/dpo/service"
	orgservice "pdpa-platform/internal/org/service"
)

// TestIndependenceDeclarations_RecordAndSign is DPO-03's own acceptance criterion directly: a
// declaration recorded for a year is unsigned until SignDeclaration is called, and signing it once
// stamps signed_at -- a second sign attempt (the same redelivered call, or a plain edit afterward) is
// refused, never silently moving the attestation date.
func TestIndependenceDeclarations_RecordAndSign(t *testing.T) {
	e := setup(t, "dpoindep")
	var appointmentID string
	e.in(t, func(ctx context.Context) error {
		le, err := e.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท ตัวอย่าง จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		a, err := e.svc.SaveAppointment(ctx, dposervice.Appointment{LegalEntityID: le.ID, DPOType: "internal",
			UserID: &e.user, ContactEmail: "dpo@example.com", AppointedAt: date(2026, 1, 1)}, 0)
		if err != nil {
			return err
		}
		appointmentID = a.ID.String()
		return nil
	})

	e.in(t, func(ctx context.Context) error {
		appointmentUUID := uuid.MustParse(appointmentID)

		list, err := e.svc.ListIndependenceDeclarations(ctx, appointmentUUID)
		if err != nil {
			return err
		}
		if len(list) != 0 {
			t.Fatalf("expected no declarations yet, got %d", len(list))
		}

		d, err := e.svc.RecordDeclaration(ctx, appointmentUUID, 2026, "ผู้จัดการฝ่ายกฎหมาย", false)
		if err != nil {
			return err
		}
		if d.SignedAt != nil {
			t.Error("a freshly recorded declaration must not be signed yet")
		}

		// RecordDeclaration again for the same year edits the still-unsigned draft rather than
		// creating a second row.
		d2, err := e.svc.RecordDeclaration(ctx, appointmentUUID, 2026, "ผู้จัดการฝ่ายกฎหมายและไอที", true)
		if err != nil {
			return err
		}
		if d2.ID != d.ID {
			t.Error("recording the same year twice must edit the existing draft, not create a second one")
		}
		if !d2.HasConflict {
			t.Error("expected has_conflict to update")
		}

		signed, err := e.svc.SignDeclaration(ctx, d2.ID, d2.RowVersion, nil)
		if err != nil {
			return err
		}
		if signed.SignedAt == nil {
			t.Fatal("expected signed_at to be stamped")
		}

		// The acceptance criterion itself: a signed declaration now exists for this year.
		list, err = e.svc.ListIndependenceDeclarations(ctx, appointmentUUID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].SignedAt == nil {
			t.Fatalf("expected exactly one signed declaration, got %+v", list)
		}

		// Signing again (stale or fresh version) is refused -- the attestation date never moves.
		if _, err := e.svc.SignDeclaration(ctx, signed.ID, signed.RowVersion, nil); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("expected ErrInvalid re-signing an already-signed declaration, got %v", err)
		}

		// Editing an already-signed year is also refused.
		if _, err := e.svc.RecordDeclaration(ctx, appointmentUUID, 2026, "changed", false); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("expected ErrInvalid editing an already-signed year, got %v", err)
		}

		// A new year is a fresh, unsigned draft of its own.
		next, err := e.svc.RecordDeclaration(ctx, appointmentUUID, 2027, "", false)
		if err != nil {
			return err
		}
		if next.SignedAt != nil {
			t.Error("a new year's declaration must start unsigned")
		}
		return nil
	})
}

// TestIndependenceDeclarations_UnknownAppointmentRefused: an unknown appointment id is refused before
// any row is written -- the same FK-visibility discipline every other dpo write already uses.
func TestIndependenceDeclarations_UnknownAppointmentRefused(t *testing.T) {
	e := setup(t, "dpoindepunk")
	e.in(t, func(ctx context.Context) error {
		unknown := uuid.Nil
		if _, err := e.svc.RecordDeclaration(ctx, unknown, 2026, "", false); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("expected ErrInvalid for an unknown appointment, got %v", err)
		}
		return nil
	})
}

// TestIndependenceDeclarations_TwoTenantIsolation: declarations are scoped by RLS through their own
// appointment -- a second tenant can never list or sign the first tenant's declarations.
func TestIndependenceDeclarations_TwoTenantIsolation(t *testing.T) {
	a := setup(t, "dpoindepisoa")
	b := setup(t, "dpoindepisob")

	var appointmentID string
	a.in(t, func(ctx context.Context) error {
		le, err := a.org.SaveLegalEntity(ctx, orgservice.LegalEntity{NameTh: "บริษัท เอ จำกัด", IsController: true}, 0)
		if err != nil {
			return err
		}
		app, err := a.svc.SaveAppointment(ctx, dposervice.Appointment{LegalEntityID: le.ID, DPOType: "internal",
			UserID: &a.user, ContactEmail: "dpo@a.example", AppointedAt: date(2026, 1, 1)}, 0)
		if err != nil {
			return err
		}
		appointmentID = app.ID.String()
		_, err = a.svc.RecordDeclaration(ctx, app.ID, 2026, "", false)
		return err
	})

	b.in(t, func(ctx context.Context) error {
		appointmentUUID := uuid.MustParse(appointmentID)
		if _, err := b.svc.ListIndependenceDeclarations(ctx, appointmentUUID); !errors.Is(err, dposervice.ErrNotFound) {
			t.Errorf("expected ErrNotFound listing another tenant's appointment, got %v", err)
		}
		if _, err := b.svc.RecordDeclaration(ctx, appointmentUUID, 2027, "", false); !errors.Is(err, dposervice.ErrInvalid) {
			t.Errorf("expected ErrInvalid recording against another tenant's appointment, got %v", err)
		}
		return nil
	})
}
