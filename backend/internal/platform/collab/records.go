package collab

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	iamservice "pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
	pdb "pdpa-platform/internal/pkg/db"
	collabstore "pdpa-platform/internal/platform/collab/store"
	"pdpa-platform/internal/platform/files"
)

// Attachment is a file attached to a record.
type Attachment struct {
	ID           uuid.UUID
	FileName     string
	MimeType     string
	SizeBytes    int64
	AVStatus     string
	UploadedBy   *uuid.UUID
	UploaderName string
	CreatedAt    time.Time
}

// Attachments lists the files attached to a record.
func (s *Service) Attachments(ctx context.Context, entityType string, entityID uuid.UUID) ([]Attachment, error) {
	if _, _, err := s.authorize(ctx, entityType, entityID, false); err != nil {
		return nil, err
	}
	rows, err := collabstore.New(pdb.MustTxFromContext(ctx)).ListRecordAttachments(ctx, collabstore.ListRecordAttachmentsParams{EntityType: &entityType, EntityID: uuidParam(entityID)})
	if err != nil {
		return nil, err
	}
	out := make([]Attachment, 0, len(rows))
	var ids []uuid.UUID
	for _, r := range rows {
		a := Attachment{ID: r.ID, FileName: r.FileName, MimeType: r.MimeType, SizeBytes: r.SizeBytes, AVStatus: r.AvStatus, CreatedAt: r.CreatedAt.Time}
		if r.CreatedBy.Valid {
			id := uuid.UUID(r.CreatedBy.Bytes)
			a.UploadedBy = &id
			ids = append(ids, id)
		}
		out = append(out, a)
	}
	names, err := iamservice.Names(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].UploadedBy != nil {
			out[i].UploaderName = names[*out[i].UploadedBy]
		}
	}
	return out, nil
}

// Attach links a file the caller uploaded (PLT-09) to a record and records it in the record's activity.
func (s *Service) Attach(ctx context.Context, entityType string, entityID, fileID uuid.UUID) error {
	_, g, err := s.authorize(ctx, entityType, entityID, true)
	if err != nil {
		return err
	}
	// Only the uploader can see an unattached file; this also proves the caller owns it.
	f, err := s.Files.Get(ctx, fileID)
	if errors.Is(err, files.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := s.Files.Attach(ctx, fileID, entityType, entityID); err != nil {
		if errors.Is(err, files.ErrAlreadyAttached) {
			return ErrInvalid
		}
		return err
	}
	return s.audit(ctx, g, "platform.file.attach", entityType, entityID, nil, map[string]any{"file_id": fileID, "file_name": f.FileName})
}

// Activity is one entry of a record's timeline.
type Activity struct {
	ID         int64
	OccurredAt time.Time
	ActorType  string
	ActorID    *uuid.UUID
	ActorName  string
	Action     string
	Before     map[string]any
	After      map[string]any
}

// Activity returns the record's audit trail, newest first: comments, attachments and whatever changes its
// module audits against it.
func (s *Service) Activity(ctx context.Context, entityType string, entityID uuid.UUID, limit int) ([]Activity, error) {
	if _, _, err := s.authorize(ctx, entityType, entityID, false); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := collabstore.New(pdb.MustTxFromContext(ctx)).ListRecordActivity(ctx, collabstore.ListRecordActivityParams{
		EntityType: &entityType, EntityID: uuidParam(entityID), PageSize: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(rows))
	var ids []uuid.UUID
	for _, r := range rows {
		a := Activity{ID: r.ID, OccurredAt: r.OccurredAt.Time, ActorType: r.ActorType, Action: r.Action}
		if r.ActorID.Valid {
			id := uuid.UUID(r.ActorID.Bytes)
			a.ActorID = &id
			ids = append(ids, id)
		}
		_ = json.Unmarshal(r.Before, &a.Before)
		_ = json.Unmarshal(r.After, &a.After)
		out = append(out, a)
	}
	names, err := iamservice.Names(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].ActorID != nil {
			out[i].ActorName = names[*out[i].ActorID]
		}
	}
	return out, nil
}

// SearchMentionable returns users the caller can @mention (active users of the tenant), for the picker.
func (s *Service) SearchMentionable(ctx context.Context, prefix string) ([]iamservice.UserName, error) {
	if _, ok := authz.FromContext(ctx); !ok {
		return nil, ErrForbidden
	}
	return iamservice.SearchUsers(ctx, prefix)
}

func uuidParam(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }
