-- +goose Up
-- DSAR-20: configurable workflow per right type — steps (the default subtask action DSAR-03/08's own
-- hardcoded rightSubtaskAction map currently bakes in at the Go level), responsible (a default assignee for
-- new requests of this type) and response letter (an optional per-type/per-purpose override code, resolved
-- the same way DSAR-13's own shared purpose templates already are). All three are read fresh on every
-- CreateRequest/Transition call, so a DPO's own change here takes effect for the very next request with no
-- redeploy — the acceptance criterion itself.
ALTER TABLE dsar.request_types ADD COLUMN default_subtask_action text;
ALTER TABLE dsar.request_types ADD COLUMN default_assignee_user_id uuid;
ALTER TABLE dsar.request_types ADD COLUMN response_code_result text;
ALTER TABLE dsar.request_types ADD COLUMN response_code_rejection text;
ALTER TABLE dsar.request_types ADD COLUMN response_code_request_info text;
ALTER TABLE dsar.request_types ADD CONSTRAINT fk_request_types_default_assignee_user_id
    FOREIGN KEY (default_assignee_user_id) REFERENCES iam.users (id);

-- +goose Down
ALTER TABLE dsar.request_types DROP CONSTRAINT IF EXISTS fk_request_types_default_assignee_user_id;
ALTER TABLE dsar.request_types DROP COLUMN IF EXISTS response_code_request_info;
ALTER TABLE dsar.request_types DROP COLUMN IF EXISTS response_code_rejection;
ALTER TABLE dsar.request_types DROP COLUMN IF EXISTS response_code_result;
ALTER TABLE dsar.request_types DROP COLUMN IF EXISTS default_assignee_user_id;
ALTER TABLE dsar.request_types DROP COLUMN IF EXISTS default_subtask_action;
