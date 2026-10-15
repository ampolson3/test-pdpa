// Package providerhttp is IAM-03's own /provider/v1 handlers. Types in provider.gen.go are generated
// from api/openapi/openapi.yaml by oapi-codegen (see oapi-codegen.yaml).
package providerhttp

import (
	"context"
	"errors"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"pdpa-platform/internal/pkg/httpx"
	provider "pdpa-platform/internal/platform/provider"
)

type Strict struct {
	svc *provider.Service
}

func NewStrict(svc *provider.Service) *Strict { return &Strict{svc: svc} }

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) ProviderListTenants(ctx context.Context, _ ProviderListTenantsRequestObject) (ProviderListTenantsResponseObject, error) {
	list, err := h.svc.ListTenants(ctx)
	if err != nil {
		return nil, err
	}
	resp := ProviderListTenants200JSONResponse{Data: make([]ProviderTenant, 0, len(list))}
	for _, t := range list {
		resp.Data = append(resp.Data, toTenantWire(t))
	}
	return resp, nil
}

func (h *Strict) ProviderGetHealth(ctx context.Context, _ ProviderGetHealthRequestObject) (ProviderGetHealthResponseObject, error) {
	health, err := h.svc.Health(ctx)
	if err != nil {
		return nil, err
	}
	return ProviderGetHealth200JSONResponse(toHealthWire(health)), nil
}

func (h *Strict) ProviderCreateTenant(ctx context.Context, req ProviderCreateTenantRequestObject) (ProviderCreateTenantResponseObject, error) {
	t, err := h.svc.CreateTenant(ctx, req.Body.Code, req.Body.Name, req.Body.PlanCode)
	if err != nil {
		return nil, problem(err)
	}
	return ProviderCreateTenant201JSONResponse{Body: toTenantWire(t), Headers: ProviderCreateTenant201ResponseHeaders{ETag: etag(t.RowVersion)}}, nil
}

func (h *Strict) ProviderGetTenant(ctx context.Context, req ProviderGetTenantRequestObject) (ProviderGetTenantResponseObject, error) {
	t, err := h.svc.GetTenant(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	return ProviderGetTenant200JSONResponse{Body: toTenantWire(t), Headers: ProviderGetTenant200ResponseHeaders{ETag: etag(t.RowVersion)}}, nil
}

func (h *Strict) ProviderSuspendTenant(ctx context.Context, req ProviderSuspendTenantRequestObject) (ProviderSuspendTenantResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	t, err := h.svc.SuspendTenant(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return ProviderSuspendTenant200JSONResponse{Body: toTenantWire(t), Headers: ProviderSuspendTenant200ResponseHeaders{ETag: etag(t.RowVersion)}}, nil
}

func (h *Strict) ProviderReactivateTenant(ctx context.Context, req ProviderReactivateTenantRequestObject) (ProviderReactivateTenantResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	t, err := h.svc.ReactivateTenant(ctx, req.Id, v)
	if err != nil {
		return nil, problem(err)
	}
	return ProviderReactivateTenant200JSONResponse{Body: toTenantWire(t), Headers: ProviderReactivateTenant200ResponseHeaders{ETag: etag(t.RowVersion)}}, nil
}

func (h *Strict) ProviderListTenantModules(ctx context.Context, req ProviderListTenantModulesRequestObject) (ProviderListTenantModulesResponseObject, error) {
	list, err := h.svc.ListTenantModules(ctx, req.Id)
	if err != nil {
		return nil, problem(err)
	}
	resp := ProviderListTenantModules200JSONResponse{Data: make([]ProviderTenantModule, 0, len(list))}
	for _, m := range list {
		resp.Data = append(resp.Data, toModuleWire(m))
	}
	return resp, nil
}

func (h *Strict) ProviderSetTenantModule(ctx context.Context, req ProviderSetTenantModuleRequestObject) (ProviderSetTenantModuleResponseObject, error) {
	v, err := parseETag(req.Params.IfMatch)
	if err != nil {
		return nil, httpx.VersionMismatch()
	}
	var licenseExpiresAt *openapi_types.Date
	var quota map[string]any
	if req.Body.LicenseExpiresAt != nil {
		licenseExpiresAt = req.Body.LicenseExpiresAt
	}
	if req.Body.Quota != nil {
		quota = *req.Body.Quota
	}
	m, err := h.svc.SetTenantModule(ctx, req.Id, req.ModuleCode, req.Body.Enabled, dateTimePtr(licenseExpiresAt), quotaJSON(quota), v)
	if err != nil {
		return nil, problem(err)
	}
	return ProviderSetTenantModule200JSONResponse{Body: toModuleWire(m), Headers: ProviderSetTenantModule200ResponseHeaders{ETag: etag(m.RowVersion)}}, nil
}

func (h *Strict) ProviderCreateFirstAdmin(ctx context.Context, req ProviderCreateFirstAdminRequestObject) (ProviderCreateFirstAdminResponseObject, error) {
	userID, err := h.svc.CreateFirstAdmin(ctx, req.Id, string(req.Body.Email), req.Body.DisplayName, req.Body.Roles)
	if err != nil {
		return nil, problem(err)
	}
	return ProviderCreateFirstAdmin201JSONResponse{UserId: userID}, nil
}

func etag(v int32) *string {
	s := strconv.Quote(strconv.Itoa(int(v)))
	return &s
}

func parseETag(h string) (int32, error) {
	h = strings.TrimPrefix(strings.TrimSpace(h), "W/")
	v, err := strconv.ParseInt(strings.Trim(h, `"`), 10, 32)
	return int32(v), err
}

func problem(err error) error {
	switch {
	case errors.Is(err, provider.ErrNotFound):
		return httpx.NotFound()
	case errors.Is(err, provider.ErrVersionMismatch):
		return httpx.VersionMismatch()
	case errors.Is(err, provider.ErrInvalidTransition):
		return httpx.InvalidTransition("provider")
	case errors.Is(err, provider.ErrDuplicateCode):
		return httpx.UnprocessableEntity("provider.duplicate_tenant_code", err.Error())
	case errors.Is(err, provider.ErrAdminAlreadyExists):
		return httpx.Problem{Status: 409, Code: "provider.admin_already_exists", Title: "An active user with this email already exists"}
	case errors.Is(err, provider.ErrInvalid):
		return httpx.UnprocessableEntity("provider.invalid_input", err.Error())
	}
	return err
}
