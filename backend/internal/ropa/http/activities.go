package ropahttp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"pdpa-platform/internal/platform/forms"
	ropaservice "pdpa-platform/internal/ropa/service"

	"pdpa-platform/internal/pkg/httpx"
)

func (h *Strict) RopaListActivities(ctx context.Context, req RopaListActivitiesRequestObject) (RopaListActivitiesResponseObject, error) {
	f := ropaservice.ActivityFilter{}
	if req.Params.OrgUnitId != nil {
		f.OrgUnitID = req.Params.OrgUnitId
	}
	if req.Params.Status != nil {
		f.Status = string(*req.Params.Status)
	}
	if req.Params.Q != nil {
		f.Query = *req.Params.Q
	}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}
	if req.Params.Cursor != nil {
		at, id, err := decodeCursor(*req.Params.Cursor)
		if err != nil {
			return nil, httpx.RequestInvalid("cursor")
		}
		f.After = &ropaservice.ActivityCursor{CreatedAt: at, ID: id}
	}
	list, next, err := h.svc.ListActivities(ctx, f)
	if err != nil {
		return nil, err
	}
	resp := RopaListActivities200JSONResponse{Data: make([]ProcessingActivity, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toActivityWire(a))
	}
	if next != nil {
		c := encodeCursor(next.CreatedAt, next.ID)
		resp.NextCursor = &c
	}
	return resp, nil
}

func (h *Strict) RopaExportProcessorActivities(ctx context.Context, _ RopaExportProcessorActivitiesRequestObject) (RopaExportProcessorActivitiesResponseObject, error) {
	buf, _, err := h.svc.ExportProcessorActivities(ctx)
	if err != nil {
		return nil, problem(err)
	}
	name := fmt.Sprintf(`attachment; filename="ropa-processor-%s.csv"`, time.Now().UTC().Format("20060102-150405"))
	return RopaExportProcessorActivities200TextcsvResponse{Body: buf, ContentLength: int64(buf.Len()),
		Headers: RopaExportProcessorActivities200ResponseHeaders{ContentDisposition: &name}}, nil
}

func (h *Strict) RopaCreateActivity(ctx context.Context, req RopaCreateActivityRequestObject) (RopaCreateActivityResponseObject, error) {
	a, err := h.svc.SaveActivity(ctx, toActivityInput(*req.Body), 0)
	if err != nil {
		return nil, problem(err)
	}
	return RopaCreateActivity201JSONResponse{Body: toActivityWire(a), Headers: RopaCreateActivity201ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaCreateActivityFromTemplate(ctx context.Context, req RopaCreateActivityFromTemplateRequestObject) (RopaCreateActivityFromTemplateResponseObject, error) {
	b := *req.Body
	a, err := h.svc.CreateActivityFromTemplate(ctx, b.ActivityTemplateId, b.LegalEntityId, b.OrgUnitId, b.Code, b.OwnerUserId)
	if err != nil {
		return nil, problem(err)
	}
	return RopaCreateActivityFromTemplate201JSONResponse{Body: toActivityWire(a), Headers: RopaCreateActivityFromTemplate201ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaCreateActivitiesFromTemplates(ctx context.Context, req RopaCreateActivitiesFromTemplatesRequestObject) (RopaCreateActivitiesFromTemplatesResponseObject, error) {
	b := *req.Body
	list, err := h.svc.CreateActivitiesFromTemplates(ctx, b.OrgUnitId, b.ActivityTemplateIds, b.OwnerUserId)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaCreateActivitiesFromTemplates201JSONResponse{Data: make([]ProcessingActivity, 0, len(list))}
	for _, a := range list {
		resp.Data = append(resp.Data, toActivityWire(a))
	}
	return resp, nil
}

func (h *Strict) RopaCreateActivityFromWizard(ctx context.Context, req RopaCreateActivityFromWizardRequestObject) (RopaCreateActivityFromWizardResponseObject, error) {
	b := *req.Body
	in := ropaservice.WizardInput{
		LegalEntityID: b.LegalEntityId, OrgUnitID: b.OrgUnitId, Code: b.Code, Name: b.Name, OwnerUserID: b.OwnerUserId,
		Purpose: b.Purpose, LegalReason: ropaservice.WizardLegalReason(b.LegalReason), ConsentPurposeID: b.ConsentPurposeId,
		SubjectTypeID: b.SubjectTypeId, DataCategoryIDs: b.DataCategoryIds, Retention: ropaservice.WizardRetentionChoice(b.Retention),
		RecipientPartyID: b.RecipientPartyId,
	}
	if b.RecipientKind != nil {
		in.RecipientKind = ropaservice.WizardRecipientKind(*b.RecipientKind)
	}
	if b.RecipientNote != nil {
		in.RecipientNote = *b.RecipientNote
	}
	a, err := h.svc.CreateActivityFromWizard(ctx, in)
	if err != nil {
		return nil, problem(err)
	}
	return RopaCreateActivityFromWizard201JSONResponse{Body: toActivityWire(a), Headers: RopaCreateActivityFromWizard201ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaApplySuggestedItems(ctx context.Context, req RopaApplySuggestedItemsRequestObject) (RopaApplySuggestedItemsResponseObject, error) {
	b := *req.Body
	in := ropaservice.ApplySuggestedItemsInput{TemplateID: b.ActivityTemplateId}
	if b.Purposes != nil {
		in.Purposes = *b.Purposes
	}
	if b.Data != nil {
		in.Data = *b.Data
	}
	if b.Retention != nil {
		in.Retention = *b.Retention
	}
	a, err := h.svc.ApplySuggestedItems(ctx, req.Id, in)
	if err != nil {
		return nil, problem(err)
	}
	return RopaApplySuggestedItems200JSONResponse(toActivityWire(a)), nil
}

func (h *Strict) RopaGetActivity(ctx context.Context, req RopaGetActivityRequestObject) (RopaGetActivityResponseObject, error) {
	a, err := h.svc.GetActivity(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return RopaGetActivity200JSONResponse{Body: toActivityWire(a), Headers: RopaGetActivity200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaUpdateActivity(ctx context.Context, req RopaUpdateActivityRequestObject) (RopaUpdateActivityResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	in := toActivityInput(*req.Body)
	in.ID = req.Id
	a, err := h.svc.SaveActivity(ctx, in, v)
	if err != nil {
		return nil, problem(err)
	}
	return RopaUpdateActivity200JSONResponse{Body: toActivityWire(a), Headers: RopaUpdateActivity200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaSubmitActivity(ctx context.Context, req RopaSubmitActivityRequestObject) (RopaSubmitActivityResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	a, err := h.svc.SubmitActivity(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return RopaSubmitActivity200JSONResponse{Body: toActivityWire(a), Headers: RopaSubmitActivity200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaApproveActivity(ctx context.Context, req RopaApproveActivityRequestObject) (RopaApproveActivityResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	a, err := h.svc.ApproveActivity(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return RopaApproveActivity200JSONResponse{Body: toActivityWire(a), Headers: RopaApproveActivity200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaReturnActivity(ctx context.Context, req RopaReturnActivityRequestObject) (RopaReturnActivityResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	var reason string
	if req.Body != nil && req.Body.Reason != nil {
		reason = *req.Body.Reason
	}
	a, err := h.svc.ReturnActivity(ctx, req.Id, v, reason)
	if err != nil {
		return nil, problem(err)
	}
	return RopaReturnActivity200JSONResponse{Body: toActivityWire(a), Headers: RopaReturnActivity200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaEndActivity(ctx context.Context, req RopaEndActivityRequestObject) (RopaEndActivityResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	var reason string
	if req.Body != nil {
		reason = req.Body.Reason
	}
	a, err := h.svc.EndActivity(ctx, req.Id, v, reason)
	if err != nil {
		return nil, problem(err)
	}
	return RopaEndActivity200JSONResponse{Body: toActivityWire(a), Headers: RopaEndActivity200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaRequestActivityReview(ctx context.Context, req RopaRequestActivityReviewRequestObject) (RopaRequestActivityReviewResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	a, err := h.svc.RequestActivityReview(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return RopaRequestActivityReview200JSONResponse{Body: toActivityWire(a), Headers: RopaRequestActivityReview200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaConfirmActivityNoChange(ctx context.Context, req RopaConfirmActivityNoChangeRequestObject) (RopaConfirmActivityNoChangeResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	a, err := h.svc.ConfirmActivityNoChange(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return RopaConfirmActivityNoChange200JSONResponse{Body: toActivityWire(a), Headers: RopaConfirmActivityNoChange200ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
}

func (h *Strict) RopaListActivityVersions(ctx context.Context, req RopaListActivityVersionsRequestObject) (RopaListActivityVersionsResponseObject, error) {
	list, err := h.svc.ListActivityVersions(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListActivityVersions200JSONResponse{Data: make([]ActivityVersion, 0, len(list))}
	for _, v := range list {
		resp.Data = append(resp.Data, toActivityVersionWire(v))
	}
	return resp, nil
}

func (h *Strict) RopaCompareActivityVersions(ctx context.Context, req RopaCompareActivityVersionsRequestObject) (RopaCompareActivityVersionsResponseObject, error) {
	diffs, err := h.svc.CompareActivityVersions(ctx, req.Params.From, req.Params.To)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaCompareActivityVersions200JSONResponse{Data: make([]ActivityVersionDiff, 0, len(diffs))}
	for _, d := range diffs {
		resp.Data = append(resp.Data, ActivityVersionDiff{Section: d.Section, Before: d.Before, After: d.After})
	}
	return resp, nil
}

func toActivityVersionWire(v ropaservice.ActivityVersion) ActivityVersion {
	out := ActivityVersion{Id: v.ID, ActivityId: v.ActivityID, VersionNo: int(v.VersionNo), CreatedAt: v.CreatedAt}
	raw, err := json.Marshal(v.Snapshot)
	if err == nil {
		var m map[string]interface{}
		if json.Unmarshal(raw, &m) == nil {
			out.Snapshot = m
		}
	}
	if v.ApprovedBy != nil {
		id := *v.ApprovedBy
		out.ApprovedBy = &id
	}
	if !v.ApprovedAt.IsZero() {
		t := v.ApprovedAt
		out.ApprovedAt = &t
	}
	return out
}

func (h *Strict) RopaListActivityPurposes(ctx context.Context, req RopaListActivityPurposesRequestObject) (RopaListActivityPurposesResponseObject, error) {
	list, err := h.svc.ListActivityPurposes(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListActivityPurposes200JSONResponse{Data: make([]ActivityPurpose, 0, len(list))}
	for _, p := range list {
		resp.Data = append(resp.Data, toPurposeWire(p))
	}
	return resp, nil
}

func (h *Strict) RopaAddActivityPurpose(ctx context.Context, req RopaAddActivityPurposeRequestObject) (RopaAddActivityPurposeResponseObject, error) {
	b := *req.Body
	p := ropaservice.ActivityPurpose{ActivityID: req.Id, PurposeID: b.PurposeId, PurposeText: b.PurposeText,
		LawfulBasisCode: b.LawfulBasisCode, ConsentPurposeID: b.ConsentPurposeId}
	out, err := h.svc.AddActivityPurpose(ctx, p)
	if err != nil {
		return nil, problem(err)
	}
	return RopaAddActivityPurpose201JSONResponse(toPurposeWire(out)), nil
}

func (h *Strict) RopaDeleteActivityPurpose(ctx context.Context, req RopaDeleteActivityPurposeRequestObject) (RopaDeleteActivityPurposeResponseObject, error) {
	if err := h.svc.DeleteActivityPurpose(ctx, req.Id, req.PurposeId); err != nil {
		return nil, problem(err)
	}
	return RopaDeleteActivityPurpose204Response{}, nil
}

func (h *Strict) RopaListActivityData(ctx context.Context, req RopaListActivityDataRequestObject) (RopaListActivityDataResponseObject, error) {
	list, err := h.svc.ListActivityData(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListActivityData200JSONResponse{Data: make([]ActivityData, 0, len(list))}
	for _, d := range list {
		resp.Data = append(resp.Data, toActivityDataWire(d))
	}
	return resp, nil
}

func (h *Strict) RopaAddActivityData(ctx context.Context, req RopaAddActivityDataRequestObject) (RopaAddActivityDataResponseObject, error) {
	b := *req.Body
	d := ropaservice.ActivityData{ActivityID: req.Id, DataCategoryID: b.DataCategoryId, SubjectTypeID: b.SubjectTypeId,
		Source: string(b.Source), SourcePartyID: b.SourcePartyId}
	if b.VolumeBand != nil {
		d.VolumeBand = string(*b.VolumeBand)
	}
	out, err := h.svc.AddActivityData(ctx, d)
	if err != nil {
		return nil, problem(err)
	}
	return RopaAddActivityData201JSONResponse(toActivityDataWire(out)), nil
}

func (h *Strict) RopaDeleteActivityData(ctx context.Context, req RopaDeleteActivityDataRequestObject) (RopaDeleteActivityDataResponseObject, error) {
	if err := h.svc.DeleteActivityData(ctx, req.Id, req.DataId); err != nil {
		return nil, problem(err)
	}
	return RopaDeleteActivityData204Response{}, nil
}

func (h *Strict) RopaListRetentionRules(ctx context.Context, req RopaListRetentionRulesRequestObject) (RopaListRetentionRulesResponseObject, error) {
	list, err := h.svc.ListRetentionRules(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListRetentionRules200JSONResponse{Data: make([]RetentionRule, 0, len(list))}
	for _, r := range list {
		resp.Data = append(resp.Data, toRetentionWire(r))
	}
	return resp, nil
}

func (h *Strict) RopaAddRetentionRule(ctx context.Context, req RopaAddRetentionRuleRequestObject) (RopaAddRetentionRuleResponseObject, error) {
	b := *req.Body
	var months *int
	if b.RetentionMonths != nil {
		m := *b.RetentionMonths
		months = &m
	}
	r := ropaservice.RetentionRule{ActivityID: req.Id, DataCategoryID: b.DataCategoryId, RetentionMonths: months,
		RetentionBasis: b.RetentionBasis, TriggerEvent: b.TriggerEvent, DisposalMethod: string(b.DisposalMethod)}
	out, err := h.svc.AddRetentionRule(ctx, r)
	if err != nil {
		return nil, problem(err)
	}
	return RopaAddRetentionRule201JSONResponse(toRetentionWire(out)), nil
}

func (h *Strict) RopaDeleteRetentionRule(ctx context.Context, req RopaDeleteRetentionRuleRequestObject) (RopaDeleteRetentionRuleResponseObject, error) {
	if err := h.svc.DeleteRetentionRule(ctx, req.Id, req.RuleId); err != nil {
		return nil, problem(err)
	}
	return RopaDeleteRetentionRule204Response{}, nil
}

func (h *Strict) RopaListActivityRecipients(ctx context.Context, req RopaListActivityRecipientsRequestObject) (RopaListActivityRecipientsResponseObject, error) {
	list, err := h.svc.ListActivityRecipients(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListActivityRecipients200JSONResponse{Data: make([]ActivityRecipient, 0, len(list))}
	for _, r := range list {
		resp.Data = append(resp.Data, toRecipientWire(r))
	}
	return resp, nil
}

func (h *Strict) RopaAddActivityRecipient(ctx context.Context, req RopaAddActivityRecipientRequestObject) (RopaAddActivityRecipientResponseObject, error) {
	b := *req.Body
	r := ropaservice.ActivityRecipient{ActivityID: req.Id, PartyID: b.PartyId, RecipientRole: string(b.RecipientRole)}
	if b.DisclosureBasis != nil {
		r.DisclosureBasis = *b.DisclosureBasis
	}
	if b.DataCategoryIds != nil {
		r.DataCategoryIDs = *b.DataCategoryIds
	}
	out, err := h.svc.AddActivityRecipient(ctx, r)
	if err != nil {
		return nil, problem(err)
	}
	return RopaAddActivityRecipient201JSONResponse(toRecipientWire(out)), nil
}

func (h *Strict) RopaDeleteActivityRecipient(ctx context.Context, req RopaDeleteActivityRecipientRequestObject) (RopaDeleteActivityRecipientResponseObject, error) {
	if err := h.svc.DeleteActivityRecipient(ctx, req.Id, req.RecipientId); err != nil {
		return nil, problem(err)
	}
	return RopaDeleteActivityRecipient204Response{}, nil
}

func (h *Strict) RopaListActivityTransfers(ctx context.Context, req RopaListActivityTransfersRequestObject) (RopaListActivityTransfersResponseObject, error) {
	list, err := h.svc.ListActivityTransfers(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := RopaListActivityTransfers200JSONResponse{Data: make([]ActivityTransfer, 0, len(list))}
	for _, t := range list {
		resp.Data = append(resp.Data, toTransferWire(t))
	}
	return resp, nil
}

func (h *Strict) RopaAddActivityTransfer(ctx context.Context, req RopaAddActivityTransferRequestObject) (RopaAddActivityTransferResponseObject, error) {
	b := *req.Body
	tr := ropaservice.ActivityTransfer{ActivityID: req.Id, RecipientID: b.RecipientId, CountryCode: b.CountryCode, TransferBasis: string(b.TransferBasis)}
	if b.Safeguards != nil {
		tr.Safeguards = *b.Safeguards
	}
	out, err := h.svc.AddActivityTransfer(ctx, tr)
	if err != nil {
		return nil, problem(err)
	}
	return RopaAddActivityTransfer201JSONResponse(toTransferWire(out)), nil
}

func (h *Strict) RopaDeleteActivityTransfer(ctx context.Context, req RopaDeleteActivityTransferRequestObject) (RopaDeleteActivityTransferResponseObject, error) {
	if err := h.svc.DeleteActivityTransfer(ctx, req.Id, req.TransferId); err != nil {
		return nil, problem(err)
	}
	return RopaDeleteActivityTransfer204Response{}, nil
}

func (h *Strict) RopaRecordTransferTia(ctx context.Context, req RopaRecordTransferTiaRequestObject) (RopaRecordTransferTiaResponseObject, error) {
	out, err := h.svc.RecordTransferAssessment(ctx, req.Id, req.TransferId, req.Body.TemplateCode, forms.Answers(req.Body.Answers))
	if err != nil {
		return nil, problem(err)
	}
	return RopaRecordTransferTia200JSONResponse(toTransferWire(out)), nil
}

func toTransferWire(tr ropaservice.ActivityTransfer) ActivityTransfer {
	return ActivityTransfer{Id: tr.ID, ActivityId: tr.ActivityID, RecipientId: tr.RecipientID, CountryCode: tr.CountryCode,
		TransferBasis: ActivityTransferBasis(tr.TransferBasis), Safeguards: ptr(tr.Safeguards), TiaAssessmentId: tr.TiaAssessmentID,
		RowVersion: int(tr.RowVersion), CreatedAt: tr.CreatedAt.UTC()}
}

func toActivityInput(b ProcessingActivityInput) ropaservice.Activity {
	a := ropaservice.Activity{LegalEntityID: b.LegalEntityId, OrgUnitID: b.OrgUnitId, Code: b.Code, Name: b.Name,
		Role: string(b.Role), ControllerPartyID: b.ControllerPartyId, OwnerUserID: b.OwnerUserId}
	if b.Description != nil {
		a.Description = *b.Description
	}
	if b.RightsAndAccess != nil {
		a.RightsAndAccess = *b.RightsAndAccess
	}
	return a
}

func toActivityWire(a ropaservice.Activity) ProcessingActivity {
	w := ProcessingActivity{Id: a.ID, LegalEntityId: a.LegalEntityID, OrgUnitId: a.OrgUnitID, Code: a.Code, Name: a.Name,
		Description: ptr(a.Description), Role: ActivityRole(a.Role), ControllerPartyId: a.ControllerPartyID, OwnerUserId: a.OwnerUserID,
		RightsAndAccess: ptr(a.RightsAndAccess), Status: ActivityStatus(a.Status), Completeness: a.Completeness,
		RowVersion: int(a.RowVersion), UpdatedAt: a.UpdatedAt.UTC()}
	if a.MissingItems != nil {
		items := make([]ProcessingActivityMissingItems, 0, len(a.MissingItems))
		for _, m := range a.MissingItems {
			items = append(items, ProcessingActivityMissingItems(m))
		}
		w.MissingItems = &items
	}
	if a.EndedAt != nil {
		t := a.EndedAt.UTC()
		w.EndedAt = &t
	}
	w.EndReason = a.EndReason
	return w
}

func toPurposeWire(p ropaservice.ActivityPurpose) ActivityPurpose {
	return ActivityPurpose{Id: p.ID, ActivityId: p.ActivityID, PurposeId: p.PurposeID, PurposeText: p.PurposeText,
		LawfulBasisCode: p.LawfulBasisCode, ConsentPurposeId: p.ConsentPurposeID, RowVersion: int(p.RowVersion), CreatedAt: p.CreatedAt.UTC()}
}

func toActivityDataWire(d ropaservice.ActivityData) ActivityData {
	w := ActivityData{Id: d.ID, ActivityId: d.ActivityID, DataCategoryId: d.DataCategoryID, SubjectTypeId: d.SubjectTypeID,
		Source: ActivityDataSource(d.Source), SourcePartyId: d.SourcePartyID, IsSensitive: d.IsSensitive,
		RowVersion: int(d.RowVersion), CreatedAt: d.CreatedAt.UTC()}
	if d.VolumeBand != "" {
		v := ActivityVolumeBand(d.VolumeBand)
		w.VolumeBand = &v
	}
	return w
}

func toRetentionWire(r ropaservice.RetentionRule) RetentionRule {
	return RetentionRule{Id: r.ID, ActivityId: r.ActivityID, DataCategoryId: r.DataCategoryID, RetentionMonths: r.RetentionMonths,
		RetentionBasis: r.RetentionBasis, TriggerEvent: r.TriggerEvent, DisposalMethod: ActivityDisposalMethod(r.DisposalMethod),
		RowVersion: int(r.RowVersion), CreatedAt: r.CreatedAt.UTC()}
}

func toRecipientWire(r ropaservice.ActivityRecipient) ActivityRecipient {
	w := ActivityRecipient{Id: r.ID, ActivityId: r.ActivityID, PartyId: r.PartyID, RecipientRole: ActivityRecipientRole(r.RecipientRole),
		DisclosureBasis: ptr(r.DisclosureBasis), RowVersion: int(r.RowVersion), CreatedAt: r.CreatedAt.UTC()}
	if len(r.DataCategoryIDs) > 0 {
		ids := r.DataCategoryIDs
		w.DataCategoryIds = &ids
	}
	return w
}
