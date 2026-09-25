package httpx

import "context"

// Principal is who's making the request, as established by the AuthN (#7) and Tenant (#8)
// middlewares before AuthZ (#9) or Tx (#11) run.
type Principal struct {
	UserID     string // iam.users.id, "" for guest/data-subject/system principals not backed by a user row
	TenantID   string
	ActorType  string // user | api_client | guest | data_subject | system — platform.audit_log.actor_type
	MFAAcrHigh bool   // true if the token's acr indicates step-up/MFA was performed this session
}

type principalCtxKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalCtxKey{}, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalCtxKey{}).(Principal)
	return p, ok
}
