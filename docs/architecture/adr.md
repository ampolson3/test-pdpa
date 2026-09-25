# Architecture Decision Records

ADR-01 ถึง ADR-14 มาจาก Development Plan (ชีต Architecture) · ADR-15 ถึง ADR-24 มาจากชุดวิเคราะห์ระบบ (SA) และการ reconcile ตอนสร้างชุดนี้ — ทั้งหมดสถานะ **Accepted (ร่าง)** ให้ยืนยันใน T04 · เปลี่ยนการตัดสินใจ = เพิ่ม ADR ใหม่ที่ supersede ของเดิม ไม่แก้ย้อนหลัง

## ADR-01 รูปแบบสถาปัตยกรรม

- **ตัดสินใจ:** Go modular monolith + worker แยก binary
- **ทางเลือกที่พิจารณา:** Microservices ตั้งแต่ต้น
- **เหตุผล / ผลที่ตามมา:** ทีมขนาดกลางและต้องติดตั้ง on-prem ง่าย; แยก service ภายหลังได้ตามขอบเขตโมดูล (เช่น consent-api)

## ADR-02 Frontend

- **ตัดสินใจ:** Next.js App Router 2 แอป (admin + portal) ใน monorepo Turborepo + pnpm
- **ทางเลือกที่พิจารณา:** SPA (Vite + React) แยกจาก API
- **เหตุผล / ผลที่ตามมา:** SSR/ISR สำหรับหน้า public, BFF เก็บ token ฝั่ง server, ใช้ UI kit และ api-client ร่วมกัน

## ADR-03 สัญญา API

- **ตัดสินใจ:** OpenAPI-first: oapi-codegen (Go) + openapi-typescript (TS)
- **ทางเลือกที่พิจารณา:** gRPC / Connect, GraphQL
- **เหตุผล / ผลที่ตามมา:** public API และ SDK ต้องเป็น REST อยู่แล้ว; generate type ทั้งสองฝั่ง; ใส่ x-permission ใน spec เพื่อตรวจสิทธิ์อัตโนมัติ

## ADR-04 การเข้าถึงข้อมูล

- **ตัดสินใจ:** pgx + sqlc + goose
- **ทางเลือกที่พิจารณา:** GORM, ent
- **เหตุผล / ผลที่ตามมา:** SQL ชัดเจน คุม performance และ RLS ได้ และ type-safe

## ADR-05 Multi-tenancy

- **ตัดสินใจ:** Shared DB + tenant_id + PostgreSQL RLS; ทางเลือก DB แยกต่อ tenant
- **ทางเลือกที่พิจารณา:** schema ต่อ tenant
- **เหตุผล / ผลที่ตามมา:** RLS กันพลาดที่ระดับฐานข้อมูล; ลูกค้าใหญ่/on-prem ใช้ DB แยกด้วยโค้ดเดียวกัน

## ADR-06 Authentication

- **ตัดสินใจ:** Keycloak
- **ทางเลือกที่พิจารณา:** เขียนเองใน Go, Zitadel, บริการ SaaS (Auth0 / Entra External ID)
- **เหตุผล / ผลที่ตามมา:** SAML/OIDC/LDAP/MFA/brute-force พร้อมใช้ ลดความเสี่ยงด้านความปลอดภัย ติดตั้ง on-prem ได้; ต้องมีทักษะดูแล Keycloak (PoC ใน T13)

## ADR-07 Authorization

- **ตัดสินใจ:** เขียนเองใน Go: RBAC + data scope + record rule, cache ใน Valkey
- **ทางเลือกที่พิจารณา:** OPA, Casbin, OpenFGA, Cerbos
- **เหตุผล / ผลที่ตามมา:** โมเดลสิทธิ์ผูกกับโครงสร้างองค์กรและโมดูล; เร็วและทดสอบง่าย; พิจารณา OpenFGA ถ้าต้องแชร์ราย record ซับซ้อน

## ADR-08 Job และ workflow

- **ตัดสินใจ:** River + state machine ของเราเอง
- **ทางเลือกที่พิจารณา:** Temporal, Asynq, Kafka
- **เหตุผล / ผลที่ตามมา:** ใช้ PostgreSQL ที่มีอยู่ enqueue ใน transaction เดียวกับข้อมูล ไม่ต้องเพิ่ม infra; workflow ของงาน PDPA ไม่ซับซ้อนถึงขั้นต้องใช้ Temporal

## ADR-09 เอกสาร PDF / Word

- **ตัดสินใจ:** Gotenberg + template HTML/DOCX + ฟอนต์ไทย
- **ทางเลือกที่พิจารณา:** ไลบรารี commercial (เช่น UniDoc)
- **เหตุผล / ผลที่ตามมา:** open source และแสดงภาษาไทยถูกต้อง; ตรวจ license ไลบรารี DOCX

## ADR-10 Editor ของประกาศและสัญญา

- **ตัดสินใจ:** TipTap (ProseMirror) เก็บเป็น JSON
- **ทางเลือกที่พิจารณา:** Lexical, CKEditor
- **เหตุผล / ผลที่ตามมา:** รองรับ custom node สำหรับ clause และ merge field; ตรวจส่วนขยายที่มี license แยก

## ADR-11 Cookie SDK

- **ตัดสินใจ:** Vanilla TypeScript ไม่มี framework
- **ทางเลือกที่พิจารณา:** React / Preact widget
- **เหตุผล / ผลที่ตามมา:** ต้องเล็กและไม่ชนกับเว็บของลูกค้า

## ADR-12 ค้นหาภาษาไทย

- **ตัดสินใจ:** pg_trgm ก่อน, OpenSearch + Thai analyzer เมื่อจำเป็น
- **ทางเลือกที่พิจารณา:** Elasticsearch ตั้งแต่ต้น
- **เหตุผล / ผลที่ตามมา:** ลด infra ช่วงแรก; ภาษาไทยไม่มีช่องว่างระหว่างคำจึงต้องใช้ trigram หรือ analyzer เฉพาะ

## ADR-13 Hosting

- **ตัดสินใจ:** Kubernetes บน cloud ที่มี data center ในไทย + ชุด on-prem
- **ทางเลือกที่พิจารณา:** cloud ต่างประเทศ
- **เหตุผล / ผลที่ตามมา:** จุดต่างจาก OneTrust (ไม่มี region ในไทย) และตอบโจทย์ภาครัฐ / การเงิน

## ADR-14 AI

- **ตัดสินใจ:** AI gateway กลาง + human-in-the-loop + ปกปิด PII
- **ทางเลือกที่พิจารณา:** เรียก LLM ตรงจากแต่ละโมดูล
- **เหตุผล / ผลที่ตามมา:** คุมความเสี่ยงข้อมูลรั่วและค่าใช้จ่ายในจุดเดียว รองรับ LLM on-prem

## ADR-15 ส่ง event

- **ตัดสินใจ:** Transactional outbox (platform.outbox_events) + River dispatcher
- **ทางเลือกที่พิจารณา:** publish ตรงไป broker ใน request
- **เหตุผล / ผลที่ตามมา:** event ไม่หายและไม่หลุดเมื่อ transaction rollback · ลำดับรับประกันภายใน aggregate · ไม่ต้องเพิ่ม broker ช่วงแรก

## ADR-16 Primary key

- **ตัดสินใจ:** UUIDv7 สร้างในแอป (DEFAULT gen_random_uuid() เป็นค่าสำรอง) · audit_log ใช้ bigint identity
- **ทางเลือกที่พิจารณา:** bigserial ทุกตาราง, UUIDv4
- **เหตุผล / ผลที่ตามมา:** เรียงตามเวลา index ไม่กระจาย · สร้าง id ก่อน insert ได้ (outbox / idempotency) · เดาไม่ได้

## ADR-17 ขอบเขต module ในฐานข้อมูล

- **ตัดสินใจ:** 1 module = 1 schema · ห้าม query ตารางของ module อื่น (เรียก service / รับ event)
- **ทางเลือกที่พิจารณา:** schema เดียวทั้งระบบ
- **เหตุผล / ผลที่ตามมา:** บังคับ bounded context ด้วยโครงสร้าง · แยก service ได้ภายหลัง · ตรวจ import ด้วย depguard

## ADR-18 RLS ในทางปฏิบัติ

- **ตัดสินใจ:** SET LOCAL app.tenant_id ทุก transaction · policy ใช้ NULLIF(current_setting('app.tenant_id', true), '')::uuid · FORCE RLS
- **ทางเลือกที่พิจารณา:** กรอง tenant ในโค้ดอย่างเดียว
- **เหตุผล / ผลที่ตามมา:** connection pool ที่เคยตั้งค่าแล้วคืนค่า '' → NULLIF ทำให้ได้ 0 แถวแทน error · owner ก็ถูกบังคับด้วย FORCE

## ADR-19 หา tenant ของ request สาธารณะ

- **ตัดสินใจ:** platform.public_keys (RLS: public_read อ่านได้ทุก request · tenant_write เขียนได้เฉพาะ tenant เจ้าของ) → SET LOCAL app.tenant_id แล้วจึงทำงาน
- **ทางเลือกที่พิจารณา:** ใช้ DB role ที่ BYPASSRLS ใน public API
- **เหตุผล / ผลที่ตามมา:** public API ไม่มี JWT · ไม่ต้องให้ api ข้าม RLS · key หมุน / เพิกถอนได้ · ผูก allowed_origins

## ADR-20 ตารางปริมาณสูง

- **ตัดสินใจ:** partition รายเดือน (audit_log, security_events, consent_transactions, cookie consent_records) + LFK
- **ทางเลือกที่พิจารณา:** ตารางเดียวขนาดใหญ่
- **เหตุผล / ผลที่ตามมา:** ลบ / archive ตาม retention ด้วย DETACH PARTITION · index เล็ก · FK ไปตาราง partition ใช้ logical reference ตรวจใน service

## ADR-21 ค่าสถานะ consent

- **ตัดสินใจ:** ตัวพิมพ์ใหญ่ตาม FSD V3.2 (ACTIVE, WITHDRAWN, CONSENTED …) ส่วน module อื่นเป็น snake_case
- **ทางเลือกที่พิจารณา:** เปลี่ยนเป็นตัวเล็กทั้งหมด
- **เหตุผล / ผลที่ตามมา:** เข้ากันกับระบบ Consent เดิม (.NET) ข้อมูลที่ย้ายมา และ integration ที่ใช้อยู่ (T03 / T26)

## ADR-22 รูปแบบ error

- **ตัดสินใจ:** RFC 9457 problem+json พร้อม `code` คงที่ต่อกรณี (เช่น consent.purpose_inactive, authz.denied)
- **ทางเลือกที่พิจารณา:** error string อิสระ
- **เหตุผล / ผลที่ตามมา:** frontend / integrator แปลข้อความและตัดสินใจจาก code ได้ · ทดสอบได้

## ADR-23 หลักฐานที่แก้ไขไม่ได้

- **ตัดสินใจ:** append-only + hash chain (prev_hash → hash) สำหรับ consent_receipts และ audit_log · object lock สำหรับไฟล์หลักฐาน
- **ทางเลือกที่พิจารณา:** log ธรรมดา
- **เหตุผล / ผลที่ตามมา:** พิสูจน์ต่อ สคส. / ศาลได้ว่าไม่ถูกแก้ไขย้อนหลัง

## ADR-24 โครงสร้าง repository

- **ตัดสินใจ:** monorepo เดียว: apps/*, packages/* (pnpm + Turborepo) + backend/ (Go module) + api/openapi + deploy + infra + tests
- **ทางเลือกที่พิจารณา:** แยก repo frontend / backend
- **เหตุผล / ผลที่ตามมา:** เปลี่ยน OpenAPI แล้ว generate ทั้งสองฝั่งใน PR เดียว · CI ตรวจ codegen diff
