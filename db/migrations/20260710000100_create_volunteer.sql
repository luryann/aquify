-- +goose Up
-- Volunteer jobs & household service hours (TeamUnify "Job Signup" parity).

-- Org-level defaults; seasons and households may override requirements.
CREATE TABLE volunteer_settings (
    org_id                         TEXT         PRIMARY KEY,
    default_required_hours         NUMERIC(6,2) NOT NULL DEFAULT 0 CHECK (default_required_hours >= 0),
    default_buyout_cents_per_hour  INTEGER      NOT NULL DEFAULT 0 CHECK (default_buyout_cents_per_hour >= 0),
    cancellation_cutoff_hours      INTEGER      NOT NULL DEFAULT 48 CHECK (cancellation_cutoff_hours BETWEEN 0 AND 720),
    reminders_enabled              BOOLEAN      NOT NULL DEFAULT TRUE,
    timezone                       TEXT         NOT NULL DEFAULT 'UTC',
    updated_at                     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_by                     TEXT         NOT NULL DEFAULT ''
);

-- A job slot: a role with N spots at a meet or a standalone club event.
CREATE TABLE volunteer_jobs (
    id                     UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                 TEXT         NOT NULL,
    meet_id                TEXT,
    event_name             TEXT         NOT NULL DEFAULT '',
    season_id              UUID,
    role                   TEXT         NOT NULL CHECK (role IN ('timer','concessions','marshal','official','hospitality','setup','admin','other')),
    title                  TEXT         NOT NULL,
    description            TEXT         NOT NULL DEFAULT '',
    location               TEXT         NOT NULL DEFAULT '',
    starts_at              TIMESTAMPTZ  NOT NULL,
    ends_at                TIMESTAMPTZ  NOT NULL,
    capacity               INTEGER      NOT NULL CHECK (capacity BETWEEN 1 AND 500),
    hour_credit            NUMERIC(5,2) NOT NULL CHECK (hour_credit >= 0 AND hour_credit <= 24),
    min_age                INTEGER      CHECK (min_age IS NULL OR min_age BETWEEN 1 AND 99),
    requires_certification BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by             TEXT         NOT NULL DEFAULT '',
    updated_by             TEXT         NOT NULL DEFAULT '',
    CONSTRAINT volunteer_jobs_time_order CHECK (ends_at > starts_at),
    CONSTRAINT volunteer_jobs_anchor CHECK (meet_id IS NOT NULL OR event_name <> '')
);

CREATE INDEX volunteer_jobs_org_start_idx ON volunteer_jobs (org_id, starts_at);
CREATE INDEX volunteer_jobs_org_meet_idx ON volunteer_jobs (org_id, meet_id) WHERE meet_id IS NOT NULL;

CREATE TABLE volunteer_signups (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           TEXT         NOT NULL,
    job_id           UUID         NOT NULL REFERENCES volunteer_jobs(id) ON DELETE CASCADE,
    household_id     TEXT         NOT NULL,
    volunteer_name   TEXT         NOT NULL CHECK (volunteer_name <> ''),
    swimmer_id       TEXT,
    status           TEXT         NOT NULL CHECK (status IN ('confirmed','cancelled','worked','no_show')),
    hours_credited   NUMERIC(5,2) CHECK (hours_credited IS NULL OR (hours_credited >= 0 AND hours_credited <= 24)),
    notes            TEXT         NOT NULL DEFAULT '',
    signed_up_by     TEXT         NOT NULL DEFAULT '',
    cancelled_at     TIMESTAMPTZ,
    cancelled_by     TEXT         NOT NULL DEFAULT '',
    checked_in_at    TIMESTAMPTZ,
    checked_in_by    TEXT         NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX volunteer_signups_job_idx ON volunteer_signups (job_id);
CREATE INDEX volunteer_signups_household_idx ON volunteer_signups (org_id, household_id);
-- The same person cannot hold two live spots on one job.
CREATE UNIQUE INDEX volunteer_signups_live_person_uniq
    ON volunteer_signups (job_id, household_id, lower(volunteer_name))
    WHERE status <> 'cancelled';

-- Season requirement (household_id NULL) or per-household override.
CREATE TABLE volunteer_requirements (
    id                     UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                 TEXT         NOT NULL,
    season_id              UUID         NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    household_id           TEXT,
    required_hours         NUMERIC(6,2) NOT NULL CHECK (required_hours >= 0),
    buyout_cents_per_hour  INTEGER      NOT NULL CHECK (buyout_cents_per_hour >= 0),
    notes                  TEXT         NOT NULL DEFAULT '',
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_by             TEXT         NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX volunteer_requirements_scope_uniq
    ON volunteer_requirements (org_id, season_id, household_id) NULLS NOT DISTINCT;

-- Manual hour adjustments (positive or negative) made by staff.
CREATE TABLE volunteer_credits (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        TEXT         NOT NULL,
    season_id     UUID         NOT NULL,
    household_id  TEXT         NOT NULL,
    hours         NUMERIC(6,2) NOT NULL CHECK (hours <> 0 AND hours BETWEEN -200 AND 200),
    reason        TEXT         NOT NULL CHECK (reason <> ''),
    created_by    TEXT         NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX volunteer_credits_scope_idx ON volunteer_credits (org_id, season_id, household_id);

-- One buyout per household per season. A row is reserved ('pending') before
-- the invoice is created so concurrent requests cannot double-bill.
CREATE TABLE volunteer_buyouts (
    id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              TEXT         NOT NULL,
    season_id           UUID         NOT NULL,
    household_id        TEXT         NOT NULL,
    status              TEXT         NOT NULL CHECK (status IN ('pending','billed')),
    hours_short         NUMERIC(6,2) NOT NULL CHECK (hours_short > 0),
    rate_cents_per_hour INTEGER      NOT NULL CHECK (rate_cents_per_hour > 0),
    amount_cents        INTEGER      NOT NULL CHECK (amount_cents > 0),
    invoice_id          TEXT         NOT NULL DEFAULT '',
    invoice_item_id     TEXT         NOT NULL DEFAULT '',
    created_by          TEXT         NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT volunteer_buyouts_scope_uniq UNIQUE (org_id, season_id, household_id)
);

-- +goose Down
DROP TABLE IF EXISTS volunteer_buyouts;
DROP TABLE IF EXISTS volunteer_credits;
DROP TABLE IF EXISTS volunteer_requirements;
DROP TABLE IF EXISTS volunteer_signups;
DROP TABLE IF EXISTS volunteer_jobs;
DROP TABLE IF EXISTS volunteer_settings;
