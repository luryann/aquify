-- +goose Up
-- Make swimmer_id NOT NULL and change ON DELETE to RESTRICT so results are never
-- silently orphaned if a swimmer record is deleted.
ALTER TABLE results
    ALTER COLUMN swimmer_id SET NOT NULL,
    DROP CONSTRAINT IF EXISTS results_swimmer_id_fkey,
    ADD CONSTRAINT results_swimmer_id_fkey
        FOREIGN KEY (swimmer_id) REFERENCES swimmers(id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE results
    DROP CONSTRAINT IF EXISTS results_swimmer_id_fkey,
    ADD CONSTRAINT results_swimmer_id_fkey
        FOREIGN KEY (swimmer_id) REFERENCES swimmers(id) ON DELETE SET NULL,
    ALTER COLUMN swimmer_id DROP NOT NULL;
