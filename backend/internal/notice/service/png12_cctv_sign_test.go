package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	noticeservice "pdpa-platform/internal/notice/service"
	"pdpa-platform/internal/platform/docs/render"
)

// TestGenerateCCTVSign_RefusesUnpublishedOrUnknownFull is the acceptance criterion's own precondition: a
// QR code can only ever open an already-published notice.
func TestGenerateCCTVSign_RefusesUnpublishedOrUnknownFull(t *testing.T) {
	e := setup(t, "png12badfull")
	n := draftNotice(t, e, "unpublished")

	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.GenerateCCTVSign(ctx, n.ID, noticeservice.CCTVSignInput{}); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("own notice unpublished: %v, want ErrInvalid", err)
		}
		full := publishedNotice(t, e, "unknownfull")
		unknown := uuid.New()
		if _, _, err := e.svc.GenerateCCTVSign(ctx, full.ID, noticeservice.CCTVSignInput{FullNoticeID: &unknown}); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("unknown full_notice_id: %v, want ErrInvalid", err)
		}
		notPublished := draftNotice(t, e, "draftfull")
		if _, _, err := e.svc.GenerateCCTVSign(ctx, full.ID, noticeservice.CCTVSignInput{FullNoticeID: &notPublished.ID}); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("unpublished full_notice_id: %v, want ErrInvalid", err)
		}
		return nil
	})
}

// TestGenerateCCTVSign_Validation covers the plain input rules and the no-renderer sentinels (this
// fixture's own svc.PDF/svc.Image are both nil, same as every other test in this package).
func TestGenerateCCTVSign_Validation(t *testing.T) {
	e := setup(t, "png12val")
	n := publishedNotice(t, e, "val")

	e.in(t, func(ctx context.Context) error {
		if _, _, err := e.svc.GenerateCCTVSign(ctx, n.ID, noticeservice.CCTVSignInput{Lang: "fr"}); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("bad lang: %v, want ErrInvalid", err)
		}
		if _, _, err := e.svc.GenerateCCTVSign(ctx, n.ID, noticeservice.CCTVSignInput{Format: "gif"}); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("bad format: %v, want ErrInvalid", err)
		}
		if _, _, err := e.svc.GenerateCCTVSign(ctx, n.ID, noticeservice.CCTVSignInput{Format: "pdf", Paper: "letter"}); !errors.Is(err, noticeservice.ErrInvalid) {
			t.Errorf("bad paper: %v, want ErrInvalid", err)
		}
		if _, _, err := e.svc.GenerateCCTVSign(ctx, n.ID, noticeservice.CCTVSignInput{Format: "pdf"}); !errors.Is(err, render.ErrNoRenderer) {
			t.Errorf("pdf with no PDF renderer: %v, want ErrNoRenderer", err)
		}
		if _, _, err := e.svc.GenerateCCTVSign(ctx, n.ID, noticeservice.CCTVSignInput{Format: "png"}); !errors.Is(err, render.ErrNoImageRenderer) {
			t.Errorf("png with no image renderer: %v, want ErrNoImageRenderer", err)
		}
		return nil
	})
}
