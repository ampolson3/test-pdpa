-- DSAR-20: a tenant's own configuration (default_subtask_action/default_assignee_user_id/response_code_*)
-- can never be written onto the global seeded row (migration 00043, tenant_id IS NULL) — RLS's own
-- tenant_write policy only allows writing a row whose tenant_id is the caller's own. So a tenant that
-- configures a right type gets its own row sharing that type's code (uq_request_types_code already allows
-- one row per (tenant_id, code), including tenant_id NULL for the global default); the "effective" row for
-- a code is always the tenant's own if one exists, else the global default — ListRequestTypes/GetRequestType
-- resolve that preference so every other caller (CreateRequest, Transition, the intake picker) just sees one
-- row per code with no special-casing of its own.

-- name: ListRequestTypes :many
SELECT DISTINCT ON (code) id, code, name_th, name_en, legal_ref, sla_days, default_subtask_action, default_assignee_user_id,
    response_code_result, response_code_rejection, response_code_request_info, row_version
FROM dsar.request_types ORDER BY code, tenant_id NULLS LAST;

-- name: GetRequestTypeCode :one
SELECT code FROM dsar.request_types WHERE id = $1;

-- name: GetEffectiveRequestTypeByCode :one
SELECT id, code, name_th, name_en, legal_ref, sla_days, default_subtask_action, default_assignee_user_id,
    response_code_result, response_code_rejection, response_code_request_info, row_version
FROM dsar.request_types WHERE code = $1 ORDER BY tenant_id NULLS LAST LIMIT 1;

-- name: GetRequestTypeDefaultsByCode :one
SELECT name_th, name_en, legal_ref, sla_days FROM dsar.request_types WHERE code = sqlc.arg(code) AND tenant_id IS NULL;

-- name: UpsertRequestTypeConfig :one
-- Always a tenant-owned row (current tenant, never NULL) — the display fields (name/legal_ref/sla_days) are
-- copied from the global default once, at first save, purely to satisfy this table's own NOT NULL columns;
-- DSAR-20's own config columns are what every later save actually changes.
INSERT INTO dsar.request_types (tenant_id, code, name_th, name_en, legal_ref, sla_days,
    default_subtask_action, default_assignee_user_id, response_code_result, response_code_rejection, response_code_request_info)
VALUES (current_setting('app.tenant_id')::uuid, sqlc.arg(code), sqlc.arg(name_th), sqlc.arg(name_en), sqlc.arg(legal_ref), sqlc.arg(sla_days),
    sqlc.narg(default_subtask_action), sqlc.narg(default_assignee_user_id), sqlc.narg(response_code_result),
    sqlc.narg(response_code_rejection), sqlc.narg(response_code_request_info))
ON CONFLICT ON CONSTRAINT uq_request_types_code DO UPDATE SET
    default_subtask_action = EXCLUDED.default_subtask_action,
    default_assignee_user_id = EXCLUDED.default_assignee_user_id,
    response_code_result = EXCLUDED.response_code_result,
    response_code_rejection = EXCLUDED.response_code_rejection,
    response_code_request_info = EXCLUDED.response_code_request_info
RETURNING id, code, name_th, name_en, legal_ref, sla_days, default_subtask_action, default_assignee_user_id,
    response_code_result, response_code_rejection, response_code_request_info, row_version;
