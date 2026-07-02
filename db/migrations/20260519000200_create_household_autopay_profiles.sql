-- +goose Up
CREATE TABLE household_autopay_profiles (
    id                          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                      UUID NOT NULL,
    household_id                UUID NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    enabled                     BOOLEAN NOT NULL DEFAULT false,
    payment_method_type         TEXT NOT NULL DEFAULT '',
    payment_method_brand        TEXT,
    payment_method_last4        TEXT,
    payment_method_label        TEXT NOT NULL DEFAULT '',
    payment_method_external_ref TEXT,
    payment_method_exp_month    INTEGER,
    payment_method_exp_year     INTEGER,
    trigger_days_before_due     INTEGER NOT NULL DEFAULT 0 CHECK (trigger_days_before_due IN (0, 3, 7)),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by                  TEXT,
    UNIQUE (org_id, household_id)
);

CREATE INDEX idx_household_autopay_profiles_org_household
    ON household_autopay_profiles (org_id, household_id);

-- +goose Down
DROP TABLE IF EXISTS household_autopay_profiles;
