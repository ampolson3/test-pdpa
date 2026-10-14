// Package provider is IAM-03's own backend: the SUPER-only /provider/v1 console that creates and
// suspends tenants, enables/disables the modules a tenant has actually licensed, and creates a new
// tenant's first admin. decisions.md D-24 (ex Q-19): SUPER accounts live in a reserved tenant,
// `platform` (migration 00064), so AuthZ's ordinary per-tenant grant resolution already works for
// SUPER with no special-casing — only the writes this package itself makes are special.
//
// CLAUDE.md rule 1: platform.tenants and platform.tenant_modules are two of the few tables pdpa_app
// cannot write at all (deploy/db/10-grants.sql) — a SUPER request's own transaction is scoped by RLS
// to the reserved `platform` tenant, which can never legitimately contain another tenant's row. So
// this package, alone among the whole backend, is allowed to hold a second pool authenticating as
// pdpa_platform (BYPASSRLS) and write those two tables directly with it — never inside the request's
// own tenant-scoped transaction, and never used by any other package.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pdb "pdpa-platform/internal/pkg/db"
	auditservice "pdpa-platform/internal/platform/audit/service"
)

var (
	ErrNotFound           = errors.New("provider: not found")
	ErrInvalid            = errors.New("provider: invalid input")
	ErrDuplicateCode      = errors.New("provider: tenant code already exists")
	ErrInvalidTransition  = errors.New("provider: invalid tenant status transition")
	ErrVersionMismatch    = errors.New("provider: version mismatch")
	ErrAdminAlreadyExists = errors.New("provider: an active user with this email already exists in the tenant")
)

// PurchasableModules are the x-module tags (api/openapi/openapi.yaml) that are real, licensable epic
// modules — everything except "iam" (a locked-out tenant must still be able to sign in) and
// "platform" (shared plumbing every module sits on), which internal/pkg/modulegate never gates.
var PurchasableModules = []string{
	"org", "consent", "notice", "breach", "dpia", "ropa", "dpo", "risk", "dsar", "vendor", "agreement", "gov", "dataflow",
}

func isPurchasable(code string) bool {
	for _, m := range PurchasableModules {
		if m == code {
			return true
		}
	}
	return false
}

// Service is the only holder of the pdpa_platform pool in the whole backend (see package doc).
type Service struct {
	Platform *pgxpool.Pool // pdpa_platform (BYPASSRLS) — platform.tenants / platform.tenant_modules
	App      *pgxpool.Pool // pdpa_app — used only via db.WithTenantTx, to create a new tenant's first admin
	Audit    *auditservice.Service
}

type Tenant struct {
	ID          uuid.UUID
	Code        string
	Name        string
	Status      string
	PlanCode    string
	Deployment  string
	DataRegion  string
	SuspendedAt *time.Time
	RowVersion  int32
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (s *Service) scanTenant(row pgx.Row) (Tenant, error) {
	var t Tenant
	if err := row.Scan(&t.ID, &t.Code, &t.Name, &t.Status, &t.PlanCode, &t.Deployment, &t.DataRegion,
		&t.SuspendedAt, &t.RowVersion, &t.CreatedAt, &t.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Tenant{}, ErrNotFound
		}
		return Tenant{}, err
	}
	return t, nil
}

const tenantColumns = "id, code, name, status, plan_code, deployment, data_region, suspended_at, row_version, created_at, updated_at"

// CreateTenant is IAM-03's own "สร้าง tenant": a bare-minimum row — plan/module/quota assignment
// follows as separate SetTenantModule calls, and the tenant's first admin as a separate
// CreateFirstAdmin call, matching the module doc's own description of these as distinct actions.
func (s *Service) CreateTenant(ctx context.Context, code, name, planCode string) (Tenant, error) {
	code, name, planCode = trim(code), trim(name), trim(planCode)
	if code == "" || name == "" || planCode == "" {
		return Tenant{}, fmt.Errorf("%w: code, name and plan_code are required", ErrInvalid)
	}
	row := s.Platform.QueryRow(ctx, `INSERT INTO platform.tenants (code, name, plan_code) VALUES ($1, $2, $3)
		RETURNING `+tenantColumns, code, name, planCode)
	t, err := s.scanTenant(row)
	if err != nil {
		if isUniqueViolation(err) {
			return Tenant{}, ErrDuplicateCode
		}
		return Tenant{}, err
	}
	return t, nil
}

func (s *Service) GetTenant(ctx context.Context, id uuid.UUID) (Tenant, error) {
	row := s.Platform.QueryRow(ctx, `SELECT `+tenantColumns+` FROM platform.tenants WHERE id = $1`, id)
	return s.scanTenant(row)
}

// ListTenants has no pagination (like ROPA-04/DPIA-12's own platform-wide lists): the provider
// console's own tenant roster is the kind of list a platform operator browses whole, not pages of a
// single tenant's own records.
func (s *Service) ListTenants(ctx context.Context) ([]Tenant, error) {
	rows, err := s.Platform.Query(ctx, `SELECT `+tenantColumns+` FROM platform.tenants ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tenant
	for rows.Next() {
		t, err := s.scanTenant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// tenantTransitions is the whole of this feature's own tiny state machine on platform.tenants.status
// (trial/active <-> suspended, either -> terminated is out of scope — no feature needs it yet).
var tenantTransitions = map[string]map[string]bool{
	"trial":     {"suspended": true},
	"active":    {"suspended": true},
	"suspended": {"active": true},
}

func (s *Service) transition(ctx context.Context, id uuid.UUID, to string, rowVersion int32) (Tenant, error) {
	current, err := s.GetTenant(ctx, id)
	if err != nil {
		return Tenant{}, err
	}
	if current.RowVersion != rowVersion {
		return Tenant{}, ErrVersionMismatch
	}
	if !tenantTransitions[current.Status][to] {
		return Tenant{}, ErrInvalidTransition
	}
	var suspendedAtExpr string
	if to == "suspended" {
		suspendedAtExpr = "now()"
	} else {
		suspendedAtExpr = "NULL"
	}
	row := s.Platform.QueryRow(ctx, `UPDATE platform.tenants SET status = $1, suspended_at = `+suspendedAtExpr+`
		WHERE id = $2 AND row_version = $3 RETURNING `+tenantColumns, to, id, rowVersion)
	return s.scanTenant(row)
}

// SuspendTenant is the acceptance criterion's prerequisite half: a suspended tenant's own users can no
// longer sign in (AuthN's own token issuance checks iam.users/tenant status — IAM-02, not this
// package's concern); this package only owns the platform.tenants.status column itself.
func (s *Service) SuspendTenant(ctx context.Context, id uuid.UUID, rowVersion int32) (Tenant, error) {
	return s.transition(ctx, id, "suspended", rowVersion)
}

func (s *Service) ReactivateTenant(ctx context.Context, id uuid.UUID, rowVersion int32) (Tenant, error) {
	return s.transition(ctx, id, "active", rowVersion)
}

type TenantModule struct {
	TenantID         uuid.UUID
	ModuleCode       string
	Enabled          bool
	LicenseExpiresAt *time.Time
	Quota            json.RawMessage
	RowVersion       int32
}

// ListTenantModules is IAM-03's own read side for the console's module toggle list: every
// PurchasableModules code, not just the ones with an explicit row — platform.tenant_modules is
// opt-out (internal/pkg/modulegate treats a missing row as enabled), so a module the tenant has never
// touched must still show up, enabled, with row_version 0 (the same "defaults until first save"
// convention ORG-20's own org_settings already established) so the console's first toggle on it can
// send a real If-Match.
func (s *Service) ListTenantModules(ctx context.Context, tenantID uuid.UUID) ([]TenantModule, error) {
	rows, err := s.Platform.Query(ctx, `SELECT module_code, enabled, license_expires_at, quota, row_version
		FROM platform.tenant_modules WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	existing := map[string]TenantModule{}
	for rows.Next() {
		var m TenantModule
		m.TenantID = tenantID
		if err := rows.Scan(&m.ModuleCode, &m.Enabled, &m.LicenseExpiresAt, &m.Quota, &m.RowVersion); err != nil {
			return nil, err
		}
		existing[m.ModuleCode] = m
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]TenantModule, 0, len(PurchasableModules))
	for _, code := range PurchasableModules {
		if m, ok := existing[code]; ok {
			out = append(out, m)
			continue
		}
		out = append(out, TenantModule{TenantID: tenantID, ModuleCode: code, Enabled: true, Quota: json.RawMessage(`{}`)})
	}
	return out, nil
}

// SetTenantModule is the acceptance criterion's other half: flipping enabled=false here is what
// internal/pkg/modulegate reads on every later request against that tenant+module. rowVersion 0 means
// "I'm setting this for the first time" (ListTenantModules's own virtual default row) and inserts;
// anything else must match the stored row_version exactly, like every other ETag-guarded update in this
// codebase.
func (s *Service) SetTenantModule(ctx context.Context, tenantID uuid.UUID, moduleCode string, enabled bool,
	licenseExpiresAt *time.Time, quota json.RawMessage, rowVersion int32) (TenantModule, error) {
	if !isPurchasable(moduleCode) {
		return TenantModule{}, fmt.Errorf("%w: module_code", ErrInvalid)
	}
	if len(quota) == 0 {
		quota = json.RawMessage(`{}`)
	}
	if _, err := s.GetTenant(ctx, tenantID); err != nil {
		return TenantModule{}, err
	}

	var row pgx.Row
	if rowVersion == 0 {
		// The caller claims there's no row yet (ListTenantModules's own virtual default). A real
		// conflict means that claim was stale — ON CONFLICT DO NOTHING then returns no row, which
		// the scan below reports as ErrVersionMismatch exactly like a failed UPDATE WHERE would.
		row = s.Platform.QueryRow(ctx, `INSERT INTO platform.tenant_modules (tenant_id, module_code, enabled, license_expires_at, quota)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tenant_id, module_code) DO NOTHING
			RETURNING module_code, enabled, license_expires_at, quota, row_version`,
			tenantID, moduleCode, enabled, licenseExpiresAt, quota)
	} else {
		row = s.Platform.QueryRow(ctx, `UPDATE platform.tenant_modules SET enabled = $3, license_expires_at = $4, quota = $5
			WHERE tenant_id = $1 AND module_code = $2 AND row_version = $6
			RETURNING module_code, enabled, license_expires_at, quota, row_version`,
			tenantID, moduleCode, enabled, licenseExpiresAt, quota, rowVersion)
	}
	var m TenantModule
	m.TenantID = tenantID
	err := row.Scan(&m.ModuleCode, &m.Enabled, &m.LicenseExpiresAt, &m.Quota, &m.RowVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TenantModule{}, ErrVersionMismatch
		}
		return TenantModule{}, err
	}
	return m, nil
}

// CreateFirstAdmin is the module doc's own "สร้างผู้ดูแล tenant คนแรก": a DB-only iam.users row plus a
// role_assignments grant, following cmd/devseed's exact precedent — no real Keycloak account, since
// Keycloak isn't available in this environment (the same gap IAM-01 itself is blocked on). It runs
// through s.App (pdpa_app), not s.Platform: this write is entirely inside the new tenant's own RLS
// scope, so it uses the ordinary tenant-transaction path every other module uses, never the bypass
// pool.
func (s *Service) CreateFirstAdmin(ctx context.Context, tenantID uuid.UUID, email, displayName string, roleCodes []string) (uuid.UUID, error) {
	email, displayName = trim(email), trim(displayName)
	if email == "" || displayName == "" || len(roleCodes) == 0 {
		return uuid.Nil, fmt.Errorf("%w: email, display_name and at least one role are required", ErrInvalid)
	}
	if _, err := s.GetTenant(ctx, tenantID); err != nil {
		return uuid.Nil, err
	}

	var userID uuid.UUID
	err := pdb.WithTenantTx(ctx, s.App, tenantID.String(), "", func(ctx context.Context) error {
		tx := pdb.MustTxFromContext(ctx)
		var existing int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM iam.users WHERE tenant_id = $1 AND email = $2 AND status != 'disabled'`,
			tenantID, email).Scan(&existing); err != nil {
			return err
		}
		if existing > 0 {
			return ErrAdminAlreadyExists
		}
		if err := tx.QueryRow(ctx, `INSERT INTO iam.users (tenant_id, email, display_name, status)
			VALUES ($1, $2, $3, 'active') RETURNING id`, tenantID, email, displayName).Scan(&userID); err != nil {
			return err
		}
		for _, code := range roleCodes {
			var roleID uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT id FROM iam.roles WHERE code = $1 AND tenant_id IS NULL`, code).Scan(&roleID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("%w: role %s", ErrInvalid, code)
				}
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO iam.role_assignments (tenant_id, user_id, role_id, scope_type)
				VALUES ($1, $2, $3, 'tenant')`, tenantID, userID, roleID); err != nil {
				return err
			}
		}
		if s.Audit != nil {
			if err := s.Audit.Write(ctx, auditservice.Entry{TenantID: tenantID, ActorType: "system",
				Action: "provider.tenant.create_first_admin", EntityType: "iam_user", EntityID: &userID}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return userID, nil
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
