---
description: Design or extend the OpenAPI contract for a feature (paths, schemas, x-permission, errors)
argument-hint: <FEATURE-ID or module>
---

Design the API contract for **$ARGUMENTS** before any implementation.

1. Read `api/openapi/README.md` (conventions), the feature / module in `docs/modules/`, the related processes and sequences, `docs/security/permissions.md` and the data dictionary of the tables involved.
2. Reuse endpoints already fixed by the SA pack (listed in the module doc and in `docs/architecture/integration.md`) — do not rename them.
3. For each operation define: surface and path (module prefix from the README table), `operationId`, tag, `x-permission` (an existing code from `docs/security/permissions.yaml`, or propose a new one and say so), request/response schemas in snake_case, `Idempotency-Key` / `If-Match` where required, pagination for lists, problem+json responses with the stable `code` values each operation can return, and at least one example.
4. Map every acceptance criterion of the feature to an operation or show why it is UI-only.
5. Edit `api/openapi/` accordingly, validate the spec, run `make gen` if the scaffold exists, and summarise the changes plus any new permission codes (these need a migration and an update to `docs/security/permissions.yaml`).
