---
description: Review a change against PDPA rules, security policy and project conventions
argument-hint: "[path, branch or blank for the current diff]"
---

Review "$ARGUMENTS" for this PDPA platform (if empty, review all local changes: `git status --porcelain`, `git diff HEAD`, and read every untracked file it lists). Use the `pdpa-compliance` skill and check, citing file:line for every finding:

1. Tenant isolation: every query inside a `SET LOCAL app.tenant_id` transaction; no BYPASSRLS/superuser connection in API code; new repositories have two-tenant tests.
2. Authorization: every new/changed operation has the right `x-permission`; data scope and record rules applied; 403 contract tests exist.
3. PII: nothing personal, secret, token or OTP in logs, errors, events or analytics; `*_enc` + `blind_index` used for identifiers; masking by default; unmask flow audited.
4. Evidence: append-only tables untouched by UPDATE/DELETE; audit_log + outbox written in the same transaction as state changes; transitions allowed by `docs/states/state-machines.yaml`.
5. Law (`docs/legal/pdpa-rules.md`): consent is granular, unbundled, versioned and withdrawable; notices contain the ม.23 items; DSAR 30-day SLA and recorded refusals (ม.39); breach 72 h clock from `aware_at` and late reason; retention and deletion; cross-border checks; minors' consent.
6. API conventions: problem+json codes, Idempotency-Key, ETag/If-Match, pagination, snake_case, UTC timestamps.
7. Migrations: new file only, Down works, RLS/trigger/indexes/unique keys present.
8. i18n and accessibility of new UI text; no hard-coded legal wording.

Report findings grouped as **Blocking**, **Should fix**, **Nit**, each with the rule it violates and a concrete fix. Say explicitly when a category has no findings.
