---
description: Show everything linked to an ID (feature, process, state machine, table, event or permission)
argument-hint: <ID e.g. DSAR-08 | BP-06 | ST-02 | dsar.requests | consent.withdrawn | ropa.activity>
---

Trace **$ARGUMENTS** across the knowledge base and the code:

- Search `docs/` (backlog.csv, modules, processes, sequences, states, data, security, architecture/events.yaml, legal) and `api/openapi/`, `backend/`, `apps/`, `packages/` for the ID.
- Produce a compact map: feature(s) and acceptance criteria · actors · processes / sequences / state machines · tables and columns · endpoints and x-permission · events and jobs · legal sections · where it is implemented in code and which tests cover it.
- End with gaps: documented but not implemented, implemented but undocumented, or inconsistencies between sources (apply the source-of-truth order in `CLAUDE.md`).
