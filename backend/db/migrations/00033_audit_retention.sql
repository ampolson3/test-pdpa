-- +goose Up
-- PLT-12 retention (decisions.md D-22): platform.audit_log is kept 5 years. Whole monthly partitions older than that are
-- dropped by the SECURITY DEFINER function below (worker job audit.retention). Dropping rows breaks each tenant's hash
-- chain at its oldest surviving row, so before a partition goes the function records, per tenant, the last row it
-- removes (platform.audit_chain_anchors); audit Verify then starts the chain from the newest anchor instead of NULL.
-- The anchors are append-only evidence of the purge. Rows in audit_log_default are never dropped here.

CREATE TABLE platform.audit_chain_anchors (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL,
    partition_name text NOT NULL,
    dropped_through timestamptz NOT NULL,
    last_id bigint NOT NULL,
    last_occurred_at timestamptz NOT NULL,
    last_hash char(64) NOT NULL,
    rows_dropped bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pk_platform_audit_chain_anchors PRIMARY KEY (id),
    CONSTRAINT uq_audit_chain_anchors UNIQUE (tenant_id, partition_name),
    CONSTRAINT fk_audit_chain_anchors_tenant FOREIGN KEY (tenant_id) REFERENCES platform.tenants (id)
);
CREATE INDEX ix_platform_audit_chain_anchors_latest ON platform.audit_chain_anchors (tenant_id, dropped_through DESC);
ALTER TABLE platform.audit_chain_anchors ENABLE ROW LEVEL SECURITY;
ALTER TABLE platform.audit_chain_anchors FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON platform.audit_chain_anchors
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);
COMMENT ON TABLE platform.audit_chain_anchors IS 'หลักฐานการลบ partition ของ audit_log ที่พ้นระยะเก็บ: hash สุดท้ายที่ถูกลบต่อ tenant (append-only)';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION platform.drop_expired_audit_partitions(p_keep_months int DEFAULT 60)
RETURNS TABLE (partition_name text, rows_dropped bigint)
LANGUAGE plpgsql
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
    v_cutoff timestamptz;
    v_part record;
    v_from timestamptz;
    v_to timestamptz;
    v_tenant uuid;
    v_prev_tenant text := current_setting('app.tenant_id', true);
    v_last record;
    v_count bigint;
    v_total bigint;
BEGIN
    -- The retention decided for the audit log is 5 years; the caller may keep longer, never shorter.
    IF p_keep_months IS NULL OR p_keep_months < 60 THEN
        RAISE EXCEPTION 'audit retention must be at least 60 months, got %', p_keep_months USING ERRCODE = '22023';
    END IF;
    v_cutoff := (date_trunc('month', now() AT TIME ZONE 'UTC') - make_interval(months => p_keep_months)) AT TIME ZONE 'UTC';

    FOR v_part IN
        SELECT c.relname
        FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE i.inhparent = 'platform.audit_log'::regclass AND n.nspname = 'platform' AND c.relname ~ '^audit_log_y[0-9]{4}m[0-9]{2}$'
        ORDER BY c.relname
    LOOP
        v_from := (to_date(substr(v_part.relname, 12, 4) || substr(v_part.relname, 17, 2) || '01', 'YYYYMMDD')::timestamp) AT TIME ZONE 'UTC';
        v_to := ((v_from AT TIME ZONE 'UTC') + interval '1 month') AT TIME ZONE 'UTC';
        CONTINUE WHEN v_to > v_cutoff;

        v_total := 0;
        -- Partitions force RLS, which applies to this function's owner too: read one tenant at a time.
        FOR v_tenant IN SELECT id FROM platform.tenants LOOP
            PERFORM set_config('app.tenant_id', v_tenant::text, true);
            EXECUTE format('SELECT id, occurred_at, hash FROM platform.%I WHERE tenant_id = $1 ORDER BY occurred_at DESC, id DESC LIMIT 1', v_part.relname)
                INTO v_last USING v_tenant;
            CONTINUE WHEN v_last IS NULL OR v_last.id IS NULL;
            EXECUTE format('SELECT count(*) FROM platform.%I WHERE tenant_id = $1', v_part.relname) INTO v_count USING v_tenant;
            INSERT INTO platform.audit_chain_anchors (tenant_id, partition_name, dropped_through, last_id, last_occurred_at, last_hash, rows_dropped)
            VALUES (v_tenant, v_part.relname, v_to, v_last.id, v_last.occurred_at, v_last.hash, v_count);
            v_total := v_total + v_count;
        END LOOP;
        PERFORM set_config('app.tenant_id', coalesce(v_prev_tenant, ''), true);

        EXECUTE format('DROP TABLE platform.%I', v_part.relname);
        partition_name := v_part.relname;
        rows_dropped := v_total;
        RETURN NEXT;
    END LOOP;
    PERFORM set_config('app.tenant_id', coalesce(v_prev_tenant, ''), true);
END;
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION platform.drop_expired_audit_partitions(int) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION platform.drop_expired_audit_partitions(int) TO pdpa_app;
-- Only the function writes anchors.
REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON platform.audit_chain_anchors FROM pdpa_app, pdpa_platform;
GRANT SELECT ON platform.audit_chain_anchors TO pdpa_app;

-- +goose Down
DROP FUNCTION IF EXISTS platform.drop_expired_audit_partitions(int);
DROP TABLE IF EXISTS platform.audit_chain_anchors;
