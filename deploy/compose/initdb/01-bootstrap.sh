#!/bin/bash
# Wraps deploy/db/00-bootstrap.sql (mounted read-only at /deploy-db, NOT under
# /docker-entrypoint-initdb.d, so Postgres's own auto-run doesn't execute it without these
# variables) with the psql -v values it expects. Runs once, only on first container init.
set -euo pipefail

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" \
  -v migrator_password="$PDPA_MIGRATOR_PASSWORD" \
  -v app_password="$PDPA_APP_PASSWORD" \
  -v platform_password="$PDPA_PLATFORM_PASSWORD" \
  -v readonly_password="$PDPA_READONLY_PASSWORD" \
  -f /deploy-db/00-bootstrap.sql
