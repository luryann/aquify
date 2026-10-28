-- +goose Up
CREATE TABLE club_events (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         TEXT        NOT NULL,
    title          TEXT        NOT NULL,
    category       TEXT        NOT NULL CHECK (category IN ('social','fundraiser','team_meeting','clinic','banquet','dryland','other')),
    description    TEXT        NOT NULL DEFAULT '',
    location       TEXT        NOT NULL DEFAULT '',
    starts_at      TIMESTAMPTZ NOT NULL,
    ends_at        TIMESTAMPTZ,
    all_day        BOOLEAN     NOT NULL DEFAULT FALSE,
    audience       TEXT        NOT NULL DEFAULT 'all' CHECK (audience IN ('all','groups')),
    group_codes    TEXT[]      NOT NULL DEFAULT '{}',
    rsvp_enabled   BOOLEAN     NOT NULL DEFAULT FALSE,
    rsvp_mode      TEXT        NOT NULL DEFAULT 'household' CHECK (rsvp_mode IN ('swimmer','household')),
    rsvp_deadline  TIMESTAMPTZ,
    capacity       INTEGER     CHECK (capacity IS NULL OR capacity > 0),
    fee_cents      INTEGER     NOT NULL DEFAULT 0 CHECK (fee_cents >= 0),
    status         TEXT        NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled','cancelled')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by     TEXT        NOT NULL DEFAULT '',
    updated_by     TEXT        NOT NULL DEFAULT '',
    CONSTRAINT club_events_ends_after_start CHECK (ends_at IS NULL OR ends_at >= starts_at),
    CONSTRAINT club_events_fee_requires_rsvp CHECK (fee_cents = 0 OR rsvp_enabled)
);

CREATE INDEX club_events_org_starts_idx ON club_events(org_id, starts_at);

CREATE TABLE club_event_rsvps (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          TEXT        NOT NULL,
    event_id        UUID        NOT NULL REFERENCES club_events(id) ON DELETE CASCADE,
    household_id    TEXT        NOT NULL,
    swimmer_id      TEXT        NOT NULL DEFAULT '',
    response        TEXT        NOT NULL CHECK (response IN ('yes','no','maybe')),
    guest_count     INTEGER     NOT NULL DEFAULT 0 CHECK (guest_count >= 0 AND guest_count <= 50),
    note            TEXT        NOT NULL DEFAULT '',
    invoice_id      TEXT        NOT NULL DEFAULT '',
    billed_quantity INTEGER     NOT NULL DEFAULT 0,
    responded_by    TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT club_event_rsvps_party_unique UNIQUE (event_id, household_id, swimmer_id)
);

CREATE INDEX club_event_rsvps_household_idx ON club_event_rsvps(org_id, household_id);

CREATE TABLE calendar_feed_tokens (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        TEXT        NOT NULL,
    household_id  TEXT        NOT NULL,
    token_hash    TEXT        NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by    TEXT        NOT NULL DEFAULT '',
    last_used_at  TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ
);

-- At most one live feed token per household; rotation revokes the old row.
CREATE UNIQUE INDEX calendar_feed_tokens_active_household_idx
    ON calendar_feed_tokens(org_id, household_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS calendar_feed_tokens;
DROP TABLE IF EXISTS club_event_rsvps;
DROP TABLE IF EXISTS club_events;
