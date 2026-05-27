CREATE TABLE portfolio_metrics (
  time TIMESTAMPTZ NOT NULL,
  job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  open DECIMAL(20,4),
  high DECIMAL(20,4),
  low DECIMAL(20,4),
  close DECIMAL(20,4),
  PRIMARY KEY (job_id, time)
);
SELECT create_hypertable('portfolio_metrics', 'time');
CREATE INDEX idx_portfolio_metrics_job_id ON portfolio_metrics(job_id, time DESC);
