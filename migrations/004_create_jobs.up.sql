CREATE TABLE jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  strategy_version_id UUID NOT NULL REFERENCES strategy_versions(id),
  type VARCHAR(20) CHECK (type IN ('backtest', 'live')),
  status VARCHAR(20) CHECK (status IN ('queued', 'running', 'completed', 'failed')),
  data_source VARCHAR(20) CHECK (data_source IN ('alpaca', 'csv')),
  symbols TEXT[] NOT NULL,
  resolution VARCHAR(10) NOT NULL,
  start_date DATE,
  end_date DATE,
  warmup_days INT,
  csv_s3_key VARCHAR(512),
  error_message TEXT,
  timeout_seconds INT DEFAULT 7200,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ
);
CREATE INDEX idx_jobs_user_id ON jobs(user_id);
CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_created_at ON jobs(created_at DESC);
