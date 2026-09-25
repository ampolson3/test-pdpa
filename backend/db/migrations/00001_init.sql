-- +goose Up
-- generated from the SA data model (same source as backend/db/schema.sql and the ERD pages).
-- After the first deploy, never edit an applied migration: add a new numbered file instead.

-- gen_random_uuid() อยู่ใน core ตั้งแต่ PostgreSQL 13 จึงไม่ต้องใช้ pgcrypto
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;
-- pgvector ใช้กับผู้ช่วย AI (P4): ติดตั้ง extension ก่อนสร้างตาราง gov.kb_chunks
CREATE EXTENSION IF NOT EXISTS vector;

CREATE SCHEMA IF NOT EXISTS platform;
COMMENT ON SCHEMA platform IS 'บริการกลางของแพลตฟอร์ม: tenant, workflow, form, เอกสาร, ไฟล์, แจ้งเตือน, event, audit';
CREATE SCHEMA IF NOT EXISTS iam;
COMMENT ON SCHEMA iam IS 'ผู้ใช้ สิทธิ์ ขอบเขตข้อมูล API client และการยืนยันตัวตน';
CREATE SCHEMA IF NOT EXISTS org;
COMMENT ON SCHEMA org IS 'นิติบุคคล โครงสร้างหน่วยงาน หน่วยงานภายนอก และข้อมูลตั้งต้น';
CREATE SCHEMA IF NOT EXISTS consent;
COMMENT ON SCHEMA consent IS 'ความยินยอมตาม FSD V3.2: Data Element, Purpose, Purpose Preference, Collection Point, Consent Transaction, Reconcile';
CREATE SCHEMA IF NOT EXISTS cookie;
COMMENT ON SCHEMA cookie IS 'โดเมน แบนเนอร์ คุกกี้ ผลสแกน และหลักฐานความยินยอมคุกกี้';
CREATE SCHEMA IF NOT EXISTS notice;
COMMENT ON SCHEMA notice IS 'ประกาศความเป็นส่วนตัว เวอร์ชัน การรับทราบ และการแจ้งตาม ม.25';
CREATE SCHEMA IF NOT EXISTS ropa;
COMMENT ON SCHEMA ropa IS 'RoPA ทะเบียนข้อมูล asset การโอน retention และคลังกิจกรรมมาตรฐาน';
CREATE SCHEMA IF NOT EXISTS dataflow;
COMMENT ON SCHEMA dataflow IS 'แผนผังการไหลของข้อมูลและ data discovery';
CREATE SCHEMA IF NOT EXISTS risk;
COMMENT ON SCHEMA risk IS 'risk matrix ทะเบียนความเสี่ยง control และการวิเคราะห์ช่องว่าง';
CREATE SCHEMA IF NOT EXISTS assess;
COMMENT ON SCHEMA assess IS 'แบบประเมิน DPIA / LIA / TIA / security / maturity (assessment engine)';
CREATE SCHEMA IF NOT EXISTS dsar;
COMMENT ON SCHEMA dsar IS 'คำขอใช้สิทธิของเจ้าของข้อมูล';
CREATE SCHEMA IF NOT EXISTS breach;
COMMENT ON SCHEMA breach IS 'เหตุละเมิดข้อมูลและการแจ้ง สคส. / เจ้าของข้อมูล';
CREATE SCHEMA IF NOT EXISTS vendor;
COMMENT ON SCHEMA vendor IS 'คู่ค้าและผู้ประมวลผล';
CREATE SCHEMA IF NOT EXISTS agreement;
COMMENT ON SCHEMA agreement IS 'ข้อตกลง DPA / DSA (agreement engine)';
CREATE SCHEMA IF NOT EXISTS dpo;
COMMENT ON SCHEMA dpo IS 'งานของ DPO: การแต่งตั้ง งาน คำปรึกษา คลังความรู้';
CREATE SCHEMA IF NOT EXISTS gov;
COMMENT ON SCHEMA gov IS 'อบรม นโยบาย การตรวจประเมิน retention การติดต่อ สคส. และทะเบียน AI';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION platform.set_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN NEW.updated_at := now(); NEW.row_version := OLD.row_version + 1; RETURN NEW; END; $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS platform.set_updated_at();
DROP SCHEMA IF EXISTS gov;
DROP SCHEMA IF EXISTS dpo;
DROP SCHEMA IF EXISTS agreement;
DROP SCHEMA IF EXISTS vendor;
DROP SCHEMA IF EXISTS breach;
DROP SCHEMA IF EXISTS dsar;
DROP SCHEMA IF EXISTS assess;
DROP SCHEMA IF EXISTS risk;
DROP SCHEMA IF EXISTS dataflow;
DROP SCHEMA IF EXISTS ropa;
DROP SCHEMA IF EXISTS notice;
DROP SCHEMA IF EXISTS cookie;
DROP SCHEMA IF EXISTS consent;
DROP SCHEMA IF EXISTS org;
DROP SCHEMA IF EXISTS iam;
DROP SCHEMA IF EXISTS platform;
-- extensions belong to deploy/db/00-bootstrap.sql (superuser) and are intentionally not dropped here
