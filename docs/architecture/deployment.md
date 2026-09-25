# Deployment และ CI/CD

> ต้นฉบับ: ARC-02 และ ARC-06 ใน `design/PDPA_System_Analysis.drawio`

## Kubernetes (production · TH region · 3 AZ)

| namespace | workload |
|---|---|
| `ingress` | ingress-nginx ×2 (TLS termination) · cert-manager |
| `web` | admin-web HPA 2–6 · portal-web HPA 2–10 |
| `app` | api HPA 3–12 (PDB minAvailable 2) · worker HPA 2–8 ตาม queue depth · scanner 1–4 (node pool แยก + egress proxy) · gotenberg ×2 · clamav ×2 · migrate (Job, Argo CD PreSync) |
| `identity` | keycloak ×2 HA + PostgreSQL แยก (CloudNativePG) |
| `security` | openbao ×3 (raft, auto-unseal KMS) · external-secrets |
| `observability` | otel-collector (DaemonSet) · prometheus + alertmanager · grafana · loki · tempo |
| `argocd` | argo-cd (app-of-apps) · argo-rollouts (canary api / web) |

ทุก namespace ใช้ NetworkPolicy แบบ default-deny · secret จาก OpenBao ผ่าน external-secrets (ไม่มี secret ใน git)

## Data tier

- PostgreSQL 16+ (CloudNativePG หรือ managed): primary + 2 replicas (sync 1) · PITR ด้วย WAL → object storage · backup เก็บ 35 วัน
- extension ที่ต้องมี: citext, ltree, pg_trgm, vector (pgvector) — ตรวจกับ managed service ก่อนเลือก provider
- Valkey: Sentinel ×3 · AOF
- Object storage: S3 / MinIO (erasure coding) · versioning · object lock สำหรับไฟล์หลักฐาน
- DR: warm standby ใน region / data center ที่ 2 · PostgreSQL replica (async) · bucket replication · RPO ≤ 15 นาที · RTO ≤ 4 ชม.

## Environment

| env | deploy | ข้อมูล | integration | ผู้ใช้ |
|---|---|---|---|---|
| dev | auto จาก main ทุก commit | ข้อมูลสังเคราะห์ (seed) | mock / sandbox ทั้งหมด | ทีมพัฒนา |
| sit | promote tag รายวัน | ข้อมูลสังเคราะห์ | Keycloak test realm · SMS / e-Sign sandbox | QA |
| uat | promote tag + approval | ข้อมูลที่ผ่าน masking เท่านั้น | ใกล้เคียง prod (sandbox ภายนอก) | ผู้ใช้ธุรกิจ / DPO |
| prod | approval + change window · canary 10 → 50 → 100% | ข้อมูลจริง (TH region) | production | ลูกค้า |
| dr | sync manifest ต่อเนื่อง | replica ของ prod | standby | — |

## CI/CD pipeline

| # | ขั้น | รายละเอียด |
|---|---|---|
| 1 | Commit / PR | trunk-based · PR ≥ 1 reviewer |
| 2 | Lint | golangci-lint · eslint · sqlfluff |
| 3 | Codegen check | oapi-codegen · sqlc · openapi-typescript ต้องไม่มี diff |
| 4 | Unit test | go test -race · vitest · coverage ≥ 70% |
| 5 | Build | Go binaries · Next.js · SDK → OCI images |
| 6 | Security scan | govulncheck · osv-scanner · semgrep · trivy image |
| 7 | Integration test | testcontainers PostgreSQL + Valkey |
| 8 | SBOM + sign | syft · cosign → registry |
| 9 | Deploy dev | Argo CD auto-sync · PreSync: goose up |
| 10 | E2E | Playwright (admin + portal) · API contract |
| 11 | SIT → UAT | promote tag · manual approval |
| 12 | Prod | change window · canary 10→50→100% · smoke test |

## Database migration

- ครั้งแรกต่อ environment: `deploy/db/00-bootstrap.sql` (superuser: role, database, extension)
- `cmd/migrate` (Argo CD PreSync Job): เชื่อมต่อเป็น `pdpa_migrator` ด้วย `options=-c role=pdpa_owner` → `goose up` → River migrations → `deploy/db/10-grants.sql`
- baseline: `00001_init` (schema, function) · `00002`–`00017` (1 ไฟล์ต่อ schema) · `00018_foreign_keys` · `00019_seed_iam_permissions` · `00020_partition_maintenance`
- ไฟล์ที่ deploy แล้วห้ามแก้ — เปลี่ยน schema = ไฟล์ใหม่ลำดับถัดไป (`goose -s create <name> sql` ใช้เลขเรียงแทน timestamp)
- expand → migrate data → contract: เพิ่มคอลัมน์ / ตารางก่อน, deploy โค้ดที่รองรับทั้งสองแบบ, ย้ายข้อมูลด้วย job, ลบของเก่าใน release ถัดไป
- ห้าม lock ตารางใหญ่นาน: index ใหม่บนตารางใหญ่ใช้ `CREATE INDEX CONCURRENTLY` ในไฟล์ที่มี `-- +goose NO TRANSACTION`
- ตาราง partition ใช้ CONCURRENTLY กับตารางแม่ไม่ได้: `CREATE INDEX … ON ONLY <parent>` → `CREATE INDEX CONCURRENTLY` ทีละ partition → `ALTER INDEX <parent_idx> ATTACH PARTITION <partition_idx>`
- ตาราง append-only หรือ global ใหม่ต้องเพิ่มใน REVOKE ของ `deploy/db/10-grants.sql` ใน PR เดียวกัน
- partition รายเดือนสร้างล่วงหน้า 3 เดือนด้วย job `partition.maintain` ที่เรียก `SELECT platform.ensure_monthly_partitions(3)` ทุกวัน (ถ้าเดือนนั้นมีแถวค้างใน default partition จะ error — ตั้ง alert)
- ทุก migration ต้องมี `-- +goose Down` ที่ย้อนได้จริง และผ่าน integration test up → down → up

## Observability

- OpenTelemetry SDK ใน Go และ Next.js → otel-collector → Prometheus (metrics) · Loki (log JSON, ไม่มี PII) · Tempo (trace)
- alert ขั้นต่ำ: error rate / latency ของ API, queue depth ของ River, webhook dead, SLA DSAR ใกล้ครบ, breach 72 ชม., login ผิดปกติ, unmask / export จำนวนมาก, backup ล้มเหลว
- on-call runbook อยู่ใน T18 · DR runbook ใน T19
