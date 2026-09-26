// Package breachhttp holds the admin endpoints of the breach module (BRE). Types in breach.gen.go are generated from
// api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package breachhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	breach "pdpa-platform/internal/breach/service"
	"pdpa-platform/internal/pkg/httpx"
	"pdpa-platform/internal/platform/forms"
)

type Strict struct {
	svc *breach.Service
}

func NewStrict(svc *breach.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

// ---- incidents ----

func (h *Strict) BreachListIncidents(ctx context.Context, req BreachListIncidentsRequestObject) (BreachListIncidentsResponseObject, error) {
	p := req.Params
	f := breach.Filter{Owner: p.OwnerId, From: p.From, To: p.To}
	if p.Status != nil {
		f.Status = string(*p.Status)
	}
	if p.Risk != nil {
		f.Risk = string(*p.Risk)
	}
	if p.Q != nil {
		f.Query = *p.Q
	}
	if p.OpenOnly != nil {
		f.OpenOnly = *p.OpenOnly
	}
	if p.Cursor != nil {
		f.Cursor = *p.Cursor
	}
	if p.Limit != nil {
		f.Limit = *p.Limit
	}
	list, next, err := h.svc.List(ctx, f)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListIncidents200JSONResponse{Data: make([]BreachIncident, 0, len(list))}
	for _, in := range list {
		w, err := incidentWire(in)
		if err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	if next != "" {
		out.NextCursor = &next
	}
	return out, nil
}

func input(b any) (breach.Input, error) {
	var raw struct {
		LegalEntityID       uuid.UUID   `json:"legal_entity_id"`
		ReportedVia         string      `json:"reported_via"`
		Title               string      `json:"title"`
		Description         string      `json:"description"`
		BreachTypes         []string    `json:"breach_types"`
		IncidentType        string      `json:"incident_type"`
		OccurredAt          *time.Time  `json:"occurred_at"`
		AwareAt             time.Time   `json:"aware_at"`
		ContainedAt         *time.Time  `json:"contained_at"`
		AffectedSubjects    *int32      `json:"affected_subjects"`
		AffectedCategoryIDs []uuid.UUID `json:"affected_category_ids"`
		OwnerUserID         *uuid.UUID  `json:"owner_user_id"`
		IsDrill             bool        `json:"is_drill"`
		AwareAtReason       string      `json:"aware_at_reason"`
	}
	if err := convert(b, &raw); err != nil {
		return breach.Input{}, err
	}
	return breach.Input{LegalEntityID: raw.LegalEntityID, ReportedVia: raw.ReportedVia, Title: raw.Title, Description: raw.Description,
		BreachTypes: raw.BreachTypes, IncidentType: raw.IncidentType, OccurredAt: raw.OccurredAt, AwareAt: raw.AwareAt, ContainedAt: raw.ContainedAt,
		AffectedSubjects: raw.AffectedSubjects, AffectedCategoryIDs: raw.AffectedCategoryIDs, OwnerID: raw.OwnerUserID, IsDrill: raw.IsDrill,
		AwareAtReason: raw.AwareAtReason}, nil
}

func (h *Strict) BreachCreateIncident(ctx context.Context, req BreachCreateIncidentRequestObject) (BreachCreateIncidentResponseObject, error) {
	in, err := input(req.Body)
	if err != nil {
		return nil, err
	}
	got, err := h.svc.Create(ctx, in)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := incidentWire(got)
	if err != nil {
		return nil, err
	}
	return BreachCreateIncident201JSONResponse{Body: w, Headers: BreachCreateIncident201ResponseHeaders{ETag: etag(got.RowVersion)}}, nil
}

func (h *Strict) BreachGetIncident(ctx context.Context, req BreachGetIncidentRequestObject) (BreachGetIncidentResponseObject, error) {
	got, err := h.svc.Get(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := incidentWire(got)
	if err != nil {
		return nil, err
	}
	return BreachGetIncident200JSONResponse{Body: w, Headers: BreachGetIncident200ResponseHeaders{ETag: etag(got.RowVersion)}}, nil
}

func (h *Strict) BreachUpdateIncident(ctx context.Context, req BreachUpdateIncidentRequestObject) (BreachUpdateIncidentResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in, err := input(req.Body)
	if err != nil {
		return nil, err
	}
	got, err := h.svc.Update(ctx, req.Id, v, in)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := incidentWire(got)
	if err != nil {
		return nil, err
	}
	return BreachUpdateIncident200JSONResponse{Body: w, Headers: BreachUpdateIncident200ResponseHeaders{ETag: etag(got.RowVersion)}}, nil
}

func (h *Strict) BreachTransitionIncident(ctx context.Context, req BreachTransitionIncidentRequestObject) (BreachTransitionIncidentResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	b := req.Body
	reason := ""
	if b.Reason != nil {
		reason = *b.Reason
	}
	got, err := h.svc.Transition(ctx, req.Id, v, string(b.To), reason, b.OwnerUserId)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := incidentWire(got)
	if err != nil {
		return nil, err
	}
	return BreachTransitionIncident200JSONResponse{Body: w, Headers: BreachTransitionIncident200ResponseHeaders{ETag: etag(got.RowVersion)}}, nil
}

func (h *Strict) BreachDecideIncident(ctx context.Context, req BreachDecideIncidentRequestObject) (BreachDecideIncidentResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	got, err := h.svc.Decide(ctx, req.Id, v, string(req.Body.Decision), req.Body.Reason)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := incidentWire(got)
	if err != nil {
		return nil, err
	}
	return BreachDecideIncident200JSONResponse{Body: w, Headers: BreachDecideIncident200ResponseHeaders{ETag: etag(got.RowVersion)}}, nil
}

// ---- assessments ----

func (h *Strict) BreachListAssessments(ctx context.Context, req BreachListAssessmentsRequestObject) (BreachListAssessmentsResponseObject, error) {
	list, err := h.svc.Assessments(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListAssessments200JSONResponse{Data: make([]BreachAssessment, 0, len(list))}
	for _, a := range list {
		var w BreachAssessment
		if err := convert(assessmentMap(a), &w); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	return out, nil
}

func (h *Strict) BreachAssessIncident(ctx context.Context, req BreachAssessIncidentRequestObject) (BreachAssessIncidentResponseObject, error) {
	a, err := h.svc.Assess(ctx, req.Id, req.Body.FormId, forms.Answers(req.Body.Answers))
	if err != nil {
		return nil, ToProblem(err)
	}
	var w BreachAssessment
	if err := convert(assessmentMap(a), &w); err != nil {
		return nil, err
	}
	return BreachAssessIncident201JSONResponse(w), nil
}

// ---- timeline & evidence ----

func (h *Strict) BreachGetTimeline(ctx context.Context, req BreachGetTimelineRequestObject) (BreachGetTimelineResponseObject, error) {
	items, err := h.svc.Timeline(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachGetTimeline200JSONResponse{Data: make([]BreachTimelineItem, 0, len(items))}
	for _, it := range items {
		m := map[string]any{"id": it.ID, "occurred_at": it.OccurredAt.UTC(), "type": it.Type, "auto": it.Auto}
		if it.Token != "" {
			m["token"] = it.Token
		}
		if it.Text != "" {
			m["text"] = it.Text
		}
		if it.ActorID != nil {
			m["actor_id"], m["actor_name"] = it.ActorID, it.ActorName
		}
		var w BreachTimelineItem
		if err := convert(m, &w); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	return out, nil
}

func (h *Strict) BreachAddNote(ctx context.Context, req BreachAddNoteRequestObject) (BreachAddNoteResponseObject, error) {
	if err := h.svc.AddNote(ctx, req.Id, req.Body.OccurredAt, req.Body.Text); err != nil {
		return nil, ToProblem(err)
	}
	return BreachAddNote204Response{}, nil
}

func evidenceMap(e breach.Evidence) map[string]any {
	m := map[string]any{"id": e.ID, "file_id": e.FileID, "file_name": e.FileName, "sha256": e.SHA256, "size_bytes": e.SizeBytes, "av_status": e.AVStatus,
		"collected_at": e.CollectedAt.UTC()}
	if e.Description != "" {
		m["description"] = e.Description
	}
	if e.CollectedBy != nil {
		m["collected_by"], m["collected_by_name"] = e.CollectedBy, e.CollectedByName
	}
	return m
}

func (h *Strict) BreachListEvidence(ctx context.Context, req BreachListEvidenceRequestObject) (BreachListEvidenceResponseObject, error) {
	list, err := h.svc.ListEvidence(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListEvidence200JSONResponse{Data: make([]BreachEvidence, 0, len(list))}
	for _, e := range list {
		var w BreachEvidence
		if err := convert(evidenceMap(e), &w); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	return out, nil
}

func (h *Strict) BreachAddEvidence(ctx context.Context, req BreachAddEvidenceRequestObject) (BreachAddEvidenceResponseObject, error) {
	desc := ""
	if req.Body.Description != nil {
		desc = *req.Body.Description
	}
	e, err := h.svc.AddEvidence(ctx, req.Id, req.Body.FileId, desc)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w BreachEvidence
	if err := convert(evidenceMap(e), &w); err != nil {
		return nil, err
	}
	return BreachAddEvidence201JSONResponse(w), nil
}

// ---- PDPC filing rounds (BRE-08/09) ----

func pdpcMap(n breach.PDPCNotification) map[string]any {
	m := map[string]any{"id": n.ID, "incident_id": n.IncidentID, "sequence_no": n.SequenceNo, "notification_type": n.NotificationType,
		"document_version_id": n.DocumentVersionID, "submitted_at": n.SubmittedAt.UTC(), "is_late": n.IsLate, "row_version": n.RowVersion,
		"created_at": n.CreatedAt.UTC()}
	if n.SubmissionRef != "" {
		m["submission_ref"] = n.SubmissionRef
	}
	if n.LateReason != "" {
		m["late_reason"] = n.LateReason
	}
	if n.EvidenceFileID != nil {
		m["evidence_file_id"] = n.EvidenceFileID
	}
	if n.CreatedBy != nil {
		m["created_by"], m["created_by_name"] = n.CreatedBy, n.CreatedByName
	}
	if n.ApprovedBy != nil {
		m["approved_by"], m["approved_by_name"] = n.ApprovedBy, n.ApprovedByName
	}
	return m
}

func (h *Strict) BreachListPDPCNotifications(ctx context.Context, req BreachListPDPCNotificationsRequestObject) (BreachListPDPCNotificationsResponseObject, error) {
	list, err := h.svc.ListPDPCNotifications(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListPDPCNotifications200JSONResponse{Data: make([]BreachPDPCNotification, 0, len(list))}
	for _, n := range list {
		var w BreachPDPCNotification
		if err := convert(pdpcMap(n), &w); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	return out, nil
}

func (h *Strict) BreachRecordPDPCNotification(ctx context.Context, req BreachRecordPDPCNotificationRequestObject) (BreachRecordPDPCNotificationResponseObject, error) {
	b := req.Body
	var evidence *uuid.UUID
	if b.EvidenceFileId != nil {
		evidence = b.EvidenceFileId
	}
	ref, late := "", ""
	if b.SubmissionRef != nil {
		ref = *b.SubmissionRef
	}
	if b.LateReason != nil {
		late = *b.LateReason
	}
	n, err := h.svc.CreatePDPCNotification(ctx, req.Id, string(b.NotificationType), b.DocumentVersionId, b.SubmittedAt, ref, evidence, late)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w BreachPDPCNotification
	if err := convert(pdpcMap(n), &w); err != nil {
		return nil, err
	}
	return BreachRecordPDPCNotification201JSONResponse{Body: w, Headers: BreachRecordPDPCNotification201ResponseHeaders{ETag: etag(n.RowVersion)}}, nil
}

func (h *Strict) BreachConfirmPDPCNotification(ctx context.Context, req BreachConfirmPDPCNotificationRequestObject) (BreachConfirmPDPCNotificationResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	n, err := h.svc.ConfirmPDPCNotification(ctx, req.Id, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	var w BreachPDPCNotification
	if err := convert(pdpcMap(n), &w); err != nil {
		return nil, err
	}
	return BreachConfirmPDPCNotification200JSONResponse{Body: w, Headers: BreachConfirmPDPCNotification200ResponseHeaders{ETag: etag(n.RowVersion)}}, nil
}

// ---- notices ----

func noticeVars(v BreachNoticeVars) breach.NoticeVars {
	return breach.NoticeVars{Organization: v.Organization, Summary: v.Summary, Remedy: v.Remedy, Contact: v.Contact}
}

func noticeWire(n breach.Notice) (BreachNotice, error) {
	m := map[string]any{"id": n.ID, "incident_id": n.IncidentID, "channel": n.Channel, "template_code": n.TemplateCode, "variables": n.Vars,
		"total": n.Total, "handed": n.Handed, "failed": n.Failed, "status": n.Status, "delivery": n.Delivery, "row_version": n.RowVersion,
		"created_at": n.CreatedAt.UTC()}
	for k, v := range map[string]*time.Time{"started_at": n.StartedAt, "completed_at": n.CompletedAt, "approved_at": n.ApprovedAt} {
		if v != nil {
			m[k] = v.UTC()
		}
	}
	if n.CreatedBy != nil {
		m["created_by"], m["created_by_name"] = n.CreatedBy, n.CreatedByName
	}
	if n.ApprovedBy != nil {
		m["approved_by"], m["approved_by_name"] = n.ApprovedBy, n.ApprovedByName
	}
	var w BreachNotice
	return w, convert(m, &w)
}

func (h *Strict) BreachListNotices(ctx context.Context, req BreachListNoticesRequestObject) (BreachListNoticesResponseObject, error) {
	list, err := h.svc.Notices(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListNotices200JSONResponse{Data: make([]BreachNotice, 0, len(list))}
	for _, n := range list {
		w, err := noticeWire(n)
		if err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	return out, nil
}

func (h *Strict) BreachCreateNotice(ctx context.Context, req BreachCreateNoticeRequestObject) (BreachCreateNoticeResponseObject, error) {
	n, err := h.svc.CreateNotice(ctx, req.Id, string(req.Body.Channel), noticeVars(req.Body.Variables))
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := noticeWire(n)
	if err != nil {
		return nil, err
	}
	return BreachCreateNotice201JSONResponse{Body: w, Headers: BreachCreateNotice201ResponseHeaders{ETag: etag(n.RowVersion)}}, nil
}

func (h *Strict) BreachGetNotice(ctx context.Context, req BreachGetNoticeRequestObject) (BreachGetNoticeResponseObject, error) {
	n, err := h.svc.GetNotice(ctx, req.Id)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := noticeWire(n)
	if err != nil {
		return nil, err
	}
	return BreachGetNotice200JSONResponse{Body: w, Headers: BreachGetNotice200ResponseHeaders{ETag: etag(n.RowVersion)}}, nil
}

func (h *Strict) BreachUpdateNotice(ctx context.Context, req BreachUpdateNoticeRequestObject) (BreachUpdateNoticeResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	n, err := h.svc.UpdateNotice(ctx, req.Id, v, noticeVars(req.Body.Variables))
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := noticeWire(n)
	if err != nil {
		return nil, err
	}
	return BreachUpdateNotice200JSONResponse{Body: w, Headers: BreachUpdateNotice200ResponseHeaders{ETag: etag(n.RowVersion)}}, nil
}

func (h *Strict) BreachLoadRecipients(ctx context.Context, req BreachLoadRecipientsRequestObject) (BreachLoadRecipientsResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	n, err := h.svc.LoadRecipients(ctx, req.Id, v, req.Body.FileId)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := noticeWire(n)
	if err != nil {
		return nil, err
	}
	return BreachLoadRecipients200JSONResponse{Body: w, Headers: BreachLoadRecipients200ResponseHeaders{ETag: etag(n.RowVersion)}}, nil
}

func (h *Strict) BreachListRecipients(ctx context.Context, req BreachListRecipientsRequestObject) (BreachListRecipientsResponseObject, error) {
	p := req.Params
	status := ""
	if p.Status != nil {
		status = string(*p.Status)
	}
	var after *int32
	if p.AfterLine != nil {
		a := int32(*p.AfterLine)
		after = &a
	}
	limit := 0
	if p.Limit != nil {
		limit = *p.Limit
	}
	list, err := h.svc.Recipients(ctx, req.Id, status, after, limit)
	if err != nil {
		return nil, ToProblem(err)
	}
	out := BreachListRecipients200JSONResponse{Data: make([]BreachRecipient, 0, len(list))}
	for _, r := range list {
		m := map[string]any{"id": r.ID, "line": r.Line, "masked": r.Masked, "language": r.Language, "status": r.Status}
		if r.Delivery != "" {
			m["delivery"], m["attempts"] = r.Delivery, r.Attempts
		}
		if r.SentAt != nil {
			m["sent_at"] = r.SentAt.UTC()
		}
		if r.Error != "" {
			m["error"] = r.Error
		}
		var w BreachRecipient
		if err := convert(m, &w); err != nil {
			return nil, err
		}
		out.Data = append(out.Data, w)
	}
	return out, nil
}

func (h *Strict) BreachSendNotice(ctx context.Context, req BreachSendNoticeRequestObject) (BreachSendNoticeResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	n, err := h.svc.Send(ctx, req.Id, v)
	if err != nil {
		return nil, ToProblem(err)
	}
	w, err := noticeWire(n)
	if err != nil {
		return nil, err
	}
	return BreachSendNotice200JSONResponse{Body: w, Headers: BreachSendNotice200ResponseHeaders{ETag: etag(n.RowVersion)}}, nil
}

// ---- wire ----

func incidentWire(in breach.Incident) (BreachIncident, error) {
	m := map[string]any{"id": in.ID, "incident_no": in.No, "legal_entity_id": in.LegalEntityID, "reported_via": in.ReportedVia, "title": in.Title,
		"description": in.Description, "breach_types": in.BreachTypes, "aware_at": in.AwareAt.UTC(), "affected_category_ids": in.AffectedCategories,
		"status": in.Status, "is_drill": in.IsDrill, "row_version": in.RowVersion, "created_at": in.CreatedAt.UTC(), "updated_at": in.UpdatedAt.UTC(),
		"affected_subjects": in.AffectedSubjects,
		"clock":             map[string]any{"due_at": in.Clock.DueAt.UTC(), "late_by": in.Clock.LateBy.UTC(), "state": in.Clock.State, "hours_elapsed": in.Clock.HoursElapsed}}
	for k, v := range map[string]string{"incident_type": in.IncidentType, "risk_level": in.RiskLevel, "decision": in.Decision,
		"decision_reason": in.DecisionReason, "close_reason": in.CloseReason, "owner_name": in.OwnerName, "reporter_name": in.ReporterName} {
		if v != "" {
			m[k] = v
		}
	}
	for k, v := range map[string]*time.Time{"occurred_at": in.OccurredAt, "contained_at": in.ContainedAt} {
		if v != nil {
			m[k] = v.UTC()
		}
	}
	for k, v := range map[string]*uuid.UUID{"reporter_user_id": in.ReporterID, "owner_user_id": in.OwnerID, "decided_by": in.DecidedBy} {
		if v != nil {
			m[k] = v
		}
	}
	var w BreachIncident
	return w, convert(m, &w)
}

func assessmentMap(a breach.Assessment) map[string]any {
	return map[string]any{"id": a.ID, "form_submission_id": a.FormSubmissionID, "score": a.Score, "risk_level": a.RiskLevel, "factors": a.Factors,
		"assessed_by": a.AssessedBy, "assessed_by_name": a.AssessedByName, "assessed_at": a.AssessedAt.UTC()}
}

func convert(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func etag(v int32) *string {
	s := fmt.Sprintf("%q", strconv.Itoa(int(v)))
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

// ToProblem maps service errors to RFC 9457 problems with stable codes.
func ToProblem(err error) error {
	var ve *breach.ValidationError
	switch {
	case errors.As(err, &ve):
		p := httpx.Problem{Status: http.StatusUnprocessableEntity, Code: "breach.invalid", Title: "Invalid request"}
		for _, f := range ve.Fields {
			p.Errors = append(p.Errors, httpx.FieldError{Field: f.Field, Code: f.Code})
		}
		return p
	case errors.Is(err, breach.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, breach.ErrForbidden):
		return httpx.AuthzDenied()
	case errors.Is(err, breach.ErrSelfApproval):
		return httpx.Problem{Status: http.StatusForbidden, Code: "breach.self_approval", Title: "The maker of a notice can't approve it"}
	case errors.Is(err, breach.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, breach.ErrInvalidTransition):
		return httpx.Problem{Status: http.StatusConflict, Code: "breach.invalid_transition", Title: "Invalid state transition"}
	case errors.Is(err, breach.ErrClosed):
		return httpx.Problem{Status: http.StatusConflict, Code: "breach.closed", Title: "Incident is closed"}
	case errors.Is(err, breach.ErrNoAssessment):
		return httpx.Problem{Status: http.StatusConflict, Code: "breach.no_assessment", Title: "Assess the risk first"}
	case errors.Is(err, breach.ErrPDPCNoticeMissing):
		return httpx.Problem{Status: http.StatusConflict, Code: "breach.pdpc_notice_missing", Title: "The PDPC notice has not been recorded"}
	case errors.Is(err, breach.ErrSubjectNoticeMissing):
		return httpx.Problem{Status: http.StatusConflict, Code: "breach.subject_notice_missing", Title: "The data subject notice has not finished sending"}
	case errors.Is(err, breach.ErrBadDocument):
		return httpx.UnprocessableEntity("breach.bad_document", "Not a published PDPC notification form")
	case errors.Is(err, breach.ErrNoticeNotRequired):
		return httpx.Problem{Status: http.StatusConflict, Code: "breach.notice_not_required", Title: "Data subjects are not to be notified"}
	case errors.Is(err, breach.ErrDecisionTooWeak):
		return httpx.UnprocessableEntity("breach.decision_too_weak", "The decision is weaker than the assessed risk requires")
	case errors.Is(err, breach.ErrBadForm):
		return httpx.UnprocessableEntity("breach.bad_form", "Not a published breach assessment form with none/low/high bands")
	case errors.Is(err, breach.ErrFileNotUsable):
		return httpx.UnprocessableEntity("breach.file_not_usable", "File not found, not yours, not clean or already used")
	}
	return err
}
