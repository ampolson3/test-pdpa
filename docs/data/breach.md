# schema `breach`

> เหตุละเมิดข้อมูลและการแจ้ง สคส. / เจ้าของข้อมูล  
> migration: `backend/db/migrations/00013_breach.sql` · FK: `00018_foreign_keys.sql` · Go package เจ้าของ: [BRE](../modules/BRE.md) (`backend/internal/breach`)  
> ERD: `design/PDPA_System_Analysis.drawio` → ERD-12

กติกา: ตารางใน schema นี้อ่าน/เขียนได้เฉพาะ package เจ้าของ · module อื่นเรียกผ่าน service interface หรือรับ domain event

## สรุปตาราง

| ตาราง | คำอธิบาย | tenant / RLS | partition | ERD | ใช้ใน BP / SEQ |
|---|---|---|---|---|---|
| [incidents](#breach-incidents) | เหตุละเมิดข้อมูลส่วนบุคคล | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07, SEQ-06 |
| [incident_assets](#breach-incident-assets) | ระบบ / กิจกรรมที่เกี่ยวข้องกับเหตุ | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07 |
| [assessments](#breach-assessments) | การประเมินความเสี่ยงของเหตุ (ประกาศแจ้งเหตุ พ.ศ. 2565) | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07 |
| [pdpc_notifications](#breach-pdpc-notifications) | การแจ้ง สคส. (ฉบับเบื้องต้น / เพิ่มเติม / สุดท้าย) | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07, SEQ-06 |
| [subject_notifications](#breach-subject-notifications) | การแจ้งเจ้าของข้อมูล (ความเสี่ยงสูง) | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07, SEQ-06 |
| [notification_recipients](#breach-notification-recipients) | ผู้รับแจ้งรายคน | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07 |
| [playbooks](#breach-playbooks) | playbook ตามประเภทเหตุ | tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write` |  | ERD-12 | BP-07 |
| [response_tasks](#breach-response-tasks) | งานควบคุม / แก้ไข / กู้คืน | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07 |
| [evidence](#breach-evidence) | หลักฐาน (hash) | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07 |
| [timeline_events](#breach-timeline-events) | ลำดับเหตุการณ์และการตัดสินใจ | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07, SEQ-06 |
| [root_causes](#breach-root-causes) | สาเหตุและมาตรการป้องกันซ้ำ | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07 |
| [routing_rules](#breach-routing-rules) | กฎผู้รับแจ้งและทีมตอบสนอง | tenant · RLS `tenant_isolation` |  | ERD-12 | BP-07 |

<a id="breach-incidents"></a>
## breach.incidents

เหตุละเมิดข้อมูลส่วนบุคคล

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_no` | `varchar(30)` | ✓ |  | UQ |  |
| `legal_entity_id` | `uuid` | ✓ |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `reported_via` | `text` | ✓ |  |  | ค่า: `employee_form`, `public_form`, `processor`, `system`, `email`, `phone` |
| `reporter_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `reporter_contact_enc` | `bytea` |  |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `processor_party_id` | `uuid` |  |  | FK → [org.external_parties](org.md#org-external-parties) |  |
| `title` | `text` | ✓ |  |  |  |
| `description` | `text` | ✓ |  |  |  |
| `breach_types` | `text[]` | ✓ |  |  |  |
| `incident_type` | `varchar(40)` |  |  |  |  |
| `occurred_at` | `timestamptz` |  |  |  |  |
| `aware_at` | `timestamptz` | ✓ |  |  |  |
| `contained_at` | `timestamptz` |  |  |  |  |
| `affected_subjects` | `int` |  |  |  |  |
| `affected_categories` | `uuid[]` | ✓ | '{}' |  |  |
| `risk_level` | `text` |  |  |  | ค่า: `none`, `low`, `high` |
| `decision` | `text` |  |  |  | ค่า: `no_notification`, `notify_pdpc`, `notify_pdpc_and_subjects` |
| `decision_reason` | `text` |  |  |  |  |
| `decided_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `pdpc_due_at` | `timestamptz` | ✓ |  | IX |  |
| `late_reason` | `text` |  |  |  |  |
| `is_drill` | `boolean` | ✓ | false |  |  |
| `workflow_instance_id` | `uuid` |  |  | FK → [platform.workflow_instances](platform.md#platform-workflow-instances) |  |
| `status` | `text` | ✓ | 'reported' |  | ค่า: `reported`, `triage`, `assessing`, `notifying`, `remediating`, `closed` · state machine [ST-03](../states/ST-03.md) |
| `owner_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) | ผู้รับผิดชอบเหตุ (BRE-02, migration 00036) — ค่าเริ่มต้นคือผู้บันทึก |
| `close_reason` | `text` |  |  |  | เหตุผลที่ปิด (ไม่ใช่เหตุละเมิด / บทเรียน) (00036) |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_incidents_updated`
- PK: `(id)`
- Unique: `uq_incidents_incident_no UNIQUE (tenant_id, incident_no)`
- Index: `breach.incidents (tenant_id, legal_entity_id)` · `breach.incidents (tenant_id, reporter_user_id)` · `breach.incidents (tenant_id, processor_party_id)` · `breach.incidents (tenant_id, decided_by)` · `breach.incidents (tenant_id, pdpc_due_at)` · `breach.incidents (tenant_id, workflow_instance_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `breach.incident_assets.incident_id`, `breach.assessments.incident_id`, `breach.pdpc_notifications.incident_id`, `breach.subject_notifications.incident_id`, `breach.response_tasks.incident_id`, `breach.evidence.incident_id`, `breach.timeline_events.incident_id`, `breach.root_causes.incident_id`, `gov.regulator_letters.incident_id`

<a id="breach-incident-assets"></a>
## breach.incident_assets

ระบบ / กิจกรรมที่เกี่ยวข้องกับเหตุ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_id` | `uuid` | ✓ |  | FK → [breach.incidents](#breach-incidents) |  |
| `asset_id` | `uuid` |  |  | FK → [ropa.assets](ropa.md#ropa-assets) |  |
| `activity_id` | `uuid` |  |  | FK → [ropa.processing_activities](ropa.md#ropa-processing-activities) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_incident_assets_updated`
- PK: `(id)`
- Index: `breach.incident_assets (tenant_id, incident_id)` · `breach.incident_assets (tenant_id, asset_id)` · `breach.incident_assets (tenant_id, activity_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="breach-assessments"></a>
## breach.assessments

การประเมินความเสี่ยงของเหตุ (ประกาศแจ้งเหตุ พ.ศ. 2565)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_id` | `uuid` | ✓ |  | FK → [breach.incidents](#breach-incidents) |  |
| `form_submission_id` | `uuid` | ✓ |  | FK → [platform.form_submissions](platform.md#platform-form-submissions) |  |
| `score` | `numeric(6,2)` | ✓ |  |  |  |
| `risk_level` | `text` | ✓ |  |  | ค่า: `none`, `low`, `high` |
| `factors` | `jsonb` | ✓ |  |  |  |
| `assessed_by` | `uuid` | ✓ |  | FK → [iam.users](iam.md#iam-users) |  |
| `assessed_at` | `timestamptz` | ✓ |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_assessments_updated`
- PK: `(id)`
- Index: `breach.assessments (tenant_id, incident_id)` · `breach.assessments (tenant_id, form_submission_id)` · `breach.assessments (tenant_id, assessed_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="breach-pdpc-notifications"></a>
## breach.pdpc_notifications

การแจ้ง สคส. (ฉบับเบื้องต้น / เพิ่มเติม / สุดท้าย)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_id` | `uuid` | ✓ |  | FK → [breach.incidents](#breach-incidents) |  |
| `sequence_no` | `smallint` | ✓ |  |  |  |
| `notification_type` | `text` | ✓ |  |  | ค่า: `initial`, `supplementary`, `final` |
| `document_version_id` | `uuid` | ✓ |  | FK → [platform.document_versions](platform.md#platform-document-versions) |  |
| `approved_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `submitted_at` | `timestamptz` |  |  |  |  |
| `submission_ref` | `varchar(60)` |  |  |  |  |
| `is_late` | `boolean` | ✓ | false |  |  |
| `late_reason` | `text` |  |  |  |  |
| `evidence_file_id` | `uuid` |  |  | FK → [platform.files](platform.md#platform-files) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_pdpc_notifications_updated`
- PK: `(id)`
- Unique: `uq_pdpc_notifications_incident_id_sequence_no UNIQUE (incident_id, sequence_no)`
- Index: `breach.pdpc_notifications (tenant_id, incident_id)` · `breach.pdpc_notifications (tenant_id, document_version_id)` · `breach.pdpc_notifications (tenant_id, approved_by)` · `breach.pdpc_notifications (tenant_id, evidence_file_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="breach-subject-notifications"></a>
## breach.subject_notifications

การแจ้งเจ้าของข้อมูล (ความเสี่ยงสูง)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_id` | `uuid` | ✓ |  | FK → [breach.incidents](#breach-incidents) |  |
| `channel` | `text` | ✓ |  |  | ค่า: `email`, `sms`, `line`, `letter`, `website` |
| `template_id` | `uuid` |  |  | FK → [platform.notification_templates](platform.md#platform-notification-templates) |  |
| `total_recipients` | `int` | ✓ | 0 |  |  |
| `sent_count` | `int` | ✓ | 0 |  |  |
| `failed_count` | `int` | ✓ | 0 |  |  |
| `status` | `text` | ✓ | 'draft' |  | ค่า: `draft`, `sending`, `done`, `failed` |
| `started_at` | `timestamptz` |  |  |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |
| `variables` | `jsonb` | ✓ | '{}' |  | เนื้อหาเฉพาะเหตุ: organization, summary, remedy, contact (00036) |
| `template_code` | `varchar(80)` | ✓ | 'breach.subject_notice' |  | template ของ PLT-04 (00036) |
| `approved_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) | ผู้อนุมัติส่ง — ต้องไม่ใช่ผู้จัดทำ (00036) |
| `approved_at` | `timestamptz` |  |  |  | (00036) |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_subject_notifications_updated`
- PK: `(id)`
- Index: `breach.subject_notifications (tenant_id, incident_id)` · `breach.subject_notifications (tenant_id, template_id)`
- RLS: tenant · RLS `tenant_isolation`
- ถูกอ้างถึงโดย: `breach.notification_recipients.subject_notification_id`

<a id="breach-notification-recipients"></a>
## breach.notification_recipients

ผู้รับแจ้งรายคน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `subject_notification_id` | `uuid` | ✓ |  | FK → [breach.subject_notifications](#breach-subject-notifications) |  |
| `subject_id` | `uuid` |  |  | FK → [consent.data_subjects](consent.md#consent-data-subjects) |  |
| `address_enc` | `bytea` | ✓ |  |  | เข้ารหัส (envelope) — ห้าม log / ห้ามคืนค่าโดยไม่ mask |
| `status` | `text` | ✓ | 'queued' |  | ค่า: `queued`, `sent`, `failed` |
| `sent_at` | `timestamptz` |  |  |  |  |
| `error` | `text` |  |  |  |  |
| `notification_id` | `uuid` |  |  |  | ข้อความใน platform.notifications ที่ส่งต่อให้ (ติดตามผลรายคน, 00036) |
| `language` | `varchar(5)` | ✓ | 'th' |  | ภาษาของผู้รับ (00036) |
| `line_no` | `int` |  |  |  | บรรทัดใน CSV ต้นทาง (00036) |

- PK: `(id)`
- Index: `breach.notification_recipients (tenant_id, subject_notification_id)` · `breach.notification_recipients (tenant_id, subject_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="breach-playbooks"></a>
## breach.playbooks

playbook ตามประเภทเหตุ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` |  |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS · NULL = ข้อมูลกลาง |
| `incident_type` | `varchar(40)` | ✓ |  |  |  |
| `name` | `text` | ✓ |  |  |  |
| `steps` | `jsonb` | ✓ |  |  |  |
| `version_no` | `int` | ✓ | 1 |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_playbooks_updated`
- PK: `(id)`
- RLS: tenant + ข้อมูลกลาง (tenant_id NULL) · RLS `tenant_read` / `tenant_write`
- ถูกอ้างถึงโดย: `breach.response_tasks.playbook_id`

<a id="breach-response-tasks"></a>
## breach.response_tasks

งานควบคุม / แก้ไข / กู้คืน

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_id` | `uuid` | ✓ |  | FK → [breach.incidents](#breach-incidents) |  |
| `playbook_id` | `uuid` |  |  | FK → [breach.playbooks](#breach-playbooks) |  |
| `step_code` | `varchar(40)` |  |  |  |  |
| `title` | `text` | ✓ |  |  |  |
| `assignee_user_id` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `assignee_group_id` | `uuid` |  |  | FK → [iam.groups](iam.md#iam-groups) |  |
| `status` | `text` | ✓ | 'open' |  | ค่า: `open`, `in_progress`, `done`, `cancelled` |
| `due_at` | `timestamptz` |  |  |  |  |
| `completed_at` | `timestamptz` |  |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_response_tasks_updated`
- PK: `(id)`
- Index: `breach.response_tasks (tenant_id, incident_id)` · `breach.response_tasks (tenant_id, playbook_id)` · `breach.response_tasks (tenant_id, assignee_user_id)` · `breach.response_tasks (tenant_id, assignee_group_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="breach-evidence"></a>
## breach.evidence

หลักฐาน (hash)

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_id` | `uuid` | ✓ |  | FK → [breach.incidents](#breach-incidents) |  |
| `file_id` | `uuid` | ✓ |  | FK → [platform.files](platform.md#platform-files) |  |
| `description` | `text` |  |  |  |  |
| `collected_by` | `uuid` |  |  | FK → [iam.users](iam.md#iam-users) |  |
| `collected_at` | `timestamptz` | ✓ |  |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_evidence_updated`
- PK: `(id)`
- Index: `breach.evidence (tenant_id, incident_id)` · `breach.evidence (tenant_id, file_id)` · `breach.evidence (tenant_id, collected_by)`
- RLS: tenant · RLS `tenant_isolation`

<a id="breach-timeline-events"></a>
## breach.timeline_events

ลำดับเหตุการณ์และการตัดสินใจ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_id` | `uuid` | ✓ |  | FK → [breach.incidents](#breach-incidents) |  |
| `occurred_at` | `timestamptz` | ✓ |  |  |  |
| `event_type` | `text` | ✓ |  |  | ค่า: `decision`, `action`, `communication`, `system`, `note` |
| `description` | `text` | ✓ |  |  |  |
| `actor_id` | `uuid` |  |  |  |  |
| `is_auto` | `boolean` | ✓ | false |  |  |

- PK: `(id)`
- Index: `breach.timeline_events (tenant_id, incident_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="breach-root-causes"></a>
## breach.root_causes

สาเหตุและมาตรการป้องกันซ้ำ

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_id` | `uuid` | ✓ |  | FK → [breach.incidents](#breach-incidents) |  |
| `cause_category` | `varchar(40)` | ✓ |  |  |  |
| `description` | `text` | ✓ |  |  |  |
| `corrective_actions` | `jsonb` | ✓ | '[]'::jsonb |  |  |
| `risk_id` | `uuid` |  |  | FK → [risk.risks](risk.md#risk-risks) |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_root_causes_updated`
- PK: `(id)`
- Index: `breach.root_causes (tenant_id, incident_id)` · `breach.root_causes (tenant_id, risk_id)`
- RLS: tenant · RLS `tenant_isolation`

<a id="breach-routing-rules"></a>
## breach.routing_rules

กฎผู้รับแจ้งและทีมตอบสนอง

| คอลัมน์ | type | NOT NULL | default | key / อ้างอิง | หมายเหตุ |
|---|---|---|---|---|---|
| `id` | `uuid` | ✓ | gen_random_uuid() | PK |  |
| `tenant_id` | `uuid` | ✓ |  | FK → [platform.tenants](platform.md#platform-tenants) | RLS |
| `incident_type` | `varchar(40)` |  |  |  |  |
| `min_risk` | `text` |  |  |  | ค่า: `none`, `low`, `high` |
| `legal_entity_id` | `uuid` |  |  | FK → [org.legal_entities](org.md#org-legal-entities) |  |
| `group_id` | `uuid` | ✓ |  | FK → [iam.groups](iam.md#iam-groups) |  |
| `channels` | `text[]` | ✓ | '{email}' |  |  |
| `is_active` | `boolean` | ✓ | true |  |  |

- มีคอลัมน์มาตรฐาน `created_at · created_by · updated_at · updated_by · row_version` + trigger `trg_routing_rules_updated`
- PK: `(id)`
- Index: `breach.routing_rules (tenant_id, legal_entity_id)` · `breach.routing_rules (tenant_id, group_id)`
- RLS: tenant · RLS `tenant_isolation`
