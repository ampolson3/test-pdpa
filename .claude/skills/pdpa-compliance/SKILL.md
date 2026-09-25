---
name: pdpa-compliance
description: Thai PDPA and security checklist for this platform. Use when designing, implementing or reviewing anything that touches personal data, consent or cookies, privacy notices, RoPA, data subject requests, breach notification, retention or deletion, cross-border transfer, vendors/processors, DPO work, authentication, permissions, audit or tenant isolation.
---

# PDPA compliance for the PDPA platform

The product helps customers comply with the Thai Personal Data Protection Act B.E. 2562 (PDPA), so the platform itself must behave compliantly and produce evidence. Law-to-rule mapping lives in `docs/legal/pdpa-rules.md`; security policy in `docs/architecture/security.md`; permissions in `docs/security/permissions.md`.

## Before writing code
1. Identify which PDPA sections apply (the "มาตรา" column in `docs/legal/pdpa-rules.md`) and which deadline, if any, the feature computes.
2. Identify the state machine (`docs/states/`) and the process (`docs/processes/`) the feature participates in.
3. Check `docs/decisions.md` §2: if the feature depends on an open question, implement the documented default as configuration and tell the user.

## Checklist (every item needs a test or an explicit "not applicable")
**Consent (ม.19–21, 26)** — one decision per purpose, nothing pre-selected, purpose text versioned and the shown version stored with each transaction, sensitive data requires explicit consent, minors / incapacitated persons go through guardian approval (status PENDING), withdrawal is as easy as giving and propagates to downstream systems via events, receipts are hash-chained.

**Notice (ม.23, 25)** — notices pass the ม.23 checklist before review; material changes create a new version and trigger re-acknowledgement; data from other sources gets notified within 30 days (job + reminder).

**RoPA (ม.39, 40)** — every activity has purpose, lawful basis (LIA for legitimate interest), data categories, subjects, recipients, transfers, retention and security measures before approval; refusals of data subject requests are recorded against the activity.

**Data subject requests (ม.30–36)** — identity verified before disclosure (OTP / ThaID / documents, max 5 OTP attempts), 30-day SLA from `received_at`, the same response whether or not data is found, refusals need a reason and are recorded, packages are encrypted with expiring links.

**Breach (ม.37(4))** — `aware_at` starts the 72-hour clock, reminders at 24/48/66 h, late submission requires `late_reason`, high risk triggers data subject notification, the timeline is append-only, sending the PDPC form needs DPO approval (maker-checker).

**Retention and deletion (ม.33, 37(3))** — every stored category has a retention rule; deletions are approved, executed and evidenced; legal holds block deletion.

**Transfers (ม.28–29)** — destination country adequacy or a documented safeguard / exemption; TIA when required.

**Processors and sharing (ม.37(2), 40, 27)** — processors need an active DPA; controller-to-controller sharing needs a DSA; vendors are assessed before approval.

**Security (ม.37(1), policy)** — tenant isolation via `SET LOCAL app.tenant_id` + RLS; `x-permission` on every operation; MFA for privileged roles and step-up for unmask / bulk export / role changes; no PII in logs, events or error messages; `*_enc` + blind index for identifiers; masking by default; audit log entries for login, permission changes, export, unmask, delete and approvals.

## Red flags — stop and fix
- A query or job that can run without a tenant context, or code that needs BYPASSRLS in the API.
- Logging request bodies, identifiers, OTPs, tokens or free-text fields that may contain personal data.
- UPDATE/DELETE on `consent.consent_transactions`, `consent.consent_receipts`, `breach.timeline_events`, `platform.audit_log`.
- Status assignment outside the owning service or without audit + outbox in the same transaction.
- Legal wording hard-coded in code instead of templates approved by Legal.
- A deadline computed with `time.Now()` directly instead of the injectable clock.
