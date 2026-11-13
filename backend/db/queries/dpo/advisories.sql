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

-- DPO-12 ------------------------------------------------------------------------------------------

-- name: ListKbArticles :many
-- Readers see published articles only; a manager asks for drafts/archived too (include_all). Thai has no
-- word boundaries, so search is a plain case-insensitive substring match on title, body and tags.
SELECT * FROM dpo.kb_articles
WHERE (sqlc.arg(include_all)::boolean OR status = 'published')
  AND (sqlc.narg(category)::varchar IS NULL OR category = sqlc.narg(category)::varchar)
  AND (sqlc.narg(language)::varchar IS NULL OR language = sqlc.narg(language)::varchar)
  AND (sqlc.narg(source)::text IS NULL OR source = sqlc.narg(source)::text)
  AND (sqlc.narg(tag)::text IS NULL OR sqlc.narg(tag)::text = ANY(tags))
  AND (sqlc.narg(q)::text IS NULL OR title ILIKE '%' || sqlc.narg(q)::text || '%'
       OR body::text ILIKE '%' || sqlc.narg(q)::text || '%'
       OR array_to_string(tags, ' ') ILIKE '%' || sqlc.narg(q)::text || '%')
ORDER BY COALESCE(published_at, created_at) DESC, id;

-- name: GetKbArticleAnyStatus :one
SELECT * FROM dpo.kb_articles WHERE id = $1;

-- name: InsertKbArticleDraft :one
INSERT INTO dpo.kb_articles (tenant_id, category, title, body, language, tags, source, status, created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, @category::varchar, @title::text, @body::jsonb, @language::varchar,
        @tags::text[], @source::text, 'draft', sqlc.narg(actor)::uuid, sqlc.narg(actor)::uuid)
RETURNING *;

-- name: UpdateKbArticle :one
-- Only the tenant's own rows (a global default is read-only, enforced by RLS tenant_write too). Editing a
-- published article starts its next version_no; a draft edit stays on the same one.
UPDATE dpo.kb_articles
SET category = @category::varchar, title = @title::text, body = @body::jsonb, language = @language::varchar,
    tags = @tags::text[], source = @source::text,
    version_no = CASE WHEN status = 'published' THEN version_no + 1 ELSE version_no END,
    updated_at = now(), updated_by = sqlc.narg(actor)::uuid, row_version = row_version + 1
WHERE id = @id::uuid AND row_version = @row_version::int AND status IN ('draft', 'published')
RETURNING *;

-- name: SetKbArticleStatus :one
UPDATE dpo.kb_articles
SET status = @status::text,
    published_at = CASE WHEN @status::text = 'published' THEN now() ELSE published_at END,
    updated_at = now(), updated_by = sqlc.narg(actor)::uuid, row_version = row_version + 1
WHERE id = @id::uuid AND row_version = @row_version::int AND status = ANY(@from_statuses::text[])
RETURNING *;
