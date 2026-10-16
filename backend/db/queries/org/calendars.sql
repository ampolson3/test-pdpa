-- name: ListCalendars :many
SELECT id, name, timezone, workdays, is_default, row_version, updated_at
FROM org.business_calendars
ORDER BY is_default DESC, lower(name);

-- name: GetCalendar :one
SELECT id, name, timezone, workdays, is_default, row_version, updated_at
FROM org.business_calendars
WHERE id = $1;

-- name: GetCalendarByName :one
SELECT id, name, timezone, workdays, is_default, row_version, updated_at
FROM org.business_calendars
WHERE lower(name) = lower(sqlc.arg(name)::text);

-- name: GetDefaultCalendar :one
SELECT id, name, timezone, workdays, is_default, row_version, updated_at
FROM org.business_calendars
WHERE is_default;

-- name: InsertCalendar :one
INSERT INTO org.business_calendars (id, tenant_id, name, timezone, workdays, is_default, created_by, updated_by)
VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4,
        NOT EXISTS (SELECT 1 FROM org.business_calendars WHERE is_default),
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING id, name, timezone, workdays, is_default, row_version, updated_at;

-- name: UpdateCalendar :one
UPDATE org.business_calendars
SET name = $3, timezone = $4, workdays = $5, row_version = row_version + 1,
    updated_by = NULLIF(current_setting('app.user_id', true), '')::uuid
WHERE id = $1 AND row_version = $2
RETURNING id, name, timezone, workdays, is_default, row_version, updated_at;

-- name: ClearDefaultCalendar :exec
UPDATE org.business_calendars SET is_default = false WHERE is_default AND id <> $1;

-- name: MarkDefaultCalendar :exec
UPDATE org.business_calendars SET is_default = true WHERE id = $1;

-- name: UpsertDefaultCalendarSetting :exec
INSERT INTO org.org_settings (tenant_id, default_calendar_id, created_by, updated_by)
VALUES (current_setting('app.tenant_id')::uuid, $1, NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
ON CONFLICT (tenant_id) DO UPDATE
SET default_calendar_id = EXCLUDED.default_calendar_id, updated_by = EXCLUDED.updated_by, row_version = org.org_settings.row_version + 1;

-- name: ListHolidays :many
SELECT holiday_date, name
FROM org.holidays
WHERE calendar_id = $1
  AND (sqlc.narg(from_date)::date IS NULL OR holiday_date >= sqlc.narg(from_date)::date)
  AND (sqlc.narg(to_date)::date IS NULL OR holiday_date < sqlc.narg(to_date)::date)
ORDER BY holiday_date;

-- name: UpsertHoliday :one
INSERT INTO org.holidays (calendar_id, holiday_date, tenant_id, name)
VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3)
ON CONFLICT (calendar_id, holiday_date) DO UPDATE SET name = EXCLUDED.name
RETURNING holiday_date, name, (xmax = 0) AS inserted;

-- name: DeleteHoliday :execrows
DELETE FROM org.holidays WHERE calendar_id = $1 AND holiday_date = $2;
