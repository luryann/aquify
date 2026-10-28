-- +goose Up
-- groups.code is stored uppercase, but swimmer_groups.group_code was written
-- lowercase, so group rosters, practice attendance, and group-scoped views
-- never matched their links. Normalise existing links to the canonical form.
UPDATE swimmer_groups SET group_code = upper(trim(group_code)) WHERE group_code <> upper(trim(group_code));

-- +goose Down
-- Irreversible data normalisation; the previous lowercase form was a bug.
SELECT 1;
