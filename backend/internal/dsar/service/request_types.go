package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	dsarstore "pdpa-platform/internal/dsar/store"
	iamservice "pdpa-platform/internal/iam/service"
	pdb "pdpa-platform/internal/pkg/db"
)

// RequestType is one of the 9 fixed DSAR right types (dsar.requests.request_type_id CHECK), seeded globally
// (migration 00043) since the set of codes is fixed by the DDL itself, not tenant-editable master data.
// DSAR-20's own configuration columns — DefaultSubtaskAction/DefaultAssigneeUserID/ResponseCode* — are
// tenant-editable (nil means "use today's hardcoded default"), read fresh by CreateRequest/Transition.
type RequestType struct {
	ID                      uuid.UUID
	Code                    string
	NameTh                  string
	NameEn                  string
	LegalRef                string
	SLADays                 int16
	DefaultSubtaskAction    *string
	DefaultAssigneeUserID   *uuid.UUID
	ResponseCodeResult      *string
	ResponseCodeRejection   *string
	ResponseCodeRequestInfo *string
	RowVersion              int32
}

// DSAR08SubtaskActions is the fixed vocabulary DSAR-08/DSAR-03 already seeded for a subtask's own action
// column — the only values DSAR-20's own DefaultSubtaskAction override may take.
var DSAR08SubtaskActions = []string{"search", "export", "stop_marketing", "delete", "restrict", "rectify", "review"}

func validSubtaskAction(a string) bool {
	for _, x := range DSAR08SubtaskActions {
		if x == a {
			return true
		}
	}
	return false
}

// ListRequestTypes returns one row per fixed code — the tenant's own configured row (DSAR-20) when one
// exists, else the global default — since RLS never lets a tenant edit the global row directly (its own
// tenant_id is NULL, never the caller's), a tenant that configures a type gets its own row sharing that
// code instead (uq_request_types_code allows one row per (tenant_id, code)).
func (s *Service) ListRequestTypes(ctx context.Context) ([]RequestType, error) {
	rows, err := dsarstore.New(pdb.MustTxFromContext(ctx)).ListRequestTypes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RequestType, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRequestType(dsarstore.GetEffectiveRequestTypeByCodeRow(r)))
	}
	return out, nil
}

// GetRequestType resolves the *effective* row for whichever id is given — a request's own
// request_type_id may point at either the global row or (once configured) the tenant's own override row
// for the same code, and both must read back the tenant's current configuration identically.
func (s *Service) GetRequestType(ctx context.Context, id uuid.UUID) (RequestType, error) {
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	code, err := q.GetRequestTypeCode(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return RequestType{}, ErrNotFound
	}
	if err != nil {
		return RequestType{}, err
	}
	r, err := q.GetEffectiveRequestTypeByCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return RequestType{}, ErrNotFound
	}
	if err != nil {
		return RequestType{}, err
	}
	return toRequestType(r), nil
}

func toRequestType(r dsarstore.GetEffectiveRequestTypeByCodeRow) RequestType {
	rt := RequestType{ID: r.ID, Code: r.Code, NameTh: r.NameTh, NameEn: deref(r.NameEn), LegalRef: deref(r.LegalRef), SLADays: r.SlaDays,
		DefaultSubtaskAction: r.DefaultSubtaskAction, ResponseCodeResult: r.ResponseCodeResult,
		ResponseCodeRejection: r.ResponseCodeRejection, ResponseCodeRequestInfo: r.ResponseCodeRequestInfo, RowVersion: r.RowVersion}
	if r.DefaultAssigneeUserID.Valid {
		u := uuid.UUID(r.DefaultAssigneeUserID.Bytes)
		rt.DefaultAssigneeUserID = &u
	}
	return rt
}

// responseCodeOverride is the configured override for one letter purpose (result/rejection/request_info),
// or nil when this type uses the shared default for that purpose.
func (rt RequestType) responseCodeOverride(purpose string) *string {
	switch purpose {
	case "result":
		return rt.ResponseCodeResult
	case "rejection":
		return rt.ResponseCodeRejection
	case "request_info":
		return rt.ResponseCodeRequestInfo
	}
	return nil
}

// RequestTypeConfigInput is DSAR-20's own write: steps (DefaultSubtaskAction), responsible
// (DefaultAssigneeUserID) and the response-letter override code per purpose — a nil field clears that
// override back to today's hardcoded default, never leaves the prior value untouched (the whole config is
// replaced each save, the same full-replace convention this codebase's other settings forms already use).
type RequestTypeConfigInput struct {
	DefaultSubtaskAction    *string
	DefaultAssigneeUserID   *uuid.UUID
	ResponseCodeResult      *string
	ResponseCodeRejection   *string
	ResponseCodeRequestInfo *string
}

// SaveRequestTypeConfig is the acceptance criterion itself: CreateRequest/Transition/generateResponseLetter
// all call GetRequestType fresh on every call, never a cached value, so a DPO's own save here changes
// behaviour for the very next new request of this type with no redeploy. The first save for a code creates
// the tenant's own row (copying the global default's name/legal_ref/sla_days once, to satisfy this table's
// own NOT NULL columns); every later save for the same code is an in-place update of that same row.
func (s *Service) SaveRequestTypeConfig(ctx context.Context, code string, in RequestTypeConfigInput) (RequestType, error) {
	if in.DefaultSubtaskAction != nil && !validSubtaskAction(*in.DefaultSubtaskAction) {
		return RequestType{}, fmt.Errorf("%w: default_subtask_action", ErrInvalid)
	}
	if in.DefaultAssigneeUserID != nil {
		names, err := iamservice.Names(ctx, []uuid.UUID{*in.DefaultAssigneeUserID})
		if err != nil {
			return RequestType{}, err
		}
		if _, ok := names[*in.DefaultAssigneeUserID]; !ok {
			return RequestType{}, fmt.Errorf("%w: default_assignee_user_id", ErrInvalid)
		}
	}
	q := dsarstore.New(pdb.MustTxFromContext(ctx))
	defaults, err := q.GetRequestTypeDefaultsByCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return RequestType{}, fmt.Errorf("%w: code", ErrInvalid)
	}
	if err != nil {
		return RequestType{}, err
	}
	before, err := q.GetEffectiveRequestTypeByCode(ctx, code)
	if err != nil {
		return RequestType{}, err
	}
	row, err := q.UpsertRequestTypeConfig(ctx, dsarstore.UpsertRequestTypeConfigParams{
		Code: code, NameTh: defaults.NameTh, NameEn: defaults.NameEn, LegalRef: defaults.LegalRef, SlaDays: defaults.SlaDays,
		DefaultSubtaskAction: in.DefaultSubtaskAction, DefaultAssigneeUserID: pgUUID(in.DefaultAssigneeUserID),
		ResponseCodeResult: in.ResponseCodeResult, ResponseCodeRejection: in.ResponseCodeRejection, ResponseCodeRequestInfo: in.ResponseCodeRequestInfo,
	})
	if err != nil {
		return RequestType{}, err
	}
	out := toRequestType(dsarstore.GetEffectiveRequestTypeByCodeRow(row))
	return out, s.audit(ctx, "dsar.request_type.config", out.ID,
		map[string]any{"default_subtask_action": before.DefaultSubtaskAction, "default_assignee_user_id": before.DefaultAssigneeUserID},
		map[string]any{"default_subtask_action": out.DefaultSubtaskAction, "default_assignee_user_id": out.DefaultAssigneeUserID})
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
