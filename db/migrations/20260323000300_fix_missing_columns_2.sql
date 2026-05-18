-- +goose Up

-- waiver_acceptances: code uses accepted_by but migration created accepted_by_user_id
ALTER TABLE waiver_acceptances ADD COLUMN IF NOT EXISTS accepted_by UUID REFERENCES users(id) ON DELETE SET NULL;

-- meet_sessions: code uses warm_up_time but migration created warmup_time
ALTER TABLE meet_sessions RENAME COLUMN warmup_time TO warm_up_time;

-- file_exports: code expects size_bytes, expires_at, last_error
ALTER TABLE file_exports ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE file_exports ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;
ALTER TABLE file_exports ADD COLUMN IF NOT EXISTS last_error TEXT;

-- payments: code uses payment_method but migration created method; also expects household_id
ALTER TABLE payments RENAME COLUMN method TO payment_method;
ALTER TABLE payments ADD COLUMN IF NOT EXISTS household_id UUID REFERENCES households(id) ON DELETE SET NULL;

-- data_deletion_requests: code expects expires_at
ALTER TABLE data_deletion_requests ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE data_deletion_requests DROP COLUMN IF EXISTS expires_at;
ALTER TABLE payments DROP COLUMN IF EXISTS household_id;
ALTER TABLE payments RENAME COLUMN payment_method TO method;
ALTER TABLE file_exports DROP COLUMN IF EXISTS last_error;
ALTER TABLE file_exports DROP COLUMN IF EXISTS expires_at;
ALTER TABLE file_exports DROP COLUMN IF EXISTS size_bytes;
ALTER TABLE meet_sessions RENAME COLUMN warm_up_time TO warmup_time;
ALTER TABLE waiver_acceptances DROP COLUMN IF EXISTS accepted_by;
