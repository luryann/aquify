-- +goose Up
-- Fields the meet entry bundle (SD3/HY3/reports/log) carries that the product
-- did not store yet: the club's mailing address (C1/C2 records and the entry
-- log), a swimmer's middle initial (D0/D1 records) and the meet's per-swimmer
-- surcharge (cost summary on the entry reports).
ALTER TABLE org_settings
    ADD COLUMN address_line1       TEXT NOT NULL DEFAULT '',
    ADD COLUMN address_line2       TEXT NOT NULL DEFAULT '',
    ADD COLUMN address_city        TEXT NOT NULL DEFAULT '',
    ADD COLUMN address_state       TEXT NOT NULL DEFAULT '',
    ADD COLUMN address_postal_code TEXT NOT NULL DEFAULT '';

ALTER TABLE swimmers
    ADD COLUMN middle_initial TEXT NOT NULL DEFAULT '';

ALTER TABLE meets
    ADD COLUMN swimmer_fee_cents INTEGER NOT NULL DEFAULT 0 CHECK (swimmer_fee_cents >= 0);

-- +goose Down
ALTER TABLE meets DROP COLUMN IF EXISTS swimmer_fee_cents;
ALTER TABLE swimmers DROP COLUMN IF EXISTS middle_initial;
ALTER TABLE org_settings
    DROP COLUMN IF EXISTS address_postal_code,
    DROP COLUMN IF EXISTS address_state,
    DROP COLUMN IF EXISTS address_city,
    DROP COLUMN IF EXISTS address_line2,
    DROP COLUMN IF EXISTS address_line1;
