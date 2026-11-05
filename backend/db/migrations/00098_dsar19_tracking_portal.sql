-- +goose Up
-- DSAR-19 (พอร์ทัลติดตามสถานะสำหรับผู้ยื่น, แนวปฏิบัติตลาด): a loginless public portal where the data
-- subject who filed a DSAR request can check its current status and submit additional documents
-- themselves, gated by the exact OTP identity-proof mechanism CON-18/CON-19/CON-23 already established —
-- its own purpose, "dsar_track", so a plain intake/identity OTP (DSAR-01/06's own "dsar" purpose) can never
-- double as proof of someone else's tracking session (the identity gate is re-checked against this exact
-- request's own stored requester_blind_index on every call, not just the OTP's own purpose). The portal's
-- own public key names an org.legal_entities row directly, the same "no dedicated table, the key stands
-- for the entity" pattern CON-18's own preference centre already uses (publickeys.EntityPreferenceCenter) —
-- a request's own legal entity decides which of a tenant's published tracking links a data subject uses,
-- but any of the tenant's own published links works for any of its requests, since the request is resolved
-- by its own number + the requester's own stored contact, not by the key's entity id.
ALTER TABLE iam.subject_verifications DROP CONSTRAINT subject_verifications_purpose_check;
ALTER TABLE iam.subject_verifications ADD CONSTRAINT subject_verifications_purpose_check
    CHECK (purpose IN ('preference_center', 'consent', 'dsar', 'double_opt_in', 'guardian', 'dsar_track'));

ALTER TABLE platform.public_keys DROP CONSTRAINT public_keys_entity_type_check;
ALTER TABLE platform.public_keys ADD CONSTRAINT public_keys_entity_type_check
    CHECK (entity_type IN ('collection_point', 'cookie_domain', 'portal', 'notice', 'dsar_form', 'breach_form', 'dsar_package', 'consent_campaign', 'dsar_track'));

-- +goose Down
ALTER TABLE iam.subject_verifications DROP CONSTRAINT subject_verifications_purpose_check;
ALTER TABLE iam.subject_verifications ADD CONSTRAINT subject_verifications_purpose_check
    CHECK (purpose IN ('preference_center', 'consent', 'dsar', 'double_opt_in', 'guardian'));

ALTER TABLE platform.public_keys DROP CONSTRAINT public_keys_entity_type_check;
ALTER TABLE platform.public_keys ADD CONSTRAINT public_keys_entity_type_check
    CHECK (entity_type IN ('collection_point', 'cookie_domain', 'portal', 'notice', 'dsar_form', 'breach_form', 'dsar_package', 'consent_campaign'));
