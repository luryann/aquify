-- +goose Up

-- eligibility_rules: code uses rule_value but migration created 'parameters'
ALTER TABLE eligibility_rules RENAME COLUMN parameters TO rule_value;

-- meets: code expects venue column (mapped from m.Location)
ALTER TABLE meets ADD COLUMN IF NOT EXISTS venue TEXT;

-- file_exports: code expects filename column
ALTER TABLE file_exports ADD COLUMN IF NOT EXISTS filename TEXT NOT NULL DEFAULT '';

-- announcements: code filters WHERE deleted_at IS NULL
ALTER TABLE announcements ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- email_campaigns: code filters WHERE deleted_at IS NULL
ALTER TABLE email_campaigns ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- data_export_requests: code filters WHERE deleted_at IS NULL
ALTER TABLE data_export_requests ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- data_deletion_requests: code filters WHERE deleted_at IS NULL
ALTER TABLE data_deletion_requests ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE data_deletion_requests DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE data_export_requests DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE email_campaigns DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE announcements DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE file_exports DROP COLUMN IF EXISTS filename;
ALTER TABLE meets DROP COLUMN IF EXISTS venue;
ALTER TABLE eligibility_rules RENAME COLUMN rule_value TO parameters;
