package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	dsarstore "pdpa-platform/internal/dsar/store"
	pdb "pdpa-platform/internal/pkg/db"
)

// Redaction is DSAR-16's own record of one black-out done before a file was delivered to the data subject
// (ม.30 วรรคสอง — another person's own personal data must never be disclosed in the response). The table
// (dsar.redactions) was already fully specified in the baseline migrations, unused until now: no OCR is
// available in this environment to *find* another person's data automatically (the same gap DSAR-06/
// decisions.md Q-30 already resolved for an ID card photo), so a round skips the schema's own 'detecting'
// status straight to 'done' in one call — staff mark every rectangle themselves, the same manual-rectangle
// mechanism DSAR-06's own redactImage already established, reused here for any image, not just an identity
// upload. SourceFileID is attached to the request as internal evidence of what was covered, but it is
// never what DSAR-14/15 can deliver — OutputFileID is: the acceptance criterion ("กู้คืนจากไฟล์ผลลัพธ์
// ไม่ได้") is about the delivered file alone, which only ever carries the already-blacked-out bytes.
type Redaction struct {
	ID           uuid.UUID
	RequestID    uuid.UUID
	SourceFileID uuid.UUID
	OutputFileID uuid.UUID
	Regions      []Rect
	Status       string // detecting | review | done — always "done" in this pass, no OCR to pause on
	ReviewedBy   *uuid.UUID
	CreatedAt    time.Time
}

// RedactFile is the acceptance criterion's own write path: the caller's own clean, still-unattached upload
// (rule 1's own "caller's own clean upload" pattern — DSAR-04/VEN-06 already use it) is blacked out at every
// given rectangle and the result saved as a brand-new file, already attached to the request; the source is
// attached too (so its own real FK on dsar.redactions.source_file_id never dangles once PLT-09's 24h orphan
// sweep would otherwise have deleted an unattached upload), but only as internal evidence — DSAR-14's own
// package creation and DSAR-15's own export never read dsar.redactions, so the source is simply never
// offered to either as a deliverable unless a caller deliberately chooses its id, which this feature gives
// no path to do.
func (s *Service) RedactFile(ctx context.Context, requestID, rawFileID uuid.UUID, regions []Rect) (Redaction, error) {
	if len(regions) == 0 {
		return Redaction{}, fmt.Errorf("%w: regions", ErrInvalid)
	}
	if s.Files == nil {
		return Redaction{}, fmt.Errorf("dsar: file storage not wired")
	}
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return Redaction{}, err
	}
	f, err := s.Files.Get(ctx, rawFileID)
	if err != nil || f.EntityType != "" {
		return Redaction{}, fmt.Errorf("%w: raw_file_id", ErrInvalid)
	}
	r, _, err := s.Files.Open(ctx, rawFileID)
	if err != nil {
		return Redaction{}, fmt.Errorf("%w: raw_file_id", ErrInvalid)
	}
	defer r.Close()
	redacted, format, err := redactImage(r, regions)
	if err != nil {
		return Redaction{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	if err := s.Files.Attach(ctx, rawFileID, "dsar_request", requestID); err != nil {
		return Redaction{}, fmt.Errorf("%w: raw_file_id", ErrInvalid)
	}
	out, err := s.Files.SaveGenerated(ctx, "redacted."+format, bytes.NewReader(redacted), "dsar_request", requestID)
	if err != nil {
		return Redaction{}, err
	}
	regionsJSON, err := json.Marshal(regions)
	if err != nil {
		return Redaction{}, err
	}
	row, err := dsarstore.New(pdb.MustTxFromContext(ctx)).InsertRedaction(ctx, dsarstore.InsertRedactionParams{
		RequestID: requestID, SourceFileID: rawFileID, OutputFileID: pgUUID(&out.ID), Regions: regionsJSON,
	})
	if err != nil {
		return Redaction{}, err
	}
	result := toRedaction(dsarstore.ListRedactionsForRequestRow(row))
	return result, s.audit(ctx, "dsar.redaction.create", requestID, nil, map[string]any{"output_file_id": out.ID})
}

// ListRedactions reads every redaction performed for a request, newest first.
func (s *Service) ListRedactions(ctx context.Context, requestID uuid.UUID) ([]Redaction, error) {
	if _, err := s.GetRequest(ctx, requestID); err != nil {
		return nil, err
	}
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListRedactionsForRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	out := make([]Redaction, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRedaction(r))
	}
	return out, nil
}

func toRedaction(r dsarstore.ListRedactionsForRequestRow) Redaction {
	red := Redaction{ID: r.ID, RequestID: r.RequestID, SourceFileID: r.SourceFileID, Status: r.Status, CreatedAt: r.CreatedAt.Time}
	if r.OutputFileID.Valid {
		id := uuid.UUID(r.OutputFileID.Bytes)
		red.OutputFileID = id
	}
	if r.ReviewedBy.Valid {
		id := uuid.UUID(r.ReviewedBy.Bytes)
		red.ReviewedBy = &id
	}
	_ = json.Unmarshal(r.Regions, &red.Regions)
	return red
}
