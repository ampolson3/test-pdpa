-- CON-01's own acceptance criterion's evidence: one append-only row per banner decision (CLAUDE.md rule 4
-- doesn't name this table explicitly, but it is the same append-only evidence shape as consent.* and
-- breach.timeline_events — no update/delete query is ever written for it).

-- name: InsertConsentRecord :exec
INSERT INTO cookie.consent_records (tenant_id, occurred_at, domain_id, visitor_id, banner_config_id, action, choices, ip_hash, country, user_agent)
VALUES (current_setting('app.tenant_id')::uuid, now(), $1, $2, $3, $4, $5, $6, $7, $8);
