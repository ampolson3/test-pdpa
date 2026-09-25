---
description: Implement one backlog feature end-to-end (contract, data, service, UI, tests, docs)
argument-hint: <FEATURE-ID e.g. CON-09>
---

Implement feature **$ARGUMENTS** of the PDPA platform.

1. Gather context (read, do not skim):
   - the row for `$ARGUMENTS` in `docs/backlog/backlog.csv` (priority, phase, sizes, depends_on, processes, acceptance criteria);
   - its section in `docs/modules/<EPIC>.md` (anchor `#<id in lower case>`) and the module's technical summary, permissions, events and jobs at the top of that file;
   - every process / state machine / sequence linked from the feature or module (`docs/processes/`, `docs/states/`, `docs/sequences/`);
   - the data dictionary of each table involved (`docs/data/<schema>.md`) and the migration that created it;
   - the legal rows that apply in `docs/legal/pdpa-rules.md` and the rules in `CLAUDE.md`.
2. Check dependencies. A feature dependency is done when its `status` in `docs/backlog/backlog.csv` is `done` or the code clearly implements it; if you cannot tell, ask. Project-task dependencies (T-xx) that produce documents are covered by this kit: T04 → `docs/architecture/`, T05 → `docs/data/` + `api/openapi/README.md`, T06 → `docs/security/permissions.md` (pending customer confirmation, decisions Q-01). If a real dependency is missing, stop and list it.
3. Write a short plan: endpoints (method, path, x-permission), tables/columns (existing or new migration), state transitions, events, jobs, UI screens, tests mapped to each acceptance criterion. Flag anything that depends on `docs/decisions.md` §2 and ask before continuing if it changes behaviour.
4. Build in this order: OpenAPI → `make gen` → migration (if needed, new goose file with Down) → sqlc queries → store → service (validation, state machine from `docs/states/state-machines.yaml`, audit + outbox in the request transaction from the context — never open your own) → handler → frontend (i18n th/en, `packages/authz` guards) → tests.
5. Tests must include: every acceptance criterion; two-tenant RLS isolation for new repositories; 401/403 contract tests for new endpoints; allowed and forbidden transitions; deadline calculations with an injected clock.
6. Update the docs you changed (module section, data dictionary, states, events), set the feature's `status` in `docs/backlog/backlog.csv` (`in_progress` / `done`), and summarise what was done, what is left, and any open questions.
