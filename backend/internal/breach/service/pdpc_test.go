package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/breach/breachtest"
	breach "pdpa-platform/internal/breach/service"
	"pdpa-platform/internal/platform/forms"
)

func toNotifying(t *testing.T, f *breachtest.Fixture, form uuid.UUID, title string, aware time.Time, decision, reason string) breach.Incident {
	t.Helper()
	in := f.Incident(t, title, aware)
	var out breach.Incident
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		c, err := f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusTriage, "", nil)
		if err != nil {
			return err
		}
		if c, err = f.Svc.Transition(ctx, in.ID, c.RowVersion, breach.StatusAssessing, "", nil); err != nil {
			return err
		}
		if _, err := f.Svc.Assess(ctx, in.ID, form, forms.Answers{"sensitive": "no", "volume": "many", "encrypted": "no"}); err != nil {
			return err
		}
		c, err = f.Svc.Get(ctx, in.ID)
		if err != nil {
			return err
		}
		out, err = f.Svc.Decide(ctx, in.ID, c.RowVersion, decision, reason)
		return err
	})
	return out
}

// Acceptance BRE-09: leaving "notifying" needs a confirmed PDPC filing round citing a published pdpc_form document,
// and its evidence is kept; a second person must confirm it (maker-checker), matching the decision's data-subject
// requirement too (ST-03).
func TestPDPCNotification_Acceptance(t *testing.T) {
	f := breachtest.Setup(t)
	form := f.AssessmentForm(t)
	docVersion := f.PDPCForm(t, "แบบแจ้ง สคส. เหตุรั่วไหลข้อมูลลูกค้า")
	in := toNotifying(t, f, form, "ฐานข้อมูลลูกค้าถูกเข้าถึง", f.Clock.Add(-time.Hour), breach.DecisionPDPC, "แจ้ง สคส.")

	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		if _, err := f.Svc.CreatePDPCNotification(ctx, in.ID, "initial", docVersion, f.Clock, "", nil, ""); !errors.Is(err, breach.ErrForbidden) {
			t.Errorf("SEC recording a PDPC round: %v", err)
		}
		return nil
	})
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		if _, err := f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusRemediating, "", nil); !errors.Is(err, breach.ErrPDPCNoticeMissing) {
			t.Errorf("no round recorded yet: %v", err)
		}
		return nil
	})

	var n breach.PDPCNotification
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		var err error
		if _, err = f.Svc.CreatePDPCNotification(ctx, in.ID, "bogus", docVersion, f.Clock, "", nil, ""); err == nil {
			t.Error("bad notification_type accepted")
		}
		if _, err = f.Svc.CreatePDPCNotification(ctx, in.ID, "initial", docVersion, f.Clock.Add(time.Hour), "", nil, ""); err == nil {
			t.Error("submitted_at in the future accepted")
		}
		n, err = f.Svc.CreatePDPCNotification(ctx, in.ID, "initial", docVersion, f.Clock, "PDPC-2026-000123", nil, "")
		if err != nil {
			return err
		}
		if n.SequenceNo != 1 || n.IsLate || n.NotificationType != "initial" || n.CreatedByName == "" {
			t.Errorf("recorded round: %+v", n)
		}
		// Still blocked: the round hasn't been confirmed by a second person.
		if _, err := f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusRemediating, "", nil); !errors.Is(err, breach.ErrPDPCNoticeMissing) {
			t.Errorf("unconfirmed round: %v", err)
		}
		// The maker can't confirm their own round.
		if _, err := f.Svc.ConfirmPDPCNotification(ctx, n.ID, n.RowVersion); !errors.Is(err, breach.ErrSelfApproval) {
			t.Errorf("self confirmation: %v", err)
		}
		return nil
	})
	f.As(t, f.DPO2, breachtest.DPO, func(ctx context.Context) error {
		var err error
		n, err = f.Svc.ConfirmPDPCNotification(ctx, n.ID, n.RowVersion)
		if err != nil {
			return err
		}
		if n.ApprovedByName == "" {
			t.Errorf("confirmed round: %+v", n)
		}
		if _, err := f.Svc.ConfirmPDPCNotification(ctx, n.ID, n.RowVersion); !errors.Is(err, breach.ErrInvalidTransition) {
			t.Errorf("confirming twice: %v", err)
		}
		return nil
	})
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		list, err := f.Svc.ListPDPCNotifications(ctx, in.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ApprovedByName != "Dana DPO" {
			t.Errorf("list: %+v", list)
		}
		got, err := f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusRemediating, "", nil)
		if err != nil {
			return err
		}
		if got.Status != breach.StatusRemediating {
			t.Errorf("status %q", got.Status)
		}
		return nil
	})
	if got := tokens(t, f, in.ID); !containsAll(got, "pdpc_recorded:initial", "pdpc_confirmed:initial") {
		t.Errorf("timeline: %v", got)
	}
}

// Acceptance BRE-08: a round filed after the 72-hour window can't be recorded without a reason for the delay.
func TestPDPCNotification_LateNeedsReason(t *testing.T) {
	f := breachtest.Setup(t)
	form := f.AssessmentForm(t)
	docVersion := f.PDPCForm(t, "แบบแจ้ง สคส.")
	aware := f.Clock.Add(-96 * time.Hour) // already 96h ago: any submission now is late
	in := toNotifying(t, f, form, "เหตุที่แจ้งช้า", aware, breach.DecisionPDPC, "แจ้ง สคส.")

	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		var ve *breach.ValidationError
		if _, err := f.Svc.CreatePDPCNotification(ctx, in.ID, "initial", docVersion, f.Clock, "", nil, ""); !errors.As(err, &ve) ||
			ve.Fields[0].Field != "late_reason" {
			t.Errorf("late without a reason: %v", err)
		}
		n, err := f.Svc.CreatePDPCNotification(ctx, in.ID, "initial", docVersion, f.Clock, "PDPC-REF", nil, "รอผลสอบสวนจากทีมความปลอดภัย")
		if err != nil {
			return err
		}
		if !n.IsLate || n.LateReason == "" {
			t.Errorf("late round: %+v", n)
		}
		return nil
	})
}

// ST-03: when the decision includes the data subjects, leaving "notifying" also needs their notice to have
// finished sending, not just the PDPC round.
func TestPDPCNotification_AlsoNeedsSubjectNotice(t *testing.T) {
	f := breachtest.Setup(t)
	form := f.AssessmentForm(t)
	docVersion := f.PDPCForm(t, "แบบแจ้ง สคส. ความเสี่ยงสูง")
	in := toNotifying(t, f, form, "ข้อมูลสุขภาพรั่วไหล", f.Clock, breach.DecisionPDPCAndSubjects, "ความเสี่ยงสูงต่อเจ้าของข้อมูล")

	var n breach.PDPCNotification
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		var err error
		n, err = f.Svc.CreatePDPCNotification(ctx, in.ID, "initial", docVersion, f.Clock, "PDPC-REF", nil, "")
		return err
	})
	f.As(t, f.DPO2, breachtest.DPO, func(ctx context.Context) error {
		var err error
		n, err = f.Svc.ConfirmPDPCNotification(ctx, n.ID, n.RowVersion)
		return err
	})
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		if _, err := f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusRemediating, "", nil); !errors.Is(err, breach.ErrSubjectNoticeMissing) {
			t.Errorf("subject notice not sent yet: %v", err)
		}
		return nil
	})
	var notice breach.Notice
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		var err error
		notice, err = f.Svc.CreateNotice(ctx, in.ID, "email", breach.NoticeVars{Organization: "x", Summary: "x", Remedy: "x", Contact: "x"})
		return err
	})
	file := f.Upload(t, f.DPO, "recipients.csv", []byte("email\nsubject@example.com\n"))
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		var err error
		notice, err = f.Svc.LoadRecipients(ctx, notice.ID, notice.RowVersion, file)
		return err
	})
	f.As(t, f.DPO2, breachtest.DPO, func(ctx context.Context) error {
		var err error
		notice, err = f.Svc.Send(ctx, notice.ID, notice.RowVersion)
		return err
	})
	for more := true; more; {
		f.System(t, func(ctx context.Context) error {
			var err error
			more, err = f.Svc.SendChunk(ctx, notice.ID)
			return err
		})
	}
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		got, err := f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusRemediating, "", nil)
		if err != nil {
			return err
		}
		if got.Status != breach.StatusRemediating {
			t.Errorf("status %q", got.Status)
		}
		return nil
	})
}

func containsAll(got []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, g := range got {
			if g == w {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
