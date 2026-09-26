package service_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"pdpa-platform/internal/breach/breachtest"
	breach "pdpa-platform/internal/breach/service"
	pdb "pdpa-platform/internal/pkg/db"
	"pdpa-platform/internal/platform/forms"
)

func count(t *testing.T, f *breachtest.Fixture, sql string, args ...any) int {
	t.Helper()
	var n int
	f.System(t, func(ctx context.Context) error {
		return pdb.MustTxFromContext(ctx).QueryRow(ctx, sql, args...).Scan(&n)
	})
	return n
}

func tokens(t *testing.T, f *breachtest.Fixture, id uuid.UUID) []string {
	t.Helper()
	var out []string
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		items, err := f.Svc.Timeline(ctx, id)
		for _, it := range items {
			out = append(out, it.Token)
		}
		return err
	})
	return out
}

// Acceptance BRE-02: every incident has a status and a clear owner; the PDPC notice is due 72 h after awareness.
func TestRegister_StatusOwnerDeadlineAndAlerts(t *testing.T) {
	f := breachtest.Setup(t)
	aware := f.Clock.Add(-2 * time.Hour)
	in := f.Incident(t, "ไฟล์ลูกค้ารั่ว", aware)
	if in.Status != breach.StatusReported || in.OwnerID == nil || *in.OwnerID != f.Sec || in.OwnerName != "Sam Sec" {
		t.Errorf("status/owner: %+v", in)
	}
	if !in.DueAt.Equal(aware.Add(72*time.Hour)) || in.Clock.State != "on_track" || !strings.HasPrefix(in.No, fmt.Sprintf("BR-%d-", f.Clock.Year())) {
		t.Errorf("deadline/no: %+v", in)
	}
	// The owner and both DPOs are alerted at once (in-app + e-mail), breach.reported is in the outbox.
	if n := count(t, f, `SELECT count(DISTINCT recipient_user_id)::int FROM platform.notifications WHERE entity_id = $1 AND channel = 'in_app'`, in.ID); n != 3 {
		t.Errorf("alerted %d people, want owner + 2 DPOs", n)
	}
	if n := count(t, f, `SELECT count(*)::int FROM platform.outbox_events WHERE event_type = 'breach.reported' AND aggregate_id = $1`, in.ID); n != 1 {
		t.Errorf("breach.reported events: %d", n)
	}
	// The four checkpoints are scheduled.
	if n := count(t, f, `SELECT count(*)::int FROM river_job WHERE kind = 'breach.sla_timer' AND args->>'incident_id' = $1`, in.ID.String()); n != 4 {
		t.Errorf("timers: %d", n)
	}
	// Numbering is sequential per year.
	second := f.Incident(t, "อีเมลส่งผิดคน", aware)
	if second.No <= in.No {
		t.Errorf("numbers %s then %s", in.No, second.No)
	}
	// Taking it up with a new owner; closing needs approve and a reason; ST-03 refuses a jump.
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		got, err := f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusTriage, "", &f.DPO)
		if err != nil {
			return err
		}
		if got.Status != breach.StatusTriage || *got.OwnerID != f.DPO {
			t.Errorf("triage: %+v", got)
		}
		if _, err := f.Svc.Transition(ctx, in.ID, got.RowVersion, breach.StatusClosed, "ไม่ใช่เหตุละเมิด", nil); !errors.Is(err, breach.ErrForbidden) {
			t.Errorf("SEC closing: %v", err)
		}
		if _, err := f.Svc.Transition(ctx, in.ID, got.RowVersion, breach.StatusNotifying, "", nil); !errors.Is(err, breach.ErrInvalidTransition) {
			t.Errorf("triage → notifying: %v", err)
		}
		if _, err := f.Svc.Transition(ctx, in.ID, got.RowVersion-1, breach.StatusAssessing, "", nil); !errors.Is(err, breach.ErrVersionMismatch) {
			t.Errorf("stale version: %v", err)
		}
		return nil
	})
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		cur, _ := f.Svc.Get(ctx, second.ID)
		if _, err := f.Svc.Transition(ctx, second.ID, cur.RowVersion, breach.StatusClosed, "", nil); !errors.Is(err, breach.ErrInvalidTransition) {
			t.Errorf("reported → closed: %v", err)
		}
		return nil
	})
}

// Acceptance BRE-07: alerts at every checkpoint (24 / 48 / 66 h) and escalation to executives near 72 h; overdue at 72.
func TestDeadline_RemindersAndEscalation(t *testing.T) {
	f := breachtest.Setup(t)
	aware := f.Clock
	in := f.Incident(t, "แรนซัมแวร์", aware)
	fire := func(h int) {
		f.Clock = aware.Add(time.Duration(h) * time.Hour)
		f.System(t, func(ctx context.Context) error { return f.Svc.FireTimer(ctx, in.ID, h, aware) })
	}
	people := func(template string) []uuid.UUID {
		var ids []uuid.UUID
		f.System(t, func(ctx context.Context) error {
			rows, err := pdb.MustTxFromContext(ctx).Query(ctx, `SELECT DISTINCT n.recipient_user_id FROM platform.notifications n
				JOIN platform.notification_templates t ON t.id = n.template_id WHERE n.entity_id = $1 AND t.code = $2 AND n.channel = 'in_app'`, in.ID, template)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var u uuid.UUID
				_ = rows.Scan(&u)
				ids = append(ids, u)
			}
			return rows.Err()
		})
		return ids
	}
	fire(24)
	if got := people("breach.deadline_reminder"); len(got) != 3 || slices.Contains(got, f.Exec) {
		t.Errorf("24 h reminder to %v, want owner + DPOs, no executive", got)
	}
	fire(48)
	fire(66)
	if got := people("breach.deadline_reminder"); !slices.Contains(got, f.Exec) {
		t.Errorf("66 h: executive not alerted: %v", got)
	}
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		got, err := f.Svc.Get(ctx, in.ID)
		if got.Clock.State != "due_soon" {
			t.Errorf("clock at 66 h: %+v", got.Clock)
		}
		return err
	})
	fire(72)
	if got := people("breach.deadline_overdue"); !slices.Contains(got, f.Exec) || len(got) != 4 {
		t.Errorf("overdue to %v", got)
	}
	if got := tokens(t, f, in.ID); !slices.Equal(got[len(got)-4:], []string{"deadline:24", "deadline:48", "deadline:66", "deadline:72"}) {
		t.Errorf("timeline %v", got)
	}
	// A stale schedule (aware_at moved) does nothing; moving aware_at needs approve + a reason and reschedules.
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		cur, _ := f.Svc.Get(ctx, in.ID)
		_, err := f.Svc.Update(ctx, in.ID, cur.RowVersion, breach.Input{Title: cur.Title, Description: cur.Description, BreachTypes: cur.BreachTypes, AwareAt: aware.Add(time.Hour), AwareAtReason: "x"})
		if !errors.Is(err, breach.ErrForbidden) {
			t.Errorf("SEC moving aware_at: %v", err)
		}
		return nil
	})
	newAware := aware.Add(10 * time.Hour)
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		cur, _ := f.Svc.Get(ctx, in.ID)
		in2 := breach.Input{Title: cur.Title, Description: cur.Description, BreachTypes: cur.BreachTypes, AwareAt: newAware}
		if _, err := f.Svc.Update(ctx, in.ID, cur.RowVersion, in2); err == nil {
			t.Error("aware_at moved without a reason")
		}
		in2.AwareAtReason = "ทีม IT ยืนยันเวลาที่พบเหตุจริง"
		got, err := f.Svc.Update(ctx, in.ID, cur.RowVersion, in2)
		if err == nil && !got.DueAt.Equal(newAware.Add(72*time.Hour)) {
			t.Errorf("due after moving: %v", got.DueAt)
		}
		return err
	})
	before := len(tokens(t, f, in.ID))
	f.System(t, func(ctx context.Context) error { return f.Svc.FireTimer(ctx, in.ID, 24, aware) })
	if len(tokens(t, f, in.ID)) != before {
		t.Error("stale timer fired")
	}
}

// Acceptance BRE-05 / BRE-06 / BRE-12: the assessment gives a risk level with a reason per factor; every decision has
// a reason and an approver; the timeline shows every decision with its time and who made it.
func TestAssessmentDecisionTimeline(t *testing.T) {
	f := breachtest.Setup(t)
	form := f.AssessmentForm(t)
	in := f.Incident(t, "ฐานข้อมูลลูกค้าถูกเข้าถึง", f.Clock)
	var cur breach.Incident
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		var err error
		cur, err = f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusTriage, "", nil)
		if err != nil {
			return err
		}
		if _, err := f.Svc.Assess(ctx, in.ID, form, forms.Answers{"sensitive": "yes"}); !errors.Is(err, breach.ErrInvalidTransition) {
			t.Errorf("assessing in triage: %v", err)
		}
		cur, err = f.Svc.Transition(ctx, in.ID, cur.RowVersion, breach.StatusAssessing, "", nil)
		return err
	})
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		_, err := f.Svc.Assess(ctx, in.ID, form, forms.Answers{"sensitive": "yes"})
		var ve *breach.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("missing answers: %v", err)
		}
		a, err := f.Svc.Assess(ctx, in.ID, form, forms.Answers{"sensitive": "yes", "volume": "many", "encrypted": "no"})
		if err != nil {
			return err
		}
		if a.RiskLevel != "high" || a.Score != 8 || len(a.Factors) != 3 || a.Factors[0].Question != "sensitive" || a.Factors[0].Points != 5 ||
			a.Factors[0].Label["th"] != "มีข้อมูลอ่อนไหวหรือไม่" {
			t.Errorf("assessment: %+v", a)
		}
		if _, err := f.Svc.Decide(ctx, in.ID, cur.RowVersion, breach.DecisionPDPCAndSubjects, "x"); !errors.Is(err, breach.ErrForbidden) {
			t.Errorf("SEC deciding: %v", err)
		}
		return nil
	})
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		cur, _ = f.Svc.Get(ctx, in.ID)
		if cur.RiskLevel != "high" {
			t.Errorf("incident risk %q", cur.RiskLevel)
		}
		if _, err := f.Svc.Decide(ctx, in.ID, cur.RowVersion, breach.DecisionPDPC, "แจ้ง สคส. อย่างเดียว"); !errors.Is(err, breach.ErrDecisionTooWeak) {
			t.Errorf("weaker than high risk requires: %v", err)
		}
		if _, err := f.Svc.Decide(ctx, in.ID, cur.RowVersion, breach.DecisionPDPCAndSubjects, " "); err == nil {
			t.Error("decision without a reason")
		}
		got, err := f.Svc.Decide(ctx, in.ID, cur.RowVersion, breach.DecisionPDPCAndSubjects, "ข้อมูลสุขภาพไม่ได้เข้ารหัส กระทบผู้ป่วยกว่า 100 ราย")
		if err != nil {
			return err
		}
		if got.Status != breach.StatusNotifying || got.Decision != breach.DecisionPDPCAndSubjects || got.DecidedBy == nil || *got.DecidedBy != f.DPO {
			t.Errorf("decided: %+v", got)
		}
		// Leaving notifying needs the recorded PDPC notice (BRE-09).
		if _, err := f.Svc.Transition(ctx, in.ID, got.RowVersion, breach.StatusRemediating, "", nil); !errors.Is(err, breach.ErrPDPCNoticeMissing) {
			t.Errorf("notifying → remediating: %v", err)
		}
		return nil
	})
	var items []breach.TimelineItem
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		var err error
		items, err = f.Svc.Timeline(ctx, in.ID)
		return err
	})
	var decisions []string
	for _, it := range items {
		if it.Type == "decision" {
			decisions = append(decisions, it.Token+"|"+it.ActorName)
			if it.OccurredAt.IsZero() {
				t.Errorf("decision without a time: %+v", it)
			}
		}
	}
	want := []string{"status:reported:triage|Sam Sec", "status:triage:assessing|Sam Sec", "assessment:high:8|Sam Sec",
		"decision:notify_pdpc_and_subjects:high|Dee DPO", "status:assessing:notifying|Dee DPO"}
	if !slices.Equal(decisions, want) {
		t.Errorf("decisions\n got %v\nwant %v", decisions, want)
	}
	if i := slices.IndexFunc(items, func(it breach.TimelineItem) bool { return strings.HasPrefix(it.Token, "decision:") }); i < 0 || items[i].Text == "" {
		t.Error("the decision's reason is not on the timeline")
	}
	// The timeline is insert-only for the app.
	f.System(t, func(ctx context.Context) error {
		err := pdb.Savepoint(ctx, func(ctx context.Context) error {
			_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `UPDATE breach.timeline_events SET description = 'x'`)
			return err
		})
		if err == nil {
			t.Error("app role updated the timeline")
		}
		return nil
	})
	// Low risk may be escalated, none → no notice and the clock stops.
	other := f.Incident(t, "เอกสารหาย", f.Clock)
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		c, _ := f.Svc.Transition(ctx, other.ID, other.RowVersion, breach.StatusTriage, "", nil)
		c, _ = f.Svc.Transition(ctx, other.ID, c.RowVersion, breach.StatusAssessing, "", nil)
		if _, err := f.Svc.Decide(ctx, other.ID, c.RowVersion, breach.DecisionNone, "x"); !errors.Is(err, breach.ErrNoAssessment) {
			t.Errorf("decision before assessment: %v", err)
		}
		a, err := f.Svc.Assess(ctx, other.ID, form, forms.Answers{"sensitive": "no", "volume": "few", "encrypted": "yes"})
		if err != nil || a.RiskLevel != "none" {
			t.Fatalf("none assessment: %+v %v", a, err)
		}
		c, _ = f.Svc.Get(ctx, other.ID)
		got, err := f.Svc.Decide(ctx, other.ID, c.RowVersion, breach.DecisionNone, "ข้อมูลเข้ารหัสทั้งหมด ไม่มีความเสี่ยง")
		if err != nil || got.Status != breach.StatusRemediating || got.Clock.State != "stopped" {
			t.Errorf("no notice: %+v %v", got, err)
		}
		return nil
	})
}

// Acceptance BRE-10: 10,000 data subjects are notified and each one's delivery can be followed; maker-checker.
func TestSubjectNotice_TenThousandTrackedPerPerson(t *testing.T) {
	f := breachtest.Setup(t)
	form := f.AssessmentForm(t)
	in := f.Incident(t, "รหัสผ่านลูกค้ารั่ว", f.Clock)
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		c, _ := f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusTriage, "", nil)
		c, _ = f.Svc.Transition(ctx, in.ID, c.RowVersion, breach.StatusAssessing, "", nil)
		if _, err := f.Svc.CreateNotice(ctx, in.ID, "email", breach.NoticeVars{}); !errors.Is(err, breach.ErrNoticeNotRequired) {
			t.Errorf("notice before the decision: %v", err)
		}
		if _, err := f.Svc.Assess(ctx, in.ID, form, forms.Answers{"sensitive": "yes", "volume": "many", "encrypted": "no"}); err != nil {
			return err
		}
		c, _ = f.Svc.Get(ctx, in.ID)
		_, err := f.Svc.Decide(ctx, in.ID, c.RowVersion, breach.DecisionPDPCAndSubjects, "ความเสี่ยงสูง")
		return err
	})
	var csv strings.Builder
	csv.WriteString("email,language\n")
	for i := 0; i < 10000; i++ {
		lang := "th"
		if i%10 == 0 {
			lang = "en"
		}
		fmt.Fprintf(&csv, "Customer%05d@Example.com,%s\n", i, lang)
	}
	csv.WriteString("customer00001@example.com,th\n") // a duplicate of line 3, counted once
	vars := breach.NoticeVars{Organization: "บริษัท ทดสอบ จำกัด", Summary: "รหัสผ่านของท่านอาจรั่วไหล", Remedy: "โปรดเปลี่ยนรหัสผ่าน", Contact: "dpo@test.example"}
	var notice breach.Notice
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		var err error
		notice, err = f.Svc.CreateNotice(ctx, in.ID, "email", vars)
		return err
	})
	bad := f.Upload(t, f.DPO, "bad.csv", []byte("email\nnot-an-address\nok@example.com\n"))
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		_, err := f.Svc.LoadRecipients(ctx, notice.ID, notice.RowVersion, bad)
		var ve *breach.ValidationError
		if !errors.As(err, &ve) || ve.Fields[0].Field != "line 2" || ve.Fields[0].Code != "invalid_address" {
			t.Errorf("bad file: %v", err)
		}
		return nil
	})
	file := f.Upload(t, f.DPO, "recipients.csv", []byte(csv.String()))
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		var err error
		notice, err = f.Svc.LoadRecipients(ctx, notice.ID, notice.RowVersion, file)
		if err == nil && notice.Total != 10000 {
			t.Errorf("total %d", notice.Total)
		}
		if err != nil {
			return err
		}
		// Maker-checker: the DPO who drafted it can't approve it.
		if _, err := f.Svc.Send(ctx, notice.ID, notice.RowVersion); !errors.Is(err, breach.ErrSelfApproval) {
			t.Errorf("self approval: %v", err)
		}
		return nil
	})
	// Addresses are stored encrypted.
	if n := count(t, f, `SELECT count(*)::int FROM breach.notification_recipients WHERE position(convert_to('example.com','UTF8') in address_enc) > 0`); n != 0 {
		t.Errorf("%d readable addresses", n)
	}
	f.As(t, f.DPO2, breachtest.DPO, func(ctx context.Context) error {
		var err error
		notice, err = f.Svc.Send(ctx, notice.ID, notice.RowVersion)
		return err
	})
	start := time.Now()
	for more := true; more; {
		f.System(t, func(ctx context.Context) error {
			var err error
			more, err = f.Svc.SendChunk(ctx, notice.ID)
			return err
		})
	}
	t.Logf("handed 10,000 notices to the notification service in %v", time.Since(start))
	f.As(t, f.DPO, breachtest.DPO, func(ctx context.Context) error {
		got, err := f.Svc.GetNotice(ctx, notice.ID)
		if err != nil {
			return err
		}
		if got.Status != "done" || got.Handed != 10000 || got.Failed != 0 || got.Delivery["queued"] != 10000 || got.ApprovedByName != "Dana DPO" {
			t.Errorf("notice: %+v", got)
		}
		rs, err := f.Svc.Recipients(ctx, notice.ID, "", nil, 3)
		if err != nil {
			return err
		}
		if len(rs) != 3 || rs[0].Line != 2 || rs[0].Masked == "" || strings.Contains(rs[0].Masked, "customer00000") || rs[0].Status != "sent" ||
			rs[0].Delivery != "queued" || rs[0].Language != "en" {
			t.Errorf("recipients: %+v", rs)
		}
		return nil
	})
	if n := count(t, f, `SELECT count(*)::int FROM platform.outbox_events WHERE event_type = 'breach.subjects_notified' AND aggregate_id = $1`, in.ID); n != 1 {
		t.Errorf("subjects_notified events: %d", n)
	}
	if n := count(t, f, `SELECT count(*)::int FROM platform.notifications n JOIN platform.notification_templates t ON t.id = n.template_id
		WHERE n.entity_id = $1 AND t.code = 'breach.subject_notice' AND n.channel = 'email'`, in.ID); n != 10000 {
		t.Errorf("notices queued: %d", n)
	}
}

// Acceptance BRE-12 evidence + BRE-13 search, and who sees what.
func TestEvidenceSearchAndAccess(t *testing.T) {
	f := breachtest.Setup(t)
	in := f.Incident(t, "แล็ปท็อปหาย", f.Clock.Add(-30*time.Hour))
	f.Incident(t, "อีเมลส่งผิดคน", f.Clock.Add(-10*time.Hour))
	file := f.Upload(t, f.Sec, "firewall.txt", []byte("2026-09-01 blocked 10.0.0.1\n"))
	f.As(t, f.Sec, breachtest.SEC, func(ctx context.Context) error {
		ev, err := f.Svc.AddEvidence(ctx, in.ID, file, "log ไฟร์วอลล์")
		if err != nil {
			return err
		}
		if len(ev.SHA256) != 64 {
			t.Errorf("evidence: %+v", ev)
		}
		if _, err := f.Svc.AddEvidence(ctx, in.ID, file, ""); !errors.Is(err, breach.ErrFileNotUsable) {
			t.Errorf("same file twice: %v", err)
		}
		list, err := f.Svc.ListEvidence(ctx, in.ID)
		if err != nil || len(list) != 1 || list[0].SHA256 != ev.SHA256 || list[0].CollectedByName != "Sam Sec" {
			t.Errorf("list: %+v %v", list, err)
		}
		if err := f.Svc.AddNote(ctx, in.ID, nil, "ติดต่อผู้ใช้แล็ปท็อปแล้ว"); err != nil {
			return err
		}
		// Search by number, words, status; newest awareness first; paging.
		all, _, err := f.Svc.List(ctx, breach.Filter{})
		if err != nil || len(all) != 2 || all[0].Title != "อีเมลส่งผิดคน" {
			t.Errorf("list: %v %v", titles(all), err)
		}
		hit, _, _ := f.Svc.List(ctx, breach.Filter{Query: "แล็ปท็อป"})
		if len(hit) != 1 || hit[0].ID != in.ID {
			t.Errorf("search: %v", titles(hit))
		}
		byNo, _, _ := f.Svc.List(ctx, breach.Filter{Query: in.No})
		if len(byNo) != 1 {
			t.Errorf("by number: %v", titles(byNo))
		}
		page, next, _ := f.Svc.List(ctx, breach.Filter{Limit: 1})
		rest, _, _ := f.Svc.List(ctx, breach.Filter{Limit: 1, Cursor: next})
		if len(page) != 1 || len(rest) != 1 || page[0].ID == rest[0].ID {
			t.Errorf("paging: %v / %v", titles(page), titles(rest))
		}
		if pct, _, _ := f.Svc.List(ctx, breach.Filter{Query: "%"}); len(pct) != 0 {
			t.Errorf("LIKE wildcard not escaped: %v", titles(pct))
		}
		return nil
	})
	// Someone who may only report sees just their own incidents.
	f.As(t, f.Employee, breachtest.Employee, func(ctx context.Context) error {
		mine, err := f.Svc.Create(ctx, breach.Input{LegalEntityID: f.EntityA, ReportedVia: "employee_form", Title: "พบเอกสารลูกค้าในถังขยะ",
			Description: "ชั้น 3", BreachTypes: []string{"confidentiality"}, AwareAt: f.Clock})
		if err != nil {
			return err
		}
		list, _, err := f.Svc.List(ctx, breach.Filter{})
		if err != nil || len(list) != 1 || list[0].ID != mine.ID {
			t.Errorf("employee sees %v %v", titles(list), err)
		}
		if _, err := f.Svc.Get(ctx, in.ID); !errors.Is(err, breach.ErrNotFound) {
			t.Errorf("employee reads another's incident: %v", err)
		}
		return nil
	})
	if err := f.Try(f.Exec, nil, func(ctx context.Context) error {
		_, _, err := f.Svc.List(ctx, breach.Filter{})
		return err
	}); !errors.Is(err, breach.ErrForbidden) {
		t.Errorf("no permission: %v", err)
	}
}

func titles(ins []breach.Incident) []string {
	var out []string
	for _, in := range ins {
		out = append(out, in.Title)
	}
	return out
}

func TestTwoTenantIsolation(t *testing.T) {
	f := breachtest.Setup(t)
	in := f.Incident(t, "เหตุของ A", f.Clock)
	f.As(t, f.B.UserID, breachtest.DPO, func(ctx context.Context) error {
		if _, err := f.Svc.Get(ctx, in.ID); !errors.Is(err, breach.ErrNotFound) {
			t.Errorf("B reads A's incident: %v", err)
		}
		list, _, err := f.Svc.List(ctx, breach.Filter{})
		if err != nil || len(list) != 0 {
			t.Errorf("B lists %v %v", titles(list), err)
		}
		if _, err := f.Svc.Transition(ctx, in.ID, in.RowVersion, breach.StatusTriage, "", nil); !errors.Is(err, breach.ErrNotFound) {
			t.Errorf("B moves A's incident: %v", err)
		}
		// A's legal entity isn't B's to use.
		_, err = f.Svc.Create(ctx, breach.Input{LegalEntityID: f.EntityA, ReportedVia: "email", Title: "x", Description: "y",
			BreachTypes: []string{"integrity"}, AwareAt: f.Clock})
		var ve *breach.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("B uses A's legal entity: %v", err)
		}
		return nil
	})
}
