-- +goose Up
-- monthly partitions of the append-only high-volume tables, created ahead by the worker job partition.maintain
-- SECURITY DEFINER (runs as the owner) so callers only need EXECUTE; every new partition gets RLS and no direct privileges
-- (a partition queried directly would bypass the parent's policies). Creating a month that already has rows in the
-- default partition fails on purpose — keep the job running ahead of time and alert on failure.

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION platform.ensure_monthly_partitions(p_months_ahead int DEFAULT 3)
RETURNS integer
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    t record;
    m int;
    v_from timestamptz;
    v_to timestamptz;
    v_name text;
    v_role text;
    v_created int := 0;
BEGIN
    FOR t IN SELECT * FROM (VALUES
        ('platform', 'audit_log'),
        ('iam', 'security_events'),
        ('consent', 'consent_transactions'),
        ('cookie', 'consent_records')) AS x(schema_name, table_name)
    LOOP
        FOR m IN 0..p_months_ahead LOOP
            v_from := (date_trunc('month', now() AT TIME ZONE 'UTC') + make_interval(months => m)) AT TIME ZONE 'UTC';
            v_to := ((date_trunc('month', now() AT TIME ZONE 'UTC') + make_interval(months => m + 1)) AT TIME ZONE 'UTC');
            v_name := format('%s_y%s', t.table_name, to_char(v_from AT TIME ZONE 'UTC', 'YYYY"m"MM'));
            IF to_regclass(format('%I.%I', t.schema_name, v_name)) IS NULL THEN
                EXECUTE format('CREATE TABLE %I.%I PARTITION OF %I.%I FOR VALUES FROM (%L) TO (%L)',
                               t.schema_name, v_name, t.schema_name, t.table_name, v_from, v_to);
                EXECUTE format('ALTER TABLE %I.%I ENABLE ROW LEVEL SECURITY', t.schema_name, v_name);
                EXECUTE format('ALTER TABLE %I.%I FORCE ROW LEVEL SECURITY', t.schema_name, v_name);
                EXECUTE format('CREATE POLICY tenant_isolation ON %I.%I USING (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid) '
                               'WITH CHECK (tenant_id = NULLIF(current_setting(''app.tenant_id'', true), '''')::uuid)', t.schema_name, v_name);
                EXECUTE format('REVOKE ALL ON %I.%I FROM PUBLIC', t.schema_name, v_name);
                FOR v_role IN SELECT rolname FROM pg_roles WHERE rolname IN ('pdpa_app', 'pdpa_platform', 'pdpa_readonly') LOOP
                    EXECUTE format('REVOKE ALL ON %I.%I FROM %I', t.schema_name, v_name, v_role);
                END LOOP;
                v_created := v_created + 1;
            END IF;
        END LOOP;
    END LOOP;
    RETURN v_created;
END;
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform.ensure_monthly_partitions(int) FROM PUBLIC;
SELECT platform.ensure_monthly_partitions(3);

-- +goose Down
-- partitions created by the function stay attached; they are dropped with their parent tables in 00002–00017
DROP FUNCTION IF EXISTS platform.ensure_monthly_partitions(int);
