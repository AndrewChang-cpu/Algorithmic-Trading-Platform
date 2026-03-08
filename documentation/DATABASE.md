# Database Schema

## PostgreSQL Tables

### users
```sql
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
```

### strategies
```sql
CREATE TABLE strategies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name VARCHAR(255) NOT NULL,
  s3_key VARCHAR(512) NOT NULL,  -- S3 object key
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_strategies_user_id ON strategies(user_id);
```

### jobs
```sql
CREATE TABLE jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  strategy_id UUID NOT NULL REFERENCES strategies(id),
  type VARCHAR(20) CHECK (type IN ('backtest', 'live')),
  status VARCHAR(20) CHECK (status IN ('queued', 'running', 'completed', 'failed')),
  config JSONB,  -- {start_date, end_date, initial_cash, symbols, commission}
  error_message TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  started_at TIMESTAMPTZ,
  completed_at TIMESTAMPTZ
);

CREATE INDEX idx_jobs_user_id ON jobs(user_id);
CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_created_at ON jobs(created_at DESC);
```

### job_logs
```sql
CREATE TABLE job_logs (
  id SERIAL PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  timestamp TIMESTAMPTZ DEFAULT NOW(),
  level VARCHAR(20),
  message TEXT
);

CREATE INDEX idx_job_logs_job_id ON job_logs(job_id);
CREATE INDEX idx_job_logs_timestamp ON job_logs(timestamp);
```

---

## TimescaleDB Hypertables

### portfolio_metrics (Equity Curve)
```sql
CREATE TABLE portfolio_metrics (
  time TIMESTAMPTZ NOT NULL,
  job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  open DECIMAL(20, 2),
  high DECIMAL(20, 2),
  low DECIMAL(20, 2),
  close DECIMAL(20, 2),  -- Portfolio value
  PRIMARY KEY (job_id, time)
);

-- Convert to hypertable (TimescaleDB)
SELECT create_hypertable('portfolio_metrics', 'time');

-- Create index for job queries
CREATE INDEX idx_portfolio_metrics_job_id ON portfolio_metrics(job_id, time DESC);
```

**Data Source**: LEAN `charts.Strategy Equity.series.Equity.values`

**Example**:
```json
"values": [
  [1381118400, 100000.0, 100000.0, 100000.0, 100000.0],  // [timestamp, O, H, L, C]
  [1381377600, 99990.0, 100543.0, 98283.0, 98878.2174]
]
```

---

## Performance Metrics Table

### performance_metrics
```sql
CREATE TABLE performance_metrics (
  job_id UUID PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,

  -- Runtime Statistics
  equity DECIMAL(20, 2),
  fees DECIMAL(20, 2),
  holdings DECIMAL(20, 2),
  net_profit DECIMAL(20, 2),
  unrealized DECIMAL(20, 2),
  volume DECIMAL(20, 2),

  -- Portfolio Statistics (Key Metrics)
  compounding_annual_return DECIMAL(10, 4),
  drawdown DECIMAL(10, 4),
  drawdown_recovery INTEGER,
  total_net_profit DECIMAL(10, 4),
  sharpe_ratio DECIMAL(10, 4),
  probabilistic_sharpe_ratio DECIMAL(10, 4),
  sortino_ratio DECIMAL(10, 4),

  -- Risk Metrics
  alpha DECIMAL(10, 4),
  beta DECIMAL(10, 4),
  annual_standard_deviation DECIMAL(10, 4),
  annual_variance DECIMAL(10, 4),
  information_ratio DECIMAL(10, 4),
  tracking_error DECIMAL(10, 4),
  treynor_ratio DECIMAL(10, 4),
  value_at_risk_99 DECIMAL(10, 4),
  value_at_risk_95 DECIMAL(10, 4),

  -- Portfolio Metrics
  portfolio_turnover DECIMAL(10, 4),
  expectancy DECIMAL(10, 4),
  start_equity DECIMAL(20, 2),
  end_equity DECIMAL(20, 2),
  estimated_strategy_capacity DECIMAL(20, 2),
  lowest_capacity_asset VARCHAR(255),

  -- Trade Statistics
  total_number_of_trades INTEGER,
  number_of_winning_trades INTEGER,
  number_of_losing_trades INTEGER,
  total_profit_loss DECIMAL(20, 2),
  total_profit DECIMAL(20, 2),
  total_loss DECIMAL(20, 2),
  largest_profit DECIMAL(20, 2),
  largest_loss DECIMAL(20, 2),
  average_profit_loss DECIMAL(20, 2),
  average_profit DECIMAL(20, 2),
  average_loss DECIMAL(20, 2),

  -- Trade Duration
  average_trade_duration INTERVAL,
  average_winning_trade_duration INTERVAL,
  average_losing_trade_duration INTERVAL,
  median_trade_duration INTERVAL,
  maximum_drawdown_duration INTERVAL,

  -- Trade Ratios
  max_consecutive_winning_trades INTEGER,
  max_consecutive_losing_trades INTEGER,
  profit_loss_ratio DECIMAL(10, 4),
  win_loss_ratio DECIMAL(10, 4),
  win_rate DECIMAL(10, 4),
  loss_rate DECIMAL(10, 4),

  -- Excursion Metrics
  average_mae DECIMAL(20, 2),  -- Maximum Adverse Excursion
  average_mfe DECIMAL(20, 2),  -- Maximum Favorable Excursion
  largest_mae DECIMAL(20, 2),
  largest_mfe DECIMAL(20, 2),

  -- Drawdown Metrics
  maximum_closed_trade_drawdown DECIMAL(20, 2),
  maximum_intra_trade_drawdown DECIMAL(20, 2),

  -- Volatility
  profit_loss_standard_deviation DECIMAL(20, 2),
  profit_loss_downside_deviation DECIMAL(20, 2),
  profit_factor DECIMAL(10, 4),

  -- Fees
  total_fees DECIMAL(20, 2),

  -- Raw JSON (for extensibility)
  raw_statistics JSONB,
  raw_trade_statistics JSONB,

  calculated_at TIMESTAMPTZ DEFAULT NOW()
);
```

**Data Source**: LEAN `statistics`, `totalPerformance.portfolioStatistics`, `totalPerformance.tradeStatistics`

---

## Closed Trades Table

### closed_trades
```sql
CREATE TABLE closed_trades (
  id SERIAL PRIMARY KEY,
  job_id UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  symbol VARCHAR(50),
  entry_time TIMESTAMPTZ,
  exit_time TIMESTAMPTZ,
  entry_price DECIMAL(20, 8),
  exit_price DECIMAL(20, 8),
  quantity DECIMAL(20, 8),
  profit_loss DECIMAL(20, 2),
  mae DECIMAL(20, 2),
  mfe DECIMAL(20, 2),
  duration INTERVAL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_closed_trades_job_id ON closed_trades(job_id);
CREATE INDEX idx_closed_trades_symbol ON closed_trades(symbol);
```

**Data Source**: LEAN `totalPerformance.closedTrades[]`

---

## Migration Files

Store in `migrations/` directory:

```
migrations/
├── 001_create_users.sql
├── 002_create_strategies.sql
├── 003_create_jobs.sql
├── 004_create_job_logs.sql
├── 005_create_portfolio_metrics.sql
├── 006_create_performance_metrics.sql
└── 007_create_closed_trades.sql
```

Use **golang-migrate** for versioned migrations:

```bash
migrate -path migrations/ -database "postgres://user:pass@localhost:5432/atp?sslmode=disable" up
```

---

## Sample Queries

### Get Job Results with All Metrics
```sql
SELECT
  j.id,
  j.status,
  j.created_at,
  j.completed_at,
  pm.sharpe_ratio,
  pm.drawdown,
  pm.total_net_profit,
  pm.win_rate,
  pm.total_fees
FROM jobs j
LEFT JOIN performance_metrics pm ON j.id = pm.job_id
WHERE j.user_id = $1
ORDER BY j.created_at DESC
LIMIT 20;
```

### Get Equity Curve for Chart
```sql
SELECT
  time,
  open,
  high,
  low,
  close
FROM portfolio_metrics
WHERE job_id = $1
ORDER BY time ASC;
```

### Get Trade Log
```sql
SELECT
  symbol,
  entry_time,
  exit_time,
  entry_price,
  exit_price,
  quantity,
  profit_loss,
  duration
FROM closed_trades
WHERE job_id = $1
ORDER BY entry_time ASC;
```

### User Dashboard (Last 7 Days Activity)
```sql
SELECT
  COUNT(*) as total_jobs,
  SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed_jobs,
  SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) as failed_jobs,
  AVG(EXTRACT(EPOCH FROM (completed_at - started_at))) as avg_duration_seconds
FROM jobs
WHERE user_id = $1
  AND created_at > NOW() - INTERVAL '7 days';
```

---

## Backup Strategy

### Daily Backups
```bash
# PostgreSQL dump
pg_dump -h postgres-service -U atp_user -d atp > backup_$(date +%Y%m%d).sql

# Upload to S3
aws s3 cp backup_$(date +%Y%m%d).sql s3://atp-backups/postgres/

# Retention: 30 days
```

### Point-in-Time Recovery
Enable WAL archiving in PostgreSQL config:

```
wal_level = replica
archive_mode = on
archive_command = 'aws s3 cp %p s3://atp-backups/wal/%f'
```

**Recovery Point Objective (RPO)**: 5 minutes
**Recovery Time Objective (RTO)**: 15 minutes
