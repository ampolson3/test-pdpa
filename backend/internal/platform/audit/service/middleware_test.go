package service_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"pdpa-platform/internal/platform/audit/service"
)

// The request audit's action must always fit audit_log.action (varchar(80)): a longer one fails the
// insert, and with it the whole request.
func TestRequestAction(t *testing.T) {
	got := service.RequestAction("GET", "/admin/v1/platform/records/notification_template/6d12d48d-f5d8-43ea-85c3-6413bbed5e1d/comments")
	if got != "GET /admin/v1/platform/records/notification_template/{id}/comments" {
		t.Errorf("got %q", got)
	}
	long := service.RequestAction("POST", "/admin/v1/"+strings.Repeat("ส่วน/", 40))
	if n := utf8.RuneCountInString(long); n > 80 {
		t.Errorf("action is %d characters, want at most 80", n)
	}
}
