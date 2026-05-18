-- +goose Up

-- Normalize any drifted invoice due_date values so empty strings become NULL
-- before the application reads the column as a date.
ALTER TABLE invoices
  ALTER COLUMN due_date TYPE DATE
  USING NULLIF(BTRIM(due_date::text), '')::date;

-- +goose Down

ALTER TABLE invoices
  ALTER COLUMN due_date TYPE TEXT
  USING due_date::text;
