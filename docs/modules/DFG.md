# DFG — แผนผังการไหลของข้อมูล (Data Flow Generator)

> ระบบจัดการแผนผังการไหลของการประมวลผลข้อมูลส่วนบุคคลแบบอัตโนมัติ (Data Flow Generator) · ขอบเขต: แผนผังการไหลของข้อมูลที่สร้างจาก RoPA และการค้นหาข้อมูลอัตโนมัติ  
> 11 features · Must 5 / Should 4 / Nice 2 · phase: P2 (5), P3 (4), P4 (2)

## ภาพรวมทางเทคนิค

| หัวข้อ | รายละเอียด |
|---|---|
| Go package | `backend/internal/dataflow` |
| PostgreSQL schema | [`dataflow`](../data/dataflow.md) (5 ตาราง) |
| Admin API prefix | `/admin/v1/dataflow` |
| Endpoint ที่ SA กำหนดแล้ว | — (ออกแบบตาม [API conventions](../../api/openapi/README.md)) |
| หน้าจอ (Next.js) | admin: /data-flow/* (React Flow) |
| พึ่งพาบริการ | events, connectors |
| Diagram ต้นฉบับ | `design/PDPA_System_Analysis.drawio` → UC-08 DFG, DFD-1, ERD-09 |

## Actors

| key | ชื่อ | English | การยืนยันตัวตน |
|---|---|---|---|
| OWNER | เจ้าของกระบวนการ / ผู้ประสานงานแผนก | Process Owner / Champion | OIDC SSO · Admin app |
| IT | เจ้าของระบบ / IT | System Owner / IT | OIDC SSO + MFA · Admin app |
| DATASRC | แหล่งข้อมูล (DB / File / Cloud) | Data Sources | Connector credential ใน OpenBao · read-only |
| DPO | DPO / Privacy Team | DPO / Privacy Team | OIDC SSO + MFA · Admin app |
| AUDIT | ผู้ตรวจสอบ | Auditor | OIDC SSO + MFA · สิทธิ์อ่านอย่างเดียว |
| SCHED | ระบบ: Scheduler / Event | System Timer & Events | ภายในระบบ (River worker / cron) |

## รายการ feature / use case

เรียงตาม phase แล้วตามลำดับใน Function List · UC ID = Function ID = รหัสใน backlog

| ID | ชื่อ | Priority | Phase | Actor | BE | FE | UX | BP |
|---|---|---|---|---|---|---|---|---|
| [DFG-01](#dfg-01) | สร้างแผนผังอัตโนมัติจาก RoPA | Must | P2 | DPO OWNER | M | XL | Y |  |
| [DFG-02](#dfg-02) | อัปเดตแผนผังเมื่อข้อมูลเปลี่ยน | Must | P2 | SCHED | S | S | N |  |
| [DFG-03](#dfg-03) | แสดงการโอนไปต่างประเทศ | Must | P2 | DPO | S | M | N |  |
| [DFG-04](#dfg-04) | แสดงผู้ประมวลผลและผู้รับข้อมูล | Must | P2 | DPO | S | S | N |  |
| [DFG-08](#dfg-08) | ส่งออกภาพและรายงาน | Must | P2 | DPO AUDIT | M | M | N |  |
| [DFG-05](#dfg-05) | แก้ไขแผนผังแบบลากวาง | Should | P3 | DPO | S | L | Y |  |
| [DFG-06](#dfg-06) | มุมมองหลายระดับ | Should | P3 | OWNER DPO | M | M | N |  |
| [DFG-07](#dfg-07) | เผยแพร่และประวัติเวอร์ชันแผนผัง | Should | P3 | DPO | S | S | N |  |
| [DFG-09](#dfg-09) | ค้นหาข้อมูลส่วนบุคคลอัตโนมัติ (Data Discovery) | Should | P3 | IT DATASRC | XL | M | Y |  |
| [DFG-10](#dfg-10) | แผนผังระดับระบบ (Data lineage) | Nice | P4 | IT | M | M | N |  |
| [DFG-11](#dfg-11) | แผนที่โลกของการโอนข้อมูล | Nice | P4 | DPO | S | M | N |  |

### ความสัมพันธ์ระหว่าง use case

- DFG-01 «include» DFG-03 (ทุกครั้งที่ทำ DFG-01 ต้องทำ DFG-03)
- DFG-01 «include» DFG-04 (ทุกครั้งที่ทำ DFG-01 ต้องทำ DFG-04)
- DFG-02 «extend» DFG-01 (DFG-02 เป็นทางเลือก/ส่วนขยายของ DFG-01)
- DFG-09 «extend» DFG-01 (DFG-09 เป็นทางเลือก/ส่วนขยายของ DFG-01)

## ตารางข้อมูล

| ตาราง | คำอธิบาย |
|---|---|
| [dataflow.layouts](../data/dataflow.md#dataflow-layouts) | ตำแหน่ง node ที่ผู้ใช้จัดเองต่อมุมมอง |
| [dataflow.snapshots](../data/dataflow.md#dataflow-snapshots) | แผนผังที่เผยแพร่แล้ว (มีเวอร์ชัน) |
| [dataflow.classifiers](../data/dataflow.md#dataflow-classifiers) | classifier ข้อมูลส่วนบุคคล (รวมรูปแบบไทย) |
| [dataflow.discovery_scans](../data/dataflow.md#dataflow-discovery-scans) | รอบสแกนหาข้อมูลส่วนบุคคลผ่าน connector |
| [dataflow.discovery_findings](../data/dataflow.md#dataflow-discovery-findings) | ผลที่พบ (เก็บเฉพาะ metadata ไม่เก็บค่าจริง) |

## สิทธิ์ (x-permission)

รูปแบบ `x-permission: <area>.<resource>.<action>` เช่น `ropa.dataflow.read` (area ไม่จำเป็นต้องตรงกับชื่อ package) · ตัวอักษร: C สร้าง · R ดู · U แก้ไข · D ลบ · A อนุมัติ · P เผยแพร่ · E ส่งออก · X ดำเนินการ — รายละเอียดใน [permissions.md](../security/permissions.md)

| permission code | ความหมาย | role → action | หมายเหตุ |
|---|---|---|---|
| `ropa.dataflow` | แผนผังการไหลของข้อมูล | DPO `RUPE` · PRIVACY `RUE` · LEGAL `R` · OWNER `R` · IT `R` · SEC `R` · AUDIT `RE` · EXEC `R` |  |
| `ropa.discovery` | Data discovery และ connector | DPO `R` · PRIVACY `R` · IT `CRUDX` · SEC `R` · AUDIT `R` |  |

## ลำดับการ implement ที่แนะนำ

ทำตาม phase (P0 → P4) ภายใน phase ให้ทำ Must ก่อน และทำ feature ที่เป็น dependency (คอลัมน์ “ขึ้นกับ”) ก่อนเสมอ ก่อนเริ่มแต่ละ feature ให้อ่าน process / state machine ที่เกี่ยวข้องข้างบน

- **P2:** DFG-01, DFG-02, DFG-03, DFG-04, DFG-08
- **P3:** DFG-05, DFG-06, DFG-07, DFG-09
- **P4:** DFG-10, DFG-11

## รายละเอียด feature

<a id="dfg-01"></a>
### DFG-01 สร้างแผนผังอัตโนมัติจาก RoPA

*Auto data flow diagram*

- **Priority / Phase:** Must · P2 · กลุ่ม: แผนผัง
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team), OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก)
- **ขนาดงาน:** BE M (5 วัน) · FE XL (20 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** ROPA-03
- **Process:** —

**คำอธิบาย:** สร้างแผนผังแยกตามหน่วยงาน แสดงการเก็บ บันทึก ใช้ ส่ง/โอน และลบ/ทำลาย

**Backend (Go):** graph API: node (แหล่ง / ระบบ / หน่วยงาน / ผู้รับ / ประเทศ) และ edge (เก็บ / ใช้ / ส่ง / โอน / ทำลาย) สร้างจาก RoPA

**Frontend (Next.js):** React Flow + ELK auto-layout แยก swimlane ตามหน่วยงาน, legend, zoom / filter

**Acceptance criteria:** แผนผังของ 50 กิจกรรมแสดงภายใน 3 วินาทีและตรงกับข้อมูล RoPA

**Implementation (DFG-01, built together with DFG-03) — done:** `internal/dataflow/service.Graph` (no new
migration — `dataflow.layouts`/`.snapshots` already exist for the later DFG-05/DFG-07, unused here) composes
the diagram live from RoPA on every call, the same "never a stale copy" discipline DPIA-04/12 already
established — no caching, no stored snapshot. Node types: `org_unit` (one per department with an activity,
rendered as a swimlane container), `activity` (nested under its org unit, carries code/status/role/purposes/
data categories for the UI's tooltip), `recipient` (one per distinct `org.external_parties` row an
activity's `activity_recipients` references, via `Org.GetExternalParty` — rule 9), `country` (one per
distinct destination), `disposal` (one per unique `retention_rules.disposal_method` an activity names).
Edge types: `send` (activity → recipient), `transfer` (recipient/activity → country, from ROPA-08's own
`activity_transfers`, labelled with `transfer_basis`), `destroy` (activity → its disposal node). DFG-01's
own backend note lists a fifth node type ("ระบบ" / system) sourced from a RoPA asset — `ropa.
processing_activities` has no FK to `ropa.assets` yet (ROPA-02's own register and ROPA-03's own activities
were never linked), so there is nothing to draw; deferred the same way ROPA-01 deferred its own
`discovered_by_finding_id` until a real link exists. **DFG-03's own acceptance criterion is this same pass's
`transfer` edges**: for every recipient whose country isn't Thailand, if no `activity_transfers` row
references that recipient, the edge goes straight to the country node with `warning: true` instead of the
usual label — the exact "implicit, never-logged transfer" check ROPA-08's own `completeness()` already uses
for its `transfer_basis` conditional item, just drawn instead of just flagged; logging the real transfer
clears the warning edge and adds the labelled one. `GET /admin/v1/dataflow/graph` (`ropa.dataflow.read`,
optional `legal_entity_id` filter, no pagination — the whole tenant diagram is meant to be viewed as one
graph) → `internal/dataflow/http`. Frontend: `@xyflow/react` (React Flow) + `elkjs` (bundled, no web worker
needed) — `apps/admin` `/data-flow`, linked from `/ropa/activities`; ELK's own `layered` algorithm lays out
org-unit containers and their nested activity/disposal children, React Flow renders the swimlanes as
`type: "group"` nodes with a small non-interactive label child (React Flow's built-in group type draws no
text of its own); a legend covers every node/edge colour plus the warning style. Tests: unit (swimlanes +
disposal nodes/edges match what was saved; a foreign recipient with no transfer draws the warning edge and a
domestic one never gets a country node at all; logging the transfer clears the warning and adds the real
edge; the `legal_entity_id` filter; two-tenant isolation), HTTP contract (401/403/200). Not done (separate
sibling features, each with its own backlog id and unmet dependency): DFG-02 (auto-refresh on an
`ropa.updated` event — PLT-11 is done but DFG-02 wasn't in this pass's scope), DFG-04 (processor/recipient →
DPA/DSA contract links — depends on DPA-11/DSA-11, neither built), DFG-05 (drag-to-arrange, persisted via
the already-existing `dataflow.layouts`), DFG-08 (image/report export).

<a id="dfg-02"></a>
### DFG-02 อัปเดตแผนผังเมื่อข้อมูลเปลี่ยน

*Auto refresh*

- **Priority / Phase:** Must · P2 · กลุ่ม: แผนผัง
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** SCHED (ระบบ: Scheduler / Event)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DFG-01, PLT-11
- **Process:** —

**คำอธิบาย:** ปรับแผนผังตามการแก้ไข RoPA หรือข้อมูลตั้งต้น โดยไม่ต้องสร้างใหม่

**Backend (Go):** ฟัง event ropa.updated / masterdata.updated → rebuild graph เฉพาะส่วน, คงตำแหน่งที่ผู้ใช้จัดไว้

**Frontend (Next.js):** ป้ายแจ้งว่าแผนผังอัปเดตแล้ว

**Acceptance criteria:** แก้ RoPA แล้วแผนผังเปลี่ยนตามโดยไม่ต้องสร้างใหม่

**Implementation (DFG-02) — done, architecturally trivial given DFG-01:** `dataflowservice.Graph` was built
in DFG-01 to never cache or persist its result — it reads `ropa.processing_activities` and its child tables
fresh on every call (the same discipline `dpiaservice.ActivityDescription`/`Registry` already established)
— so there is no stale copy to rebuild, and no `ropa.updated`/`masterdata.updated` event listener to write:
an edit is visible on the very next read, full stop. The module doc's own backend note describes the
event-driven partial-rebuild machinery a *persisted* snapshot (`dataflow.snapshots`, DFG-07) would need to
stay in sync without recomputing the whole thing — that doesn't apply while DFG-01 never persists anything.
Proven directly (`TestGraph_ReflectsRoPAEditsWithoutRegenerating`): renaming an activity, moving it to a
different department, adding a new one and the previous activity count are all visible on the immediate
next `Graph()` call, with no action in between. What *is* this feature's own real work, the FE note's "ป้าย
แจ้งว่าแผนผังอัปเดตแล้ว" (a badge that the diagram updated) for a viewer who leaves `/data-flow` open while
someone edits RoPA elsewhere: `useDataflowGraph` now polls every 30s (`refetchInterval`, React Query), and
the page shows "last updated HH:MM:SS" (from the response's own `generated_at`) that swaps to "updating..."
during a background refetch, plus a manual "refresh now" button — no WebSocket or SSE push needed for a
30-second staleness budget. Not done: shrinking that budget with a real push channel, which nothing in this
feature's acceptance criterion calls for.

<a id="dfg-03"></a>
### DFG-03 แสดงการโอนไปต่างประเทศ

*Cross-border mapping*

- **Priority / Phase:** Must · P2 · กลุ่ม: แผนผัง
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.28-29; ประกาศ สคส. ตามมาตรา 28 และ 29 พ.ศ. 2566
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** ROPA-08
- **Process:** —

**คำอธิบาย:** ระบุประเทศปลายทางและฐานการโอน (ประเทศที่มีมาตรฐานเพียงพอ, BCR, ข้อสัญญามาตรฐาน, ข้อยกเว้น)

**Backend (Go):** node ประเทศปลายทาง + ป้ายฐานการโอน, เตือนเส้นทางที่ไม่มีฐาน

**Frontend (Next.js):** สัญลักษณ์การโอนต่างประเทศบนแผนผัง

**Acceptance criteria:** เส้นทางโอนที่ไม่มีฐานการโอนแสดงเป็นสีเตือน

**Implementation (DFG-03) — done, see [DFG-01](#dfg-01)'s own implementation note:** built in the same pass
as DFG-01 (same graph, same `transfer` edges) since both share `ROPA-08`'s data and DFG-01 «include»s DFG-03
in the module's own use-case relations above.

<a id="dfg-04"></a>
### DFG-04 แสดงผู้ประมวลผลและผู้รับข้อมูล

*Processor & recipient mapping*

- **Priority / Phase:** Must · P2 · กลุ่ม: แผนผัง
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.40
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** DPA-11, DSA-11
- **Process:** —

**คำอธิบาย:** node ผู้รับข้อมูลบนแผนผัง พร้อมลิงก์ไปยังสัญญา DPA/DSA

**Backend (Go):** node ผู้รับ + ลิงก์ DPA / DSA, เตือนผู้ประมวลผลที่ไม่มี DPA

**Frontend (Next.js):** คลิก node เพื่อดูสัญญา

**Acceptance criteria:** ผู้ประมวลผลที่ไม่มี DPA แสดงเตือนบนแผนผัง

<a id="dfg-08"></a>
### DFG-08 ส่งออกภาพและรายงาน

*Export image & report*

- **Priority / Phase:** Must · P2 · กลุ่ม: ส่งออก
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.39
- **Actor:** DPO (DPO / Privacy Team), AUDIT (ผู้ตรวจสอบ)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** PLT-16
- **Process:** —

**คำอธิบาย:** ส่งออก PNG/PDF กำหนดขนาดภาพ และออกรายงานประกอบ

**Backend (Go):** export PNG / SVG / PDF กำหนดขนาดได้ (render ฝั่ง server ด้วย headless Chromium) + รายงานประกอบ

**Frontend (Next.js):** ตัวเลือกขนาดและรูปแบบไฟล์

**Acceptance criteria:** ภาพที่ส่งออกอ่านได้ชัดในขนาด A3

<a id="dfg-05"></a>
### DFG-05 แก้ไขแผนผังแบบลากวาง

*Drag & drop editor*

- **Priority / Phase:** Should · P3 · กลุ่ม: แก้ไข
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE L (10 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** DFG-01
- **Process:** —

**คำอธิบาย:** ขยับกล่องข้อมูล เพิ่มคำอธิบาย และคืนค่าเดิม

**Backend (Go):** บันทึก layout overlay (ตำแหน่ง / คำอธิบาย) ต่อเวอร์ชัน

**Frontend (Next.js):** ลากวาง, เพิ่มคำอธิบาย, undo / redo, คืนค่าเดิม

**Acceptance criteria:** ตำแหน่งที่จัดเองยังอยู่หลังข้อมูลอัปเดต

<a id="dfg-06"></a>
### DFG-06 มุมมองหลายระดับ

*Multi-view*

- **Priority / Phase:** Should · P3 · กลุ่ม: แก้ไข
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** OWNER (เจ้าของกระบวนการ / ผู้ประสานงานแผนก), DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** DFG-01
- **Process:** —

**คำอธิบาย:** มุมมองรายแผนก รายระบบ รายบริษัท และรายประเภทข้อมูล

**Backend (Go):** query graph หลายมุม: รายแผนก / ระบบ / บริษัท / ประเภทข้อมูล

**Frontend (Next.js):** ตัวเลือกมุมมอง

**Acceptance criteria:** สลับมุมมองได้โดยข้อมูลตรงกัน

<a id="dfg-07"></a>
### DFG-07 เผยแพร่และประวัติเวอร์ชันแผนผัง

*Publish & history*

- **Priority / Phase:** Should · P3 · กลุ่ม: แก้ไข
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวปฏิบัติตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE S (3 วัน) · UX —
- **ขึ้นกับ:** PLT-08
- **Process:** —

**คำอธิบาย:** เผยแพร่แผนผังที่ตรวจแล้ว และดูประวัติการแก้ไข

**Backend (Go):** snapshot แผนผังที่ publish + ประวัติ

**Frontend (Next.js):** ปุ่ม publish + ดูเวอร์ชันเก่า

**Acceptance criteria:** ดูแผนผังที่ publish ย้อนหลังได้

<a id="dfg-09"></a>
### DFG-09 ค้นหาข้อมูลส่วนบุคคลอัตโนมัติ (Data Discovery)

*Data discovery*

- **Priority / Phase:** Should · P3 · กลุ่ม: อัตโนมัติ
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** ม.37(1)
- **Actor:** IT (เจ้าของระบบ / IT), DATASRC (แหล่งข้อมูล (DB / File / Cloud))
- **ขนาดงาน:** BE XL (20 วัน) · FE M (5 วัน) · UX มีหน้าจอใหม่
- **ขึ้นกับ:** PLT-20
- **Process:** —

**คำอธิบาย:** สแกนฐานข้อมูล ไฟล์ และคลาวด์ ตรวจจับข้อมูลแบบไทย (เลขบัตร 13 หลัก เบอร์โทรไทย ชื่อภาษาไทย) แล้วเสนอเพิ่มในแผนผัง

**Backend (Go):** discovery ผ่าน connector: sampling จาก DB / ไฟล์ / S3, classifier ข้อมูลไทย (เลขบัตร 13 หลัก + checksum, เบอร์ไทย, ชื่อ-สกุลไทยด้วยพจนานุกรม / ML), เก็บเฉพาะ metadata ไม่เก็บค่าจริง, เสนอเพิ่มในทะเบียน

**Frontend (Next.js):** หน้าผลสแกน + ยืนยัน / ปฏิเสธข้อเสนอ

**Acceptance criteria:** ตรวจจับเลขบัตรประชาชนในชุดทดสอบได้อย่างน้อย 95% และไม่มีค่าจริงถูกเก็บในระบบ

**หมายเหตุ:** OneTrust ไม่มี classifier ไทย (จุดต่าง)

<a id="dfg-10"></a>
### DFG-10 แผนผังระดับระบบ (Data lineage)

*System-level lineage*

- **Priority / Phase:** Nice · P4 · กลุ่ม: อัตโนมัติ
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** IT (เจ้าของระบบ / IT)
- **ขนาดงาน:** BE M (5 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** ROPA-02
- **Process:** —

**คำอธิบาย:** แสดงการไหลของข้อมูลระหว่างแอปพลิเคชันและฐานข้อมูลจากทะเบียนระบบ

**Backend (Go):** แผนผังระดับระบบจากทะเบียน asset และการเชื่อมต่อ

**Frontend (Next.js):** มุมมอง lineage ระดับระบบ

**Acceptance criteria:** แสดงการไหลระหว่างระบบตรงกับทะเบียน asset

<a id="dfg-11"></a>
### DFG-11 แผนที่โลกของการโอนข้อมูล

*Transfer world map*

- **Priority / Phase:** Nice · P4 · กลุ่ม: อัตโนมัติ
- **ที่มา:** Function List: 12_DataFlow
- **อ้างอิงกฎหมาย / แนวปฏิบัติ:** แนวโน้มตลาด
- **Actor:** DPO (DPO / Privacy Team)
- **ขนาดงาน:** BE S (3 วัน) · FE M (5 วัน) · UX —
- **ขึ้นกับ:** DFG-03
- **Process:** —

**คำอธิบาย:** แสดงเส้นทางการโอนข้อมูลบนแผนที่โลก

**Backend (Go):** ข้อมูลเส้นทางการโอนรายประเทศ

**Frontend (Next.js):** แผนที่โลก (ECharts geo) แสดงเส้นทางการโอน

**Acceptance criteria:** คลิกประเทศแล้วเห็นกิจกรรมที่โอนไปประเทศนั้น
