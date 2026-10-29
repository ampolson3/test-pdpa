// Package govhttp holds the gov module's admin endpoints (DPX-05 so far). Types in gov.gen.go
// are generated from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package govhttp

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	govservice "pdpa-platform/internal/gov/service"
	"pdpa-platform/internal/pkg/httpx"
)

type Strict struct {
	svc *govservice.Service
}

func NewStrict(svc *govservice.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) GovListRetentionSchedules(ctx context.Context, req GovListRetentionSchedulesRequestObject) (GovListRetentionSchedulesResponseObject, error) {
	f := govservice.ScheduleFilter{}
	if req.Params.AssetId != nil {
		f.AssetID = req.Params.AssetId
	}
	if req.Params.Status != nil {
		s := string(*req.Params.Status)
		f.Status = &s
	}
	if req.Params.Limit != nil {
		f.Limit = int32(*req.Params.Limit)
	}
	if req.Params.Cursor != nil {
		at, id, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.CursorAt, f.CursorID = &at, &id
	}
	list, err := h.svc.ListSchedules(ctx, f)
	if err != nil {
		return nil, problem(err)
	}
	resp := GovListRetentionSchedules200JSONResponse{Data: make([]RetentionSchedule, 0, len(list))}
	for _, s := range list {
		resp.Data = append(resp.Data, toScheduleWire(s))
	}
	if len(list) > 0 && int(f.Limit) == len(list) {
		c := encodeCursor(list[len(list)-1].CreatedAt, list[len(list)-1].ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) GovCreateRetentionSchedule(ctx context.Context, req GovCreateRetentionScheduleRequestObject) (GovCreateRetentionScheduleResponseObject, error) {
	in := govservice.RetentionSchedule{RetentionRuleID: req.Body.RetentionRuleId, AssetID: req.Body.AssetId, NextDueAt: req.Body.NextDueAt.Time}
	s, err := h.svc.SaveSchedule(ctx, in)
	if err != nil {
		return nil, problem(err)
	}
	return GovCreateRetentionSchedule201JSONResponse{Body: toScheduleWire(s), Headers: GovCreateRetentionSchedule201ResponseHeaders{ETag: etag(s.RowVersion)}}, nil
}

func (h *Strict) GovGetRetentionSchedule(ctx context.Context, req GovGetRetentionScheduleRequestObject) (GovGetRetentionScheduleResponseObject, error) {
	s, err := h.svc.GetSchedule(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return GovGetRetentionSchedule200JSONResponse{Body: toScheduleWire(s), Headers: GovGetRetentionSchedule200ResponseHeaders{ETag: etag(s.RowVersion)}}, nil
}

func (h *Strict) GovSetRetentionScheduleStatus(ctx context.Context, req GovSetRetentionScheduleStatusRequestObject) (GovSetRetentionScheduleStatusResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	s, err := h.svc.SetScheduleStatus(ctx, req.Id, rv, string(req.Body.Status))
	if err != nil {
		return nil, problem(err)
	}
	return GovSetRetentionScheduleStatus200JSONResponse{Body: toScheduleWire(s), Headers: GovSetRetentionScheduleStatus200ResponseHeaders{ETag: etag(s.RowVersion)}}, nil
}

func (h *Strict) GovListDisposalJobs(ctx context.Context, req GovListDisposalJobsRequestObject) (GovListDisposalJobsResponseObject, error) {
	f := govservice.JobFilter{}
	if req.Params.Status != nil {
		s := string(*req.Params.Status)
		f.Status = &s
	}
	if req.Params.Limit != nil {
		f.Limit = int32(*req.Params.Limit)
	}
	if req.Params.Cursor != nil {
		at, id, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.CursorAt, f.CursorID = &at, &id
	}
	list, err := h.svc.ListJobs(ctx, f)
	if err != nil {
		return nil, problem(err)
	}
	resp := GovListDisposalJobs200JSONResponse{Data: make([]DisposalJob, 0, len(list))}
	for _, j := range list {
		resp.Data = append(resp.Data, toJobWire(j))
	}
	if len(list) > 0 && int(f.Limit) == len(list) {
		c := encodeCursor(list[len(list)-1].CreatedAt, list[len(list)-1].ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) GovGetDisposalJob(ctx context.Context, req GovGetDisposalJobRequestObject) (GovGetDisposalJobResponseObject, error) {
	j, err := h.svc.GetJob(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return GovGetDisposalJob200JSONResponse{Body: toJobWire(j), Headers: GovGetDisposalJob200ResponseHeaders{ETag: etag(j.RowVersion)}}, nil
}

func (h *Strict) GovApproveDisposalJob(ctx context.Context, req GovApproveDisposalJobRequestObject) (GovApproveDisposalJobResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	var assignee *uuid.UUID
	if req.Body != nil && req.Body.AssigneeUserId != nil {
		assignee = req.Body.AssigneeUserId
	}
	j, err := h.svc.ApproveJob(ctx, req.Id, rv, assignee)
	if err != nil {
		return nil, problem(err)
	}
	return GovApproveDisposalJob200JSONResponse(toJobWire(j)), nil
}

func (h *Strict) GovStartDisposalJob(ctx context.Context, req GovStartDisposalJobRequestObject) (GovStartDisposalJobResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	j, err := h.svc.StartJob(ctx, req.Id, rv)
	if err != nil {
		return nil, problem(err)
	}
	return GovStartDisposalJob200JSONResponse(toJobWire(j)), nil
}

func (h *Strict) GovCompleteDisposalJob(ctx context.Context, req GovCompleteDisposalJobRequestObject) (GovCompleteDisposalJobResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	var recordCount *int32
	if req.Body.RecordCount != nil {
		c := int32(*req.Body.RecordCount)
		recordCount = &c
	}
	j, err := h.svc.CompleteJob(ctx, req.Id, rv, req.Body.EvidenceFileId, recordCount)
	if err != nil {
		return nil, problem(err)
	}
	return GovCompleteDisposalJob200JSONResponse(toJobWire(j)), nil
}

func (h *Strict) GovReleaseDisposalJobHold(ctx context.Context, req GovReleaseDisposalJobHoldRequestObject) (GovReleaseDisposalJobHoldResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	j, err := h.svc.ReleaseHold(ctx, req.Id, rv)
	if err != nil {
		return nil, problem(err)
	}
	return GovReleaseDisposalJobHold200JSONResponse(toJobWire(j)), nil
}

func (h *Strict) GovListPolicies(ctx context.Context, req GovListPoliciesRequestObject) (GovListPoliciesResponseObject, error) {
	list, err := h.svc.ListPolicies(ctx)
	if err != nil {
		return nil, problem(err)
	}
	resp := GovListPolicies200JSONResponse{Data: make([]GovPolicy, 0, len(list))}
	for _, p := range list {
		resp.Data = append(resp.Data, toPolicyWire(p))
	}
	return resp, nil
}

func (h *Strict) GovPublishPolicy(ctx context.Context, req GovPublishPolicyRequestObject) (GovPublishPolicyResponseObject, error) {
	requiresAttestation := true
	if req.Body.RequiresAttestation != nil {
		requiresAttestation = *req.Body.RequiresAttestation
	}
	title := ""
	if req.Body.Title != nil {
		title = *req.Body.Title
	}
	p, err := h.svc.PublishPolicy(ctx, req.Body.DocumentId, title, requiresAttestation)
	if err != nil {
		return nil, problem(err)
	}
	return GovPublishPolicy201JSONResponse{Body: toPolicyWire(p), Headers: GovPublishPolicy201ResponseHeaders{ETag: etag(p.RowVersion)}}, nil
}

func (h *Strict) GovGetPolicy(ctx context.Context, req GovGetPolicyRequestObject) (GovGetPolicyResponseObject, error) {
	p, err := h.svc.GetPolicy(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return GovGetPolicy200JSONResponse{Body: toPolicyWire(p), Headers: GovGetPolicy200ResponseHeaders{ETag: etag(p.RowVersion)}}, nil
}

func (h *Strict) GovAttestPolicy(ctx context.Context, req GovAttestPolicyRequestObject) (GovAttestPolicyResponseObject, error) {
	a, err := h.svc.AttestPolicy(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return GovAttestPolicy201JSONResponse(toAttestationWire(a)), nil
}

func (h *Strict) GovListPolicyAttestations(ctx context.Context, req GovListPolicyAttestationsRequestObject) (GovListPolicyAttestationsResponseObject, error) {
	list, err := h.svc.ListAttestations(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := GovListPolicyAttestations200JSONResponse{Data: make([]GovPolicyAttestation, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toAttestationWire(a))
	}
	return resp, nil
}

func (h *Strict) GovListMissingAttestations(ctx context.Context, req GovListMissingAttestationsRequestObject) (GovListMissingAttestationsResponseObject, error) {
	list, err := h.svc.MissingAttestations(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := GovListMissingAttestations200JSONResponse{Data: make([]GovMissingAttestation, 0, len(list))}
	for _, m := range list {
		resp.Data = append(resp.Data, GovMissingAttestation{UserId: m.UserID, UserName: m.UserName})
	}
	return resp, nil
}

func (h *Strict) GovListRegulatorLetters(ctx context.Context, req GovListRegulatorLettersRequestObject) (GovListRegulatorLettersResponseObject, error) {
	f := govservice.RegulatorLetterFilter{}
	if req.Params.Status != nil {
		s := string(*req.Params.Status)
		f.Status = &s
	}
	if req.Params.Direction != nil {
		d := string(*req.Params.Direction)
		f.Direction = &d
	}
	list, err := h.svc.ListRegulatorLetters(ctx, f)
	if err != nil {
		return nil, problem(err)
	}
	resp := GovListRegulatorLetters200JSONResponse{Data: make([]GovRegulatorLetter, 0, len(list))}
	for _, l := range list {
		resp.Data = append(resp.Data, toRegulatorLetterWire(l))
	}
	return resp, nil
}

func (h *Strict) GovCreateRegulatorLetter(ctx context.Context, req GovCreateRegulatorLetterRequestObject) (GovCreateRegulatorLetterResponseObject, error) {
	l, err := h.svc.SaveRegulatorLetter(ctx, regulatorLetterInputFromWire(*req.Body))
	if err != nil {
		return nil, problem(err)
	}
	return GovCreateRegulatorLetter201JSONResponse{Body: toRegulatorLetterWire(l), Headers: GovCreateRegulatorLetter201ResponseHeaders{ETag: etag(l.RowVersion)}}, nil
}

func (h *Strict) GovGetRegulatorLetter(ctx context.Context, req GovGetRegulatorLetterRequestObject) (GovGetRegulatorLetterResponseObject, error) {
	l, err := h.svc.GetRegulatorLetter(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return GovGetRegulatorLetter200JSONResponse{Body: toRegulatorLetterWire(l), Headers: GovGetRegulatorLetter200ResponseHeaders{ETag: etag(l.RowVersion)}}, nil
}

func (h *Strict) GovUpdateRegulatorLetter(ctx context.Context, req GovUpdateRegulatorLetterRequestObject) (GovUpdateRegulatorLetterResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	l, err := h.svc.UpdateRegulatorLetter(ctx, req.Id, rv, regulatorLetterInputFromWire(*req.Body))
	if err != nil {
		return nil, problem(err)
	}
	return GovUpdateRegulatorLetter200JSONResponse{Body: toRegulatorLetterWire(l), Headers: GovUpdateRegulatorLetter200ResponseHeaders{ETag: etag(l.RowVersion)}}, nil
}

func (h *Strict) GovDeleteRegulatorLetter(ctx context.Context, req GovDeleteRegulatorLetterRequestObject) (GovDeleteRegulatorLetterResponseObject, error) {
	if err := h.svc.DeleteRegulatorLetter(ctx, req.Id); err != nil {
		return nil, problem(err)
	}
	return GovDeleteRegulatorLetter204Response{}, nil
}

func (h *Strict) GovSetRegulatorLetterStatus(ctx context.Context, req GovSetRegulatorLetterStatusRequestObject) (GovSetRegulatorLetterStatusResponseObject, error) {
	rv, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	l, err := h.svc.SetRegulatorLetterStatus(ctx, req.Id, rv, string(req.Body.Status))
	if err != nil {
		return nil, problem(err)
	}
	return GovSetRegulatorLetterStatus200JSONResponse{Body: toRegulatorLetterWire(l), Headers: GovSetRegulatorLetterStatus200ResponseHeaders{ETag: etag(l.RowVersion)}}, nil
}

func regulatorLetterInputFromWire(in GovRegulatorLetterInput) govservice.RegulatorLetterInput {
	out := govservice.RegulatorLetterInput{Direction: string(in.Direction), LetterNo: in.LetterNo, Subject: in.Subject, FileID: in.FileId, IncidentID: in.IncidentId}
	if in.ReceivedAt != nil {
		t := in.ReceivedAt.Time
		out.ReceivedAt = &t
	}
	if in.DueAt != nil {
		t := in.DueAt.Time
		out.DueAt = &t
	}
	return out
}

func toRegulatorLetterWire(l govservice.RegulatorLetter) GovRegulatorLetter {
	w := GovRegulatorLetter{
		Id: l.ID, Direction: GovRegulatorLetterDirection(l.Direction), LetterNo: l.LetterNo, Subject: l.Subject,
		Status: GovRegulatorLetterStatus(l.Status), FileId: l.FileID, IncidentId: l.IncidentID, RowVersion: int(l.RowVersion),
	}
	if l.ReceivedAt != nil {
		w.ReceivedAt = &openapi_types.Date{Time: *l.ReceivedAt}
	}
	if l.DueAt != nil {
		w.DueAt = &openapi_types.Date{Time: *l.DueAt}
	}
	if l.RespondedAt != nil {
		w.RespondedAt = &openapi_types.Date{Time: *l.RespondedAt}
	}
	return w
}

func toPolicyWire(p govservice.Policy) GovPolicy {
	return GovPolicy{
		Id: p.ID, Title: p.Title, DocumentId: p.DocumentID, VersionNo: int(p.VersionNo),
		RequiresAttestation: p.RequiresAttestation, PublishedAt: p.PublishedAt, RowVersion: int(p.RowVersion),
	}
}

func toAttestationWire(a govservice.Attestation) GovPolicyAttestation {
	w := GovPolicyAttestation{
		Id: a.ID, PolicyId: a.PolicyID, VersionNo: int(a.VersionNo), UserId: a.UserID,
		AttestedAt: a.AttestedAt.UTC(),
	}
	if a.UserName != "" {
		w.UserName = &a.UserName
	}
	return w
}

func toScheduleWire(s govservice.RetentionSchedule) RetentionSchedule {
	return RetentionSchedule{
		Id: s.ID, RetentionRuleId: s.RetentionRuleID, AssetId: s.AssetID,
		NextDueAt: openapi_types.Date{Time: s.NextDueAt}, Status: RetentionScheduleStatus(s.Status),
		RowVersion: int(s.RowVersion), UpdatedAt: s.UpdatedAt.UTC(),
	}
}

func toJobWire(j govservice.DisposalJob) DisposalJob {
	w := DisposalJob{
		Id: j.ID, ScheduleId: j.ScheduleID, DueAt: openapi_types.Date{Time: j.DueAt}, Method: DisposalJobMethod(j.Method),
		LegalHoldId: j.LegalHoldID, AssigneeUserId: j.AssigneeUserID, ApprovedBy: j.ApprovedBy,
		Status: DisposalJobStatus(j.Status), EvidenceFileId: j.EvidenceFileID,
		RowVersion: int(j.RowVersion), UpdatedAt: j.UpdatedAt.UTC(),
	}
	if j.RecordCount != nil {
		c := int(*j.RecordCount)
		w.RecordCount = &c
	}
	if j.CompletedAt != nil {
		t := j.CompletedAt.UTC()
		w.CompletedAt = &t
	}
	return w
}

func problem(err error) error {
	switch {
	case errors.Is(err, govservice.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, govservice.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, govservice.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, govservice.ErrInvalid):
		return httpx.UnprocessableEntity("gov.invalid_input", err.Error())
	case errors.Is(err, govservice.ErrEvidenceRequired):
		return httpx.UnprocessableEntity("gov.evidence_required", err.Error())
	case errors.Is(err, govservice.ErrFileNotUsable):
		return httpx.UnprocessableEntity("gov.file_not_usable", err.Error())
	case errors.Is(err, govservice.ErrOnLegalHold):
		return httpx.InvalidTransition("gov")
	case errors.Is(err, govservice.ErrInvalidTransition):
		return httpx.InvalidTransition("gov")
	}
	return err
}

func etag(v int32) *string {
	s := `"` + strconv.Itoa(int(v)) + `"`
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(s string) (time.Time, uuid.UUID, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, uuid.UUID{}, err
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.UUID{}, errors.New("bad cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.UUID{}, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return time.Time{}, uuid.UUID{}, err
	}
	return at, id, nil
}
