package docs

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

type stubOrgFields map[string]string

func (s stubOrgFields) MergeFields(context.Context, uuid.UUID) (map[string]string, error) { return s, nil }

type stubDpoFields map[string]string

func (s stubDpoFields) MergeFields(context.Context, uuid.UUID) (map[string]string, error) { return s, nil }

// TestFieldValues_DpoMergeField is DPO-01's acceptance criterion at the docs layer: the current DPO's
// contact channel resolves into every document's merge fields, alongside the organization's own (org and
// dpo are independent sources — neither can blank out the other's values).
func TestFieldValues_DpoMergeField(t *testing.T) {
	s := &Service{
		Org: stubOrgFields{"org_name_th": "บริษัท ตัวอย่าง จำกัด", "org_email": "info@example.com"},
		Dpo: stubDpoFields{"dpo_name": "สมชาย ใจดี", "dpo_email": "dpo@example.com"},
	}
	le := uuid.New()
	values, err := s.fieldValues(context.Background(), "ประกาศทดสอบ", &le, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"th", "en"} {
		m := values[lang]
		if m["org_name_th"] != "บริษัท ตัวอย่าง จำกัด" || m["org_email"] != "info@example.com" {
			t.Errorf("%s: org fields missing: %+v", lang, m)
		}
		if m["dpo_name"] != "สมชาย ใจดี" || m["dpo_email"] != "dpo@example.com" {
			t.Errorf("%s: dpo fields missing: %+v", lang, m)
		}
	}
}

// TestFieldValues_NoDpoAppointment: when the dpo module reports no current appointment (empty map), the
// dpo_* keys are simply absent — same as any other unresolved merge field, not an error.
func TestFieldValues_NoDpoAppointment(t *testing.T) {
	s := &Service{Org: stubOrgFields{"org_name_th": "บริษัท ตัวอย่าง จำกัด"}, Dpo: stubDpoFields{}}
	le := uuid.New()
	now := time.Now()
	values, err := s.fieldValues(context.Background(), "x", &le, 1, &now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := values["th"]["dpo_name"]; ok {
		t.Errorf("expected no dpo_name without an appointment: %+v", values["th"])
	}
}
