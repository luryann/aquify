-- +goose Up
CREATE TABLE splits (
  id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id              UUID NOT NULL,
  result_id           UUID NOT NULL REFERENCES results(id) ON DELETE CASCADE,

  split_number        INT NOT NULL,
  split_distance      INT NOT NULL,
  split_time_ms       INT NOT NULL,
  cumulative_time_ms  INT NOT NULL,

  -- Audit
  created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by          UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by          UUID REFERENCES users(id) ON DELETE SET NULL,

  CONSTRAINT splits_result_split_unique UNIQUE (result_id, split_number)
);

CREATE INDEX splits_result_id_idx ON splits (result_id);
CREATE INDEX splits_org_id_idx    ON splits (org_id);

-- +goose Down
DROP TABLE IF EXISTS splits;
