// Package iamhttp holds handlers for the iam module. The types and ServerInterface in me.gen.go
// are generated from api/openapi/openapi.yaml (tag "iam") by oapi-codegen — see oapi-codegen.yaml
// and `make gen`. Handlers only translate between the wire format and internal/iam/service; they
// never call internal/iam/store directly (docs/architecture/code-structure.md, "Layer ภายใน module").
package iamhttp

import (
	"context"

	"github.com/google/uuid"

	"pdpa-platform/internal/iam/service"
	"pdpa-platform/internal/pkg/authz"
)

// Strict implements StrictServerInterface for the iam module.
type Strict struct {
	svc *service.Service
}

func NewStrict(svc *service.Service) *Strict {
	return &Strict{svc: svc}
}

var _ StrictServerInterface = (*Strict)(nil)

func (h *Strict) IamGetMe(ctx context.Context, _ IamGetMeRequestObject) (IamGetMeResponseObject, error) {
	grants, _ := authz.FromContext(ctx)

	userID, err := uuid.Parse(grants.UserID)
	if err != nil {
		return IamGetMe401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: UnauthorizedApplicationProblemPlusJSONResponse(newProblem(401, "authn.required", "Authentication required")),
		}, nil
	}

	me, err := h.svc.Me(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := IamGetMe200JSONResponse{
		Id:          me.ID,
		DisplayName: me.DisplayName,
		Locale:      MeLocale(me.Locale),
		Roles:       me.Roles,
		Permissions: me.Permissions,
		MfaEnrolled: me.MFAEnrolled,
	}
	resp.EmailMasked = &me.EmailMasked
	resp.Tenant.Id = me.Tenant.ID
	resp.Tenant.Code = me.Tenant.Code
	resp.Tenant.Name = me.Tenant.Name
	resp.Scopes = make([]DataScope, 0, len(me.Scopes))
	for _, s := range me.Scopes {
		ds := DataScope{ScopeType: DataScopeScopeType(s.ScopeType), IncludeDescendants: &s.IncludeDescendants}
		if s.LegalEntityID != nil {
			id, err := uuid.Parse(*s.LegalEntityID)
			if err == nil {
				ds.LegalEntityId = &id
			}
		}
		if s.OrgUnitID != nil {
			id, err := uuid.Parse(*s.OrgUnitID)
			if err == nil {
				ds.OrgUnitId = &id
			}
		}
		resp.Scopes = append(resp.Scopes, ds)
	}

	return resp, nil
}

func newProblem(status int, code, title string) Problem {
	return Problem{Status: status, Code: code, Title: title}
}
