-- +goose Up
-- The erasure worker records a failed run as status 'failed' (so it can be
-- retried), but the original constraint omitted it and the write was rejected.
ALTER TABLE data_deletion_requests DROP CONSTRAINT IF EXISTS data_deletion_requests_status_check;
ALTER TABLE data_deletion_requests ADD CONSTRAINT data_deletion_requests_status_check
  CHECK (status IN ('pending', 'reviewing', 'approved', 'processing', 'completed', 'rejected', 'failed'));

-- +goose Down
UPDATE data_deletion_requests SET status = 'pending' WHERE status = 'failed';
ALTER TABLE data_deletion_requests DROP CONSTRAINT IF EXISTS data_deletion_requests_status_check;
ALTER TABLE data_deletion_requests ADD CONSTRAINT data_deletion_requests_status_check
  CHECK (status IN ('pending', 'reviewing', 'approved', 'processing', 'completed', 'rejected'));
