-- +goose Up
CREATE TABLE IF NOT EXISTS mfa_factors (
    id                 TEXT        PRIMARY KEY,
    user_id            TEXT        NOT NULL,
    org_id             TEXT        NOT NULL,
    factor_type        TEXT        NOT NULL CHECK (factor_type IN ('totp', 'backup_code')),
    label              TEXT        NOT NULL DEFAULT '',
    is_verified        BOOLEAN     NOT NULL DEFAULT false,
    totp_secret        TEXT,
    backup_codes       JSONB,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_challenged_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS mfa_factors_user_id_idx ON mfa_factors (user_id);

-- +goose Down
DROP TABLE IF EXISTS mfa_factors;
