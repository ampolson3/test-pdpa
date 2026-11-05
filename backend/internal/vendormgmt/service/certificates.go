// VEN-06 (evidence & certificates, ม.37(1)-adjacent — แนวปฏิบัติตลาด): vendor.certificates was already
// fully specified in the baseline migrations, unused until now. Deliberately scoped to the one acceptance
// criterion ("ใบรับรองใกล้หมดอายุถูกแจ้งเตือนล่วงหน้า") — a single reminder checkpoint per certificate's own
// valid_to, the same one-time-per-record pattern DPA-10's own agreement.renewal_reminder already
// established, not a recurring sweep (a certificate's expiry date doesn't move on its own the way a
// recurring re-assessment cycle does).
package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/jobs"
	"pdpa-platform/internal/platform/notify"
	vendorstore "pdpa-platform/internal/vendormgmt/store"
)

// CertificateEntityType is the PLT-09 entity type a certificate's own evidence file attaches under.
const CertificateEntityType = "vendor_certificate"

// CertificateExpiryNoticeDays is how far ahead of valid_to the reminder fires — a tunable default (not a
// per-certificate setting the module doc asks for, unlike DPA-10's own per-agreement renewal_notice_days),
// so CLAUDE.md's "implement the listed default as config" applies rather than a decisions.md question.
const CertificateExpiryNoticeDays = 30

// Certificate is one vendor's own compliance certificate (ISO 27001, SOC 2, …) and its evidence file.
type Certificate struct {
	ID         uuid.UUID
	VendorID   uuid.UUID
	CertType   string
	Issuer     *string
	FileID     uuid.UUID
	ValidFrom  *time.Time
	ValidTo    *time.Time
	VerifiedBy *uuid.UUID
	RowVersion int32
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CertificateInput is what a caller provides when recording a certificate.
type CertificateInput struct {
	VendorID  uuid.UUID
	CertType  string
	Issuer    *string
	FileID    uuid.UUID
	ValidFrom *time.Time
	ValidTo   *time.Time
}

// AddCertificate records a vendor's certificate: the vendor must already be visible under RLS (rule 1), and
// the evidence file must be the caller's own clean, still-unattached upload (the same "caller's own clean
// upload" pattern DSAR-04/VEN-14 already use) — attached to CertificateEntityType once this succeeds. A
// valid_to schedules this feature's own single expiry-reminder checkpoint.
func (s *Service) AddCertificate(ctx context.Context, in CertificateInput) (Certificate, error) {
	if in.CertType == "" {
		return Certificate{}, fmt.Errorf("%w: cert_type", ErrInvalid)
	}
	if in.ValidFrom != nil && in.ValidTo != nil && in.ValidTo.Before(*in.ValidFrom) {
		return Certificate{}, fmt.Errorf("%w: valid_to before valid_from", ErrInvalid)
	}
	if _, err := s.GetVendor(ctx, in.VendorID); err != nil {
		return Certificate{}, fmt.Errorf("%w: vendor_id", ErrInvalid)
	}
	f, err := s.Files.Get(ctx, in.FileID)
	if err != nil || f.EntityType != "" {
		return Certificate{}, fmt.Errorf("%w: file_id", ErrInvalid)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Certificate{}, err
	}
	row, err := vendorstore.New(pdb.MustTxFromContext(ctx)).InsertCertificate(ctx, vendorstore.InsertCertificateParams{
		ID: id, VendorID: in.VendorID, CertType: in.CertType, Issuer: in.Issuer, FileID: in.FileID,
		ValidFrom: pgDate(in.ValidFrom), ValidTo: pgDate(in.ValidTo),
	})
	if err != nil {
		return Certificate{}, err
	}
	if err := s.Files.Attach(ctx, in.FileID, CertificateEntityType, id); err != nil {
		return Certificate{}, err
	}
	out := toCertificate(row)
	if err := s.scheduleCertificateExpiryReminder(ctx, out.ID, out.ValidTo); err != nil {
		return Certificate{}, err
	}
	if err := s.audit(ctx, "vendor.certificate.create", in.VendorID, nil,
		map[string]any{"certificate_id": out.ID, "cert_type": out.CertType, "valid_to": out.ValidTo}); err != nil {
		return Certificate{}, err
	}
	return out, nil
}

// ListCertificates lists one vendor's certificates, soonest-expiring first.
func (s *Service) ListCertificates(ctx context.Context, vendorID uuid.UUID) ([]Certificate, error) {
	if _, err := s.GetVendor(ctx, vendorID); err != nil {
		return nil, err
	}
	rows, err := vendorstore.New(pdb.MustTxFromContext(ctx)).ListCertificates(ctx, vendorID)
	if err != nil {
		return nil, err
	}
	out := make([]Certificate, 0, len(rows))
	for _, r := range rows {
		out = append(out, toCertificate(vendorstore.InsertCertificateRow(r)))
	}
	return out, nil
}

// DeleteCertificate removes a certificate record (not its file — PLT-09's own retention rules apply), the
// same plain by-id removal ROPA-08's own DeleteActivityTransfer already uses for a simple child table.
func (s *Service) DeleteCertificate(ctx context.Context, vendorID, id uuid.UUID) error {
	n, err := vendorstore.New(pdb.MustTxFromContext(ctx)).DeleteCertificate(ctx, vendorstore.DeleteCertificateParams{
		ID: id, VendorID: vendorID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return s.audit(ctx, "vendor.certificate.delete", vendorID, map[string]any{"certificate_id": id}, nil)
}

// CertificateReminderArgs is vendor.certificate_expiry: the single checkpoint of one certificate's own
// valid_to. ValidTo pins the schedule so a stale tick (the certificate's date changed, or it was deleted,
// since this job was enqueued) is harmless.
type CertificateReminderArgs struct {
	jobs.TenantArgs
	CertificateID string `json:"certificate_id"`
	ValidTo       string `json:"valid_to"`
}

func (CertificateReminderArgs) Kind() string { return "vendor.certificate_expiry" }

// scheduleCertificateExpiryReminder enqueues the single checkpoint CertificateExpiryNoticeDays before
// validTo, or immediately if that moment has already passed (a certificate recorded with fewer days left
// than the notice period still alerts once, at once — the same "late-recorded event still alerts" rule
// PNG-04/DSAR-07/DPA-10 already use). A nil validTo enqueues nothing — there is no date to count down to.
func (s *Service) scheduleCertificateExpiryReminder(ctx context.Context, id uuid.UUID, validTo *time.Time) error {
	if s.River == nil || validTo == nil {
		return nil
	}
	at := validTo.AddDate(0, 0, -CertificateExpiryNoticeDays)
	if now := s.now(); at.Before(now) {
		at = now
	}
	tenant, err := tenantID(ctx)
	if err != nil {
		return err
	}
	args := CertificateReminderArgs{TenantArgs: jobs.TenantArgs{TenantID: tenant.String()}, CertificateID: id.String(),
		ValidTo: validTo.Format("2006-01-02")}
	_, err = jobs.Enqueue(ctx, s.River, args, &river.InsertOpts{ScheduledAt: at, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	return err
}

// FireCertificateExpiryReminder is the acceptance criterion itself: while the certificate's own valid_to
// still matches what the job was scheduled for (and the certificate wasn't deleted since), it notifies the
// vendor's relationship owner — falling back to role SEC, the module doc's own actor for this feature, the
// same "default recipients until real per-record routing exists" pattern BRE-07/PNG-04/VEN-10 already use.
func (s *Service) FireCertificateExpiryReminder(ctx context.Context, id uuid.UUID, validTo time.Time) error {
	q := vendorstore.New(pdb.MustTxFromContext(ctx))
	row, err := q.GetCertificate(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	cert := toCertificate(vendorstore.InsertCertificateRow(row))
	if cert.ValidTo == nil || !cert.ValidTo.Equal(validTo) {
		return nil // stale schedule
	}
	if s.Notify == nil {
		return nil
	}
	v, err := s.GetVendor(ctx, cert.VendorID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	recipients, err := s.certificateReminderRecipients(ctx, v.RelationshipOwnerID)
	if err != nil {
		return err
	}
	vars := map[string]any{"cert_type": cert.CertType, "valid_to": cert.ValidTo.Format("2 Jan 2006")}
	for _, uid := range recipients {
		uid := uid
		for _, ch := range []string{"in_app", "email"} {
			err := pdb.Savepoint(ctx, func(ctx context.Context) error {
				_, err := s.Notify.Send(ctx, notify.Request{TemplateCode: "vendor.certificate_expiry", Channel: ch,
					RecipientUserID: &uid, Vars: vars, EntityType: VendorEntityType, EntityID: &v.ID, Urgent: false})
				return err
			})
			if err != nil && !(ch == "email" && errors.Is(err, notify.ErrInvalidRequest)) {
				return err
			}
		}
	}
	return nil
}

func (s *Service) certificateReminderRecipients(ctx context.Context, owner *uuid.UUID) ([]uuid.UUID, error) {
	if owner != nil {
		return []uuid.UUID{*owner}, nil
	}
	return iamservice.UsersWithRole(ctx, "SEC")
}

// CertificateReminderWorker works vendor.certificate_expiry.
type CertificateReminderWorker struct {
	river.WorkerDefaults[CertificateReminderArgs]
	Service *Service
}

func (w *CertificateReminderWorker) Work(ctx context.Context, job *river.Job[CertificateReminderArgs]) error {
	id, err1 := uuid.Parse(job.Args.CertificateID)
	to, err2 := time.Parse("2006-01-02", job.Args.ValidTo)
	if err := errors.Join(err1, err2); err != nil {
		return river.JobCancel(err)
	}
	return w.Service.FireCertificateExpiryReminder(ctx, id, to)
}

func toCertificate(r vendorstore.InsertCertificateRow) Certificate {
	c := Certificate{ID: r.ID, VendorID: r.VendorID, CertType: r.CertType, Issuer: r.Issuer, FileID: r.FileID,
		RowVersion: r.RowVersion, CreatedAt: r.CreatedAt.Time, UpdatedAt: r.UpdatedAt.Time}
	if r.ValidFrom.Valid {
		t := r.ValidFrom.Time
		c.ValidFrom = &t
	}
	if r.ValidTo.Valid {
		t := r.ValidTo.Time
		c.ValidTo = &t
	}
	if r.VerifiedBy.Valid {
		u := uuid.UUID(r.VerifiedBy.Bytes)
		c.VerifiedBy = &u
	}
	return c
}

func tenantID(ctx context.Context) (uuid.UUID, error) {
	var t string
	if err := pdb.MustTxFromContext(ctx).QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&t); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(t)
}

func pgDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}
