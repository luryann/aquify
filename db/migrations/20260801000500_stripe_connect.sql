-- +goose Up
-- Tenancy tracker item 5.1: one Stripe connected account per club, so each club's
-- payments and payouts are independent of every other club.
CREATE TABLE org_payment_accounts (
    org_id             UUID        PRIMARY KEY,
    stripe_account_id  TEXT        NOT NULL,
    charges_enabled    BOOLEAN     NOT NULL DEFAULT FALSE,
    payouts_enabled    BOOLEAN     NOT NULL DEFAULT FALSE,
    details_submitted  BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT org_payment_accounts_org_fk FOREIGN KEY (org_id) REFERENCES orgs (id) ON DELETE RESTRICT,
    -- A Stripe account can belong to exactly one club.
    CONSTRAINT org_payment_accounts_account_uidx UNIQUE (stripe_account_id)
);

CREATE TRIGGER org_payment_accounts_set_updated_at
BEFORE UPDATE ON org_payment_accounts
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- The account a payment was taken on (NULL = the platform account, i.e. every
-- payment made before the club connected its own). Refunds go back to it.
ALTER TABLE payments ADD COLUMN stripe_account_id TEXT;

-- +goose Down
ALTER TABLE payments DROP COLUMN IF EXISTS stripe_account_id;
DROP TABLE IF EXISTS org_payment_accounts;
