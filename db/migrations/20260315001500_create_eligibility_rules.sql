-- +goose Up
CREATE TABLE eligibility_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL,
  group_id UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  rule_type TEXT NOT NULL,
  parameters JSONB NOT NULL DEFAULT '{}',
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_by UUID REFERENCES users(id) ON DELETE SET NULL,
  updated_by UUID REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX eligibility_rules_org_id_idx ON eligibility_rules (org_id);
CREATE INDEX eligibility_rules_group_id_idx ON eligibility_rules (group_id);
CREATE INDEX eligibility_rules_group_active_idx ON eligibility_rules (group_id, is_active);

CREATE TRIGGER eligibility_rules_set_updated_at
BEFORE UPDATE ON eligibility_rules
FOR EACH ROW
EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS eligibility_rules;
