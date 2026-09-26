-- PDPA platform — privileges (idempotent). Run after every migration batch as the owner:
--   psql "postgresql://pdpa_migrator@<host>/pdpa?options=-c%20role%3Dpdpa_owner" -v ON_ERROR_STOP=1 -f deploy/db/10-grants.sql
-- cmd/migrate should run this file right after goose + River migrations.

GRANT USAGE ON SCHEMA public, platform, iam, org, consent, cookie, notice, ropa, dataflow, risk, assess, dsar, breach, vendor, agreement, dpo, gov
  TO pdpa_app, pdpa_platform, pdpa_readonly;

-- application roles: DML only (no DDL, no TRUNCATE)
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA
  public, platform, iam, org, consent, cookie, notice, ropa, dataflow, risk, assess, dsar, breach, vendor, agreement, dpo, gov
  TO pdpa_app, pdpa_platform;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA
  public, platform, iam, org, consent, cookie, notice, ropa, dataflow, risk, assess, dsar, breach, vendor, agreement, dpo, gov
  TO pdpa_app, pdpa_platform;
REVOKE ALL ON public.goose_db_version FROM pdpa_app, pdpa_platform;

-- append-only evidence
REVOKE UPDATE, DELETE, TRUNCATE ON platform.audit_log, consent.consent_transactions, consent.consent_receipts, breach.timeline_events
  FROM pdpa_app, pdpa_platform;
-- written only by platform.drop_expired_audit_partitions() (SECURITY DEFINER), read by audit Verify
REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON platform.audit_chain_anchors FROM pdpa_app, pdpa_platform;

-- wrapped data keys: deleting one makes its data unreadable (crypto-shredding is a tenant-offboarding step, PLT-13)
REVOKE DELETE, TRUNCATE ON platform.tenant_keys FROM pdpa_app, pdpa_platform;

-- global tables without RLS: read-only for the application, written by the provider console (pdpa_platform) or migrations
REVOKE INSERT, UPDATE, DELETE ON platform.tenants, iam.permissions, org.lawful_bases, org.countries, cookie.cookie_kb,
  agreement.mandatory_rules, gov.regulatory_updates FROM pdpa_app;

-- partitions are reachable only through their parent, where RLS applies (a partition queried directly bypasses the parent's policies)
DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT c.oid::regclass AS rel FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
           WHERE c.relispartition AND c.relkind IN ('r', 'p') AND n.nspname IN ('platform', 'iam', 'consent', 'cookie') LOOP
    EXECUTE format('REVOKE ALL ON %s FROM pdpa_app, pdpa_platform, pdpa_readonly', r.rel);
  END LOOP;
END $$;

-- worker job partition.maintain
GRANT EXECUTE ON FUNCTION platform.ensure_monthly_partitions(int) TO pdpa_app, pdpa_platform;
-- worker job audit.retention (the function refuses to keep less than 60 months)
GRANT EXECUTE ON FUNCTION platform.drop_expired_audit_partitions(int) TO pdpa_app;

-- tables created by later migrations (as pdpa_owner) get the same DML grants automatically;
-- ensure_monthly_partitions() revokes them again on every new partition
ALTER DEFAULT PRIVILEGES FOR ROLE pdpa_owner IN SCHEMA
  public, platform, iam, org, consent, cookie, notice, ropa, dataflow, risk, assess, dsar, breach, vendor, agreement, dpo, gov
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pdpa_app, pdpa_platform;
ALTER DEFAULT PRIVILEGES FOR ROLE pdpa_owner IN SCHEMA
  public, platform, iam, org, consent, cookie, notice, ropa, dataflow, risk, assess, dsar, breach, vendor, agreement, dpo, gov
  GRANT USAGE, SELECT ON SEQUENCES TO pdpa_app, pdpa_platform;
