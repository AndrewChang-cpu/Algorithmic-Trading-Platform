CREATE TABLE strategy_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  strategy_id UUID NOT NULL REFERENCES strategies(id) ON DELETE CASCADE,
  version_number INT NOT NULL,
  s3_key VARCHAR(512) NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(strategy_id, version_number)
);
CREATE INDEX idx_strategy_versions_strategy_id ON strategy_versions(strategy_id);
