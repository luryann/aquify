-- +goose Up
-- The host's age-up date: swimmer ages in entry files are computed on it, which
-- can differ from the meet's start date. NULL means "use the start date".
ALTER TABLE meets ADD COLUMN age_up_date DATE;

-- +goose Down
ALTER TABLE meets DROP COLUMN IF EXISTS age_up_date;
