-- +goose Up
-- Allow entries to be soft-scratched: retained for audit/history but excluded
-- from reports/exports.
ALTER TABLE entries DROP CONSTRAINT IF EXISTS entries_status_check;
ALTER TABLE entries ADD CONSTRAINT entries_status_check CHECK (
  status IN ('draft','submitted','approved','exported','updated_from_results','archived','scratched')
);

-- +goose Down
ALTER TABLE entries DROP CONSTRAINT IF EXISTS entries_status_check;
ALTER TABLE entries ADD CONSTRAINT entries_status_check CHECK (
  status IN ('draft','submitted','approved','exported','updated_from_results','archived')
);
