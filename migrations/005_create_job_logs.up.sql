CREATE TABLE job_logs (
  id SERIAL PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  timestamp TIMESTAMPTZ DEFAULT NOW(),
  level VARCHAR(20),
  message TEXT
);
CREATE INDEX idx_job_logs_job_id ON job_logs(job_id);
