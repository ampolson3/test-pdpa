-- name: ListRequestTypes :many
SELECT id, code, name_th, name_en, legal_ref, sla_days FROM dsar.request_types ORDER BY code;

-- name: GetRequestType :one
SELECT id, code, name_th, name_en, legal_ref, sla_days FROM dsar.request_types WHERE id = $1;
