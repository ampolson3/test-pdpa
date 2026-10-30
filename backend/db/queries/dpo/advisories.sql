-- DPO-07: dpo.advisories/dpo.kb_articles were already fully specified in the baseline migrations
-- (00016_dpo.sql) — a department's own advisory question/answer, optionally promoted into a searchable
-- knowledge-base article. No queries existed for either table until now.

-- name: LockAdvisoryNumbering :exec
SELECT pg_advisory_xact_lock(hashtext('dpo.advisory_no:' || current_setting('app.tenant_id') || ':' || @year::text));

-- name: CountAdvisoriesInYear :one
SELECT count(*)::int FROM dpo.advisories WHERE advisory_no LIKE @prefix::text || '%';

-- name: InsertAdvisory :one
INSERT INTO dpo.advisories (tenant_id, advisory_no, org_unit_id, requester_user_id, subject, question, created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, @advisory_no::varchar, sqlc.narg(org_unit_id)::uuid, @requester_user_id::uuid,
        @subject::text, @question::text, sqlc.narg(actor)::uuid, sqlc.narg(actor)::uuid)
RETURNING *;

-- name: GetAdvisory :one
SELECT * FROM dpo.advisories WHERE id = $1;

-- name: ListAdvisories :many
SELECT * FROM dpo.advisories
WHERE (sqlc.narg(org_unit_id)::uuid IS NULL OR org_unit_id = sqlc.narg(org_unit_id)::uuid)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC;

-- name: AnswerAdvisory :one
UPDATE dpo.advisories
SET status = 'answered', answer = @answer::text, answered_by = sqlc.narg(answered_by)::uuid, answered_at = now(),
    updated_at = now(), updated_by = sqlc.narg(answered_by)::uuid, row_version = row_version + 1
WHERE id = @id::uuid AND status = 'open' AND row_version = @row_version::int
RETURNING *;

-- name: LinkAdvisoryKbArticle :exec
UPDATE dpo.advisories SET kb_article_id = $2, updated_at = now() WHERE id = $1;

-- name: InsertKbArticle :one
INSERT INTO dpo.kb_articles (tenant_id, category, title, body, language, tags, source, status, published_at, created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, @category::varchar, @title::text, @body::jsonb, @language::varchar,
        @tags::text[], @source::text, 'published', now(), sqlc.narg(actor)::uuid, sqlc.narg(actor)::uuid)
RETURNING *;

-- name: GetKbArticle :one
SELECT * FROM dpo.kb_articles WHERE id = $1 AND status = 'published';

-- name: SearchKbArticles :many
SELECT * FROM dpo.kb_articles
WHERE status = 'published'
  AND (sqlc.narg(category)::varchar IS NULL OR category = sqlc.narg(category)::varchar)
  AND (sqlc.narg(q)::text IS NULL OR title ILIKE '%' || sqlc.narg(q)::text || '%' OR body::text ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY published_at DESC;
