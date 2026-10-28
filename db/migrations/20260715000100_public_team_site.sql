-- +goose Up
ALTER TABLE org_settings
    ADD COLUMN public_slug          TEXT    NOT NULL DEFAULT '',
    ADD COLUMN public_site_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN public_show_coaches  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN public_about         TEXT    NOT NULL DEFAULT '',
    ADD COLUMN public_contact_email TEXT    NOT NULL DEFAULT '',
    ADD COLUMN public_contact_phone TEXT    NOT NULL DEFAULT '';

CREATE UNIQUE INDEX org_settings_public_slug_uidx ON org_settings (public_slug) WHERE public_slug <> '';

ALTER TABLE announcements ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE club_events ADD COLUMN is_public BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE club_events DROP COLUMN IF EXISTS is_public;
ALTER TABLE announcements DROP COLUMN IF EXISTS is_public;
DROP INDEX IF EXISTS org_settings_public_slug_uidx;
ALTER TABLE org_settings
    DROP COLUMN IF EXISTS public_contact_phone,
    DROP COLUMN IF EXISTS public_contact_email,
    DROP COLUMN IF EXISTS public_about,
    DROP COLUMN IF EXISTS public_show_coaches,
    DROP COLUMN IF EXISTS public_site_enabled,
    DROP COLUMN IF EXISTS public_slug;
