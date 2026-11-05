package service_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/google/uuid"

	dsarservice "pdpa-platform/internal/dsar/service"
	pdb "pdpa-platform/internal/pkg/db"
)

// redactableImageUpload is the caller's own clean, still-unattached PLT-09 upload of a real PNG — the same
// "caller's own clean upload" fixture DSAR-04/DSAR-06's own tests already use, but with a real
// platform.files row (dsar.redactions.source_file_id carries a real FK, unlike DSAR-06's own masked_id_file_id
// path which never attaches the raw upload at all).
func redactableImageUpload(t *testing.T, ctx context.Context, ff *fakeFiles, img image.Image) uuid.UUID {
	t.Helper()
	var raw bytes.Buffer
	if err := png.Encode(&raw, img); err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	ff.raw[id] = raw.Bytes()
	_, err := pdb.MustTxFromContext(ctx).Exec(ctx, `INSERT INTO platform.files (id, tenant_id, bucket, object_key, file_name, mime_type, size_bytes, sha256, av_status)
		VALUES ($1, current_setting('app.tenant_id')::uuid, 'test', $2, 'photo.png', 'image/png', $3, $4, 'clean')`,
		id, id.String(), raw.Len(), hex.EncodeToString([]byte("x")))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// tenByTenAllWhite is a fresh 10x10 all-white fixture image — a new image.Image per call, since each call
// to redactableImageUpload re-encodes and uploads its own, distinct file.
func tenByTenAllWhite() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, color.White)
		}
	}
	return img
}

// TestRedactFile_BlacksOutGivenRegionsAndNeverDeliversTheSource is the acceptance criterion directly: the
// output file carries exactly the given rectangles blacked out, the source is attached only as internal
// evidence (never itself delivered), and an out-of-bounds region is refused before anything is written.
func TestRedactFile_BlacksOutGivenRegionsAndNeverDeliversTheSource(t *testing.T) {
	e, _, ff := identityEnv(t, "dsarredact")
	legalEntity := createTestLegalEntity(t, e, "บริษัท ทดสอบ จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	var rawID uuid.UUID
	var out dsarservice.Redaction
	e.in(t, func(ctx context.Context) error {
		rawID = redactableImageUpload(t, ctx, ff, tenByTenAllWhite())
		var err error
		out, err = e.svc.RedactFile(ctx, req.ID, rawID, []dsarservice.Rect{{X: 0, Y: 8, Width: 10, Height: 2}})
		return err
	})
	if out.Status != "done" || out.SourceFileID != rawID || out.OutputFileID == uuid.Nil {
		t.Fatalf("redaction = %+v", out)
	}
	if len(out.Regions) != 1 || out.Regions[0].Height != 2 {
		t.Fatalf("regions not recorded: %+v", out.Regions)
	}

	saved, ok := ff.saved[out.OutputFileID]
	if !ok {
		t.Fatal("output file was never saved")
	}
	decoded, err := png.Decode(bytes.NewReader(saved))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := decoded.At(5, 9).RGBA(); r != 0 || g != 0 || b != 0 {
		t.Errorf("redacted pixel at (5,9) = (%d,%d,%d), want black", r, g, b)
	}
	if r, g, b, _ := decoded.At(5, 0).RGBA(); r == 0 && g == 0 && b == 0 {
		t.Error("pixel outside the redacted rectangle was blacked out too")
	}

	// The source file is attached to the request (so its own real FK never dangles) but is never the
	// output — a caller reading the output file never gets back the original, unredacted bytes.
	if f, ok := ff.attached[rawID]; !ok || f.EntityType != "dsar_request" {
		t.Errorf("source file not attached to the request: %+v", f)
	}

	e.in(t, func(ctx context.Context) error {
		list, err := e.svc.ListRedactions(ctx, req.ID)
		if err != nil {
			return err
		}
		if len(list) != 1 || list[0].ID != out.ID {
			t.Errorf("ListRedactions = %+v", list)
		}
		return nil
	})

	// Out-of-bounds region refused before anything new is written.
	e.in(t, func(ctx context.Context) error {
		raw2 := redactableImageUpload(t, ctx, ff, tenByTenAllWhite())
		if _, err := e.svc.RedactFile(ctx, req.ID, raw2, []dsarservice.Rect{{X: 5, Y: 5, Width: 50, Height: 50}}); !errors.Is(err, dsarservice.ErrRedactionOutOfBounds) && !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("out-of-bounds redaction: got %v", err)
		}
		return nil
	})

	// No regions is refused up front.
	e.in(t, func(ctx context.Context) error {
		raw3 := redactableImageUpload(t, ctx, ff, tenByTenAllWhite())
		if _, err := e.svc.RedactFile(ctx, req.ID, raw3, nil); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("no regions: got %v, want ErrInvalid", err)
		}
		return nil
	})

	// An already-attached file (not the caller's own clean upload) is refused.
	e.in(t, func(ctx context.Context) error {
		if _, err := e.svc.RedactFile(ctx, req.ID, out.OutputFileID, []dsarservice.Rect{{X: 0, Y: 0, Width: 1, Height: 1}}); !errors.Is(err, dsarservice.ErrInvalid) {
			t.Errorf("already-attached file: got %v, want ErrInvalid", err)
		}
		return nil
	})
}

func TestRedactFile_TwoTenantIsolation(t *testing.T) {
	e, _, ff := identityEnv(t, "dsarredactiso")
	other := identityEnvOther(t)
	legalEntity := createTestLegalEntity(t, e, "บริษัท ทดสอบ จำกัด")
	req := createTestRequest(t, e, legalEntity, requestTypeID(t, e))

	e.in(t, func(ctx context.Context) error {
		rawID := redactableImageUpload(t, ctx, ff, tenByTenAllWhite())
		_, err := e.svc.RedactFile(ctx, req.ID, rawID, []dsarservice.Rect{{X: 0, Y: 0, Width: 1, Height: 1}})
		return err
	})

	other.in(t, func(ctx context.Context) error {
		if _, err := other.svc.ListRedactions(ctx, req.ID); !errors.Is(err, dsarservice.ErrNotFound) {
			t.Errorf("cross-tenant list: got %v, want ErrNotFound", err)
		}
		return nil
	})
}
