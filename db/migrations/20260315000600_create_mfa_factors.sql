-- +goose Up
CREATE TABLE mfa_factors (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  factor_type TEXT NOT NULL CHECK (factor_type IN ('totp', 'webauthn', 'sms', 'email', 'backup_code')),
  label TEXT,
  secret_encrypted TEXT,
  webauthn_credential_id TEXT,
  phone_e164 TEXT,
  email CITEXT,
  is_verified BOOLEAN NOT NULL DEFAULT false,
  verified_at TIMESTAMPTZ,
  last_challenged_at TIMESTAMPTZ,
  disabled_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL,
  CONSTRAINT mfa_factors_factor_identifier_check CHECK (
    (factor_type = 'totp' AND secret_encrypted IS NOT NULL) OR
    (factor_type = 'webauthn' AND webauthn_credential_id IS NOT NULL) OR
    (factor_type = 'sms' AND phone_e164 IS NOT NULL) OR
    (factor_type = 'email' AND email IS NOT NULL) OR
    (factor_type = 'backup_code' AND secret_encrypted IS NOT NULL)
  )
);

CREATE UNIQUE INDEX mfa_factors_webauthn_credential_id_key
  ON mfa_factors (webauthn_credential_id)
  WHERE webauthn_credential_id IS NOT NULL;
CREATE UNIQUE INDEX mfa_factors_phone_e164_user_id_key
  ON mfa_factors (user_id, phone_e164)
  WHERE phone_e164 IS NOT NULL;
CREATE UNIQUE INDEX mfa_factors_email_user_id_key
  ON mfa_factors (user_id, email)
  WHERE email IS NOT NULL;
CREATE INDEX mfa_factors_org_id_idx ON mfa_factors (org_id);
CREATE INDEX mfa_factors_user_id_idx ON mfa_factors (user_id);

CREATE TRIGGER mfa_factors_set_updated_at
BEFORE UPDATE ON mfa_factors
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS mfa_factors;
