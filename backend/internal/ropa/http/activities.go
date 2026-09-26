package ropahttp

import (
	"context"

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

func (h *Strict) RopaCreateActivity(ctx context.Context, req RopaCreateActivityRequestObject) (RopaCreateActivityResponseObject, error) {
	a, err := h.svc.SaveActivity(ctx, toActivityInput(*req.Body), 0)
	if err != nil {
		return nil, problem(err)
	}
	return RopaCreateActivity201JSONResponse{Body: toActivityWire(a), Headers: RopaCreateActivity201ResponseHeaders{ETag: etag(a.RowVersion)}}, nil
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
