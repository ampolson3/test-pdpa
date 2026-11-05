-- RRA-08 (ยอมรับความเสี่ยงพร้อมผู้อนุมัติ): risk.acceptances records who accepted a risk's own residual, why,
-- and until when — the acceptance criterion itself ("การยอมรับหมดอายุและเตือนทบทวนตามวันที่ตั้ง") is a single
-- scheduled checkpoint at expires_at, not a stored flag: there is no "expired" column, only the date itself.
-- approval_id stays NULL here — this is the approver's own direct act of record (RecordAcceptance is gated
-- by dpo.risk.approve, so the caller already *is* the approver), not a separate async maker-checker round
-- through platform.approvals (PLT-08's own engine, built for versioned documents, not a one-time decision).

-- name: InsertAcceptance :one
INSERT INTO risk.acceptances (id, tenant_id, risk_id, reason, approved_by, approved_at, expires_at, created_by, updated_by)
VALUES ($1, NULLIF(current_setting('app.tenant_id', true), '')::uuid, $2, $3, $4, $5, $6,
        NULLIF(current_setting('app.user_id', true), '')::uuid, NULLIF(current_setting('app.user_id', true), '')::uuid)
RETURNING *;

-- name: GetAcceptance :one
SELECT * FROM risk.acceptances WHERE id = $1;

-- name: ListAcceptancesForRisk :many
SELECT * FROM risk.acceptances WHERE risk_id = $1 ORDER BY created_at DESC;

-- name: GetLatestAcceptanceForRisk :one
-- The acceptance a risk's own "accepted" status currently rests on — the most recently recorded one, so a
-- re-acceptance (a fresh RecordAcceptance call before the previous one expired) always supersedes it.
SELECT * FROM risk.acceptances WHERE risk_id = $1 ORDER BY created_at DESC LIMIT 1;
