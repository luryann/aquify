-- +goose Up
CREATE TABLE practice_attendance (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       TEXT        NOT NULL,
    practice_id  UUID        NOT NULL REFERENCES practices(id) ON DELETE CASCADE,
    swimmer_id   UUID        NOT NULL REFERENCES swimmers(id) ON DELETE CASCADE,
    status       TEXT        NOT NULL CHECK (status IN ('present','absent','excused')),
    marked_at    TIMESTAMPTZ,
    marked_by    TEXT        NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT practice_attendance_practice_swimmer_unique UNIQUE (practice_id, swimmer_id)
);

CREATE INDEX practice_attendance_practice_id_idx ON practice_attendance(practice_id);
CREATE INDEX practice_attendance_swimmer_id_idx ON practice_attendance(swimmer_id);

-- +goose Down
DROP TABLE IF EXISTS practice_attendance;
