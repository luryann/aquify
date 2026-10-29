-- +goose Up
-- +goose StatementBegin
-- Last accepted TOTP time-step per factor. A code is consumed with a conditional
-- UPDATE (counter must strictly increase), so a replay is rejected by every API
-- instance and survives restarts.
ALTER TABLE mfa_factors ADD COLUMN IF NOT EXISTS totp_last_counter bigint;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE mfa_factors DROP COLUMN IF EXISTS totp_last_counter;
-- +goose StatementEnd
