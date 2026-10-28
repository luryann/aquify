-- +goose Up
-- Club finance: fee schedules, sibling discount rules, manual adjustments,
-- idempotent billing runs, household account credits, late fees.
-- All money is integer cents.

-- Widen existing billing vocabularies for the new line/payment/ledger kinds.
ALTER TABLE invoice_items DROP CONSTRAINT IF EXISTS invoice_items_category_check;
ALTER TABLE invoice_items ADD CONSTRAINT invoice_items_category_check CHECK (
  category IN ('program_fee','meet_fee','equipment','manual','late_fee','credit',
               'registration_fee','discount','adjustment')
);

ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_method_check;
ALTER TABLE payments ADD CONSTRAINT payments_method_check CHECK (
  payment_method IN ('stripe','check','cash','ach','other','account_credit')
);

ALTER TABLE ledger_events DROP CONSTRAINT IF EXISTS ledger_events_type_check;
ALTER TABLE ledger_events ADD CONSTRAINT ledger_events_type_check CHECK (
  event_type IN (
    'invoice_issued','payment_succeeded','payment_failed','refund_processed',
    'invoice_voided','invoice_written_off',
    'late_fee_assessed','credit_issued','credit_applied','credit_debited'
  )
);
ALTER TABLE ledger_events ADD COLUMN IF NOT EXISTS household_id TEXT;
CREATE INDEX IF NOT EXISTS ledger_events_org_household_idx
  ON ledger_events (org_id, household_id) WHERE household_id IS NOT NULL;

CREATE TABLE billing_fee_schedules (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        UUID NOT NULL,
  name          TEXT NOT NULL CHECK (btrim(name) <> ''),
  kind          TEXT NOT NULL CHECK (kind IN ('dues','registration')),
  group_code    TEXT NOT NULL DEFAULT '',
  season        TEXT NOT NULL DEFAULT '',
  frequency     TEXT NOT NULL CHECK (frequency IN ('monthly','quarterly','annual','one_time')),
  amount_cents  INT  NOT NULL CHECK (amount_cents > 0),
  start_date    DATE NOT NULL,
  end_date      DATE,
  proration     TEXT NOT NULL DEFAULT 'none' CHECK (proration IN ('none','daily','half','waive')),
  auto_bill     BOOLEAN NOT NULL DEFAULT false,
  active        BOOLEAN NOT NULL DEFAULT true,
  description   TEXT NOT NULL DEFAULT '',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by    TEXT NOT NULL DEFAULT '',
  updated_by    TEXT NOT NULL DEFAULT '',
  CONSTRAINT billing_fee_schedules_dates_check CHECK (end_date IS NULL OR end_date >= start_date),
  CONSTRAINT billing_fee_schedules_dues_group_check CHECK (kind <> 'dues' OR group_code <> ''),
  CONSTRAINT billing_fee_schedules_registration_check CHECK (
    kind <> 'registration' OR (season <> '' AND frequency = 'one_time')
  )
);
CREATE INDEX billing_fee_schedules_org_idx ON billing_fee_schedules (org_id, active);

CREATE TABLE billing_discount_rules (
  id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id         UUID NOT NULL,
  name           TEXT NOT NULL CHECK (btrim(name) <> ''),
  applies_to     TEXT NOT NULL CHECK (applies_to IN ('dues','registration','all')),
  min_position   INT  NOT NULL CHECK (min_position >= 2),
  max_position   INT  CHECK (max_position IS NULL OR max_position >= min_position),
  discount_type  TEXT NOT NULL CHECK (discount_type IN ('percent','fixed')),
  percent_bps    INT  NOT NULL DEFAULT 0 CHECK (percent_bps BETWEEN 0 AND 10000),
  amount_cents   INT  NOT NULL DEFAULT 0 CHECK (amount_cents >= 0),
  active         BOOLEAN NOT NULL DEFAULT true,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by     TEXT NOT NULL DEFAULT '',
  updated_by     TEXT NOT NULL DEFAULT '',
  CONSTRAINT billing_discount_rules_value_check CHECK (
    (discount_type = 'percent' AND percent_bps > 0) OR
    (discount_type = 'fixed' AND amount_cents > 0)
  )
);
CREATE INDEX billing_discount_rules_org_idx ON billing_discount_rules (org_id, active);

CREATE TABLE billing_runs (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id            UUID NOT NULL,
  as_of             DATE NOT NULL,
  invoice_status    TEXT NOT NULL CHECK (invoice_status IN ('draft','issued')),
  due_date          DATE,
  schedule_ids      TEXT[] NOT NULL DEFAULT '{}',
  trigger           TEXT NOT NULL CHECK (trigger IN ('manual','scheduler')),
  fingerprint       TEXT NOT NULL DEFAULT '',
  household_count   INT NOT NULL DEFAULT 0,
  invoice_count     INT NOT NULL DEFAULT 0,
  charge_count      INT NOT NULL DEFAULT 0,
  skipped_count     INT NOT NULL DEFAULT 0,
  gross_cents       INT NOT NULL DEFAULT 0,
  discount_cents    INT NOT NULL DEFAULT 0,
  adjustment_cents  INT NOT NULL DEFAULT 0,
  credit_applied_cents INT NOT NULL DEFAULT 0,
  net_cents         INT NOT NULL DEFAULT 0,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX billing_runs_org_created_idx ON billing_runs (org_id, created_at DESC);

-- One row per billed (schedule, period, household, swimmer). The partial
-- unique index is the cross-instance double-billing guard; voiding an invoice
-- releases its charges so the period can be re-billed deliberately.
CREATE TABLE billing_charges (
  id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id           UUID NOT NULL,
  run_id           UUID NOT NULL REFERENCES billing_runs(id) ON DELETE RESTRICT,
  invoice_id       UUID NOT NULL REFERENCES invoices(id) ON DELETE RESTRICT,
  household_id     TEXT NOT NULL,
  swimmer_id       TEXT NOT NULL DEFAULT '',
  schedule_id      UUID NOT NULL REFERENCES billing_fee_schedules(id) ON DELETE RESTRICT,
  period_key       TEXT NOT NULL,
  period_start     DATE NOT NULL,
  period_end       DATE NOT NULL,
  gross_cents      INT NOT NULL CHECK (gross_cents >= 0),
  discount_cents   INT NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
  net_cents        INT NOT NULL CHECK (net_cents >= 0),
  sibling_position INT NOT NULL DEFAULT 1 CHECK (sibling_position >= 1),
  released_at      TIMESTAMPTZ,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX billing_charges_idempotency_key
  ON billing_charges (org_id, schedule_id, period_key, household_id, swimmer_id)
  WHERE released_at IS NULL;
CREATE INDEX billing_charges_invoice_idx ON billing_charges (invoice_id);
CREATE INDEX billing_charges_run_idx ON billing_charges (run_id);

CREATE TABLE billing_adjustments (
  id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id             UUID NOT NULL,
  household_id       TEXT NOT NULL,
  swimmer_id         TEXT NOT NULL DEFAULT '',
  amount_cents       INT  NOT NULL CHECK (amount_cents <> 0),
  reason             TEXT NOT NULL CHECK (btrim(reason) <> ''),
  status             TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','applied','voided')),
  applied_invoice_id UUID REFERENCES invoices(id) ON DELETE SET NULL,
  applied_run_id     UUID REFERENCES billing_runs(id) ON DELETE SET NULL,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by         TEXT NOT NULL DEFAULT '',
  updated_by         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX billing_adjustments_org_status_idx ON billing_adjustments (org_id, status);

-- Household account credit: balance row (non-negative, DB-enforced) plus an
-- append-only entry trail.
CREATE TABLE household_credit_balances (
  org_id        UUID NOT NULL,
  household_id  TEXT NOT NULL,
  balance_cents INT  NOT NULL DEFAULT 0 CHECK (balance_cents >= 0),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, household_id)
);

CREATE TABLE household_credit_entries (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id              UUID NOT NULL,
  household_id        TEXT NOT NULL,
  amount_cents        INT  NOT NULL CHECK (amount_cents <> 0),
  kind                TEXT NOT NULL CHECK (kind IN ('manual_credit','manual_debit','overpayment','refund_to_credit','applied')),
  invoice_id          UUID REFERENCES invoices(id) ON DELETE SET NULL,
  payment_id          UUID REFERENCES payments(id) ON DELETE SET NULL,
  refund_id           UUID REFERENCES refunds(id) ON DELETE SET NULL,
  reason              TEXT NOT NULL DEFAULT '',
  balance_after_cents INT  NOT NULL CHECK (balance_after_cents >= 0),
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX household_credit_entries_household_idx ON household_credit_entries (org_id, household_id, created_at);

CREATE TABLE billing_settings (
  org_id                   UUID PRIMARY KEY,
  late_fee_enabled         BOOLEAN NOT NULL DEFAULT false,
  late_fee_grace_days      INT NOT NULL DEFAULT 7 CHECK (late_fee_grace_days BETWEEN 0 AND 365),
  late_fee_type            TEXT NOT NULL DEFAULT 'flat' CHECK (late_fee_type IN ('flat','percent')),
  late_fee_flat_cents      INT NOT NULL DEFAULT 0 CHECK (late_fee_flat_cents >= 0),
  late_fee_percent_bps     INT NOT NULL DEFAULT 0 CHECK (late_fee_percent_bps BETWEEN 0 AND 10000),
  late_fee_max_cents       INT NOT NULL DEFAULT 0 CHECK (late_fee_max_cents >= 0),
  auto_run_enabled         BOOLEAN NOT NULL DEFAULT false,
  auto_run_day             INT NOT NULL DEFAULT 1 CHECK (auto_run_day BETWEEN 1 AND 28),
  auto_run_invoice_status  TEXT NOT NULL DEFAULT 'draft' CHECK (auto_run_invoice_status IN ('draft','issued')),
  auto_run_due_days        INT NOT NULL DEFAULT 15 CHECK (auto_run_due_days BETWEEN 0 AND 120),
  updated_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_by               TEXT NOT NULL DEFAULT ''
);

-- One late fee per invoice, ever (idempotency guard for the scheduled job).
CREATE TABLE billing_late_fee_applications (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        UUID NOT NULL,
  invoice_id    UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  item_id       UUID NOT NULL,
  amount_cents  INT  NOT NULL CHECK (amount_cents > 0),
  applied_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT billing_late_fee_applications_invoice_unique UNIQUE (org_id, invoice_id)
);

-- +goose Down
DROP TABLE IF EXISTS billing_late_fee_applications;
DROP TABLE IF EXISTS billing_settings;
DROP TABLE IF EXISTS household_credit_entries;
DROP TABLE IF EXISTS household_credit_balances;
DROP TABLE IF EXISTS billing_adjustments;
DROP TABLE IF EXISTS billing_charges;
DROP TABLE IF EXISTS billing_runs;
DROP TABLE IF EXISTS billing_discount_rules;
DROP TABLE IF EXISTS billing_fee_schedules;

DROP INDEX IF EXISTS ledger_events_org_household_idx;
ALTER TABLE ledger_events DROP COLUMN IF EXISTS household_id;

-- Restore the original vocabularies. NOT VALID keeps the rollback working when
-- rows written under the wider vocabulary still exist.
ALTER TABLE ledger_events DROP CONSTRAINT IF EXISTS ledger_events_type_check;
ALTER TABLE ledger_events ADD CONSTRAINT ledger_events_type_check CHECK (
  event_type IN ('invoice_issued','payment_succeeded','payment_failed','refund_processed',
                 'invoice_voided','invoice_written_off')
) NOT VALID;

ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_method_check;
ALTER TABLE payments ADD CONSTRAINT payments_method_check CHECK (
  payment_method IN ('stripe','check','cash','ach','other')
) NOT VALID;

ALTER TABLE invoice_items DROP CONSTRAINT IF EXISTS invoice_items_category_check;
ALTER TABLE invoice_items ADD CONSTRAINT invoice_items_category_check CHECK (
  category IN ('program_fee','meet_fee','equipment','manual','late_fee','credit')
) NOT VALID;
