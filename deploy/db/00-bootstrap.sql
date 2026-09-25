-- PDPA platform — database bootstrap (run ONCE per environment by a superuser / the cloud admin role)
--   psql "postgresql://<admin>@<host>/postgres" -v ON_ERROR_STOP=1 \
--        -v migrator_password=... -v app_password=... -v platform_password=... -v readonly_password=... \
--        -f deploy/db/00-bootstrap.sql
-- Then run migrations as pdpa_migrator with the owner role (see backend/db/README.md) and deploy/db/10-grants.sql.

CREATE ROLE pdpa_owner NOLOGIN;                                                     -- owns every schema / table / function
CREATE ROLE pdpa_migrator LOGIN PASSWORD :'migrator_password' IN ROLE pdpa_owner;   -- cmd/migrate (connects with role=pdpa_owner)
CREATE ROLE pdpa_app LOGIN PASSWORD :'app_password' NOBYPASSRLS;                    -- cmd/api + tenant jobs in cmd/worker
CREATE ROLE pdpa_platform LOGIN PASSWORD :'platform_password' BYPASSRLS;            -- provider console only (/provider/v1): tenants, global master data
CREATE ROLE pdpa_readonly LOGIN PASSWORD :'readonly_password' NOBYPASSRLS;          -- reporting through masked views only

CREATE DATABASE pdpa OWNER pdpa_owner;                                             -- owner of the database also owns schema public (PG 15+)

\connect pdpa
-- extensions need superuser (pgvector is not a trusted extension); 00001_init.sql only re-checks them
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS vector;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
