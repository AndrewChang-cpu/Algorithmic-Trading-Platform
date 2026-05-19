# System Design: Algorithmic Trading Platform

## Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│ BROWSER                                                              │
└──────────────────────────┬───────────────────────────────────────────┘
                           │ HTTPS / WebSocket
                           ▼
┌──────────────────────────────────────────────────────────────────────┐
│ go-app  (Go, REST API + WebSocket)                                   │
│  • JWT auth middleware                                               │
│  • /api/auth, /api/strategies, /api/jobs endpoints                  │
│  • WS /api/stream/jobs/:id, /api/stream/portfolio/:userId            │
│  • Consumes portfolio_data from Kafka → broadcasts to WS clients    │
└──────┬───────────────────────────────────────────────┬──────────────┘
       │ S3 uploads / reads                            │ Enqueue job
       │ PostgreSQL queries                            ▼
       │                                    ┌──────────────────────────┐
       │                                    │ Redis (Celery queue)     │
       │                                    └──────────┬───────────────┘
       │                                               │
       │                                               ▼
       │                                    ┌──────────────────────────┐
       │                                    │ Celery Worker  (Python)  │
       │                                    │ run_lean_backtest()      │
       │                                    │ run_lean_live()          │
       │                                    └──┬───────┬───────────────┘
       │                            HTTP req   │       │ docker run
       │                            POST       │       ▼
       │                            /data/     │  ┌───────────────────────┐
       │                            historical │  │ LEAN Docker Container │
       │                            ▼          │  │  (lean-atp image)     │
       │                 ┌──────────────────┐  │  │                       │
       │                 │ go-data  (Go)    │  │  │ Backtest mode:        │
       │                 │ Alpaca WS → Kafka│  │  │  FileSystemDataFeed   │
       │                 │ Alpaca REST →    │  │  │  reads local CSV files│
       │                 │   market_data    │  │  │  (written by Celery)  │
       │                 │ POST /data/hist  │  │  │                       │
       │                 └────────┬─────────┘  │  │ Live mode:            │
       │                          │ writes     │  │  KafkaDataFeed reads  │
       │                          ▼            │  │  from Kafka           │
       │                 ┌──────────────────┐  │  │  PaperBrokerage sims  │
       │                 │ market_data      │◄─┘  │  fills locally        │
       │                 │ (TimescaleDB     │      └─────────┬─────────────┘
       │                 │  hypertable)     │                │ results JSON
       │                 │ Celery reads     │                │ (bind mount)
       │                 │ for CSV          │                │
       │                 │ materialization  │                │
       │                 └──────────────────┘                │
       │                                                      │
       │                 ┌────────────────────┐               │
       │                 │ Kafka              │               │
       │                 │  stock_data        │  live mode    │
       │                 │  (live bars only)  │◄──────────────┘
       │                 │  portfolio_data    │
       │                 └────────┬───────────┘
       │                 Celery polls portfolio snaps
       │                 → publishes portfolio_data
       ▼
┌────────────────────────────────────────────────────────────────────┐
│ PostgreSQL + TimescaleDB                                           │
│  users, strategies, strategy_versions, jobs, job_logs,            │
│  performance_metrics, portfolio_metrics (hypertable),             │
│  market_data (hypertable, owned by go-data)                       │
└────────────────────────────────────────────────────────────────────┘
```

## Data Flows

### Backtest flow
```
1. User submits: {strategyVersionId, symbols, startDate, endDate, resolution}
   → POST /api/jobs (go-app)
   → creates jobs record (status: queued)
   → pushes Celery task to Redis: run_lean_backtest(job_id)

2. Celery picks up task
   → downloads strategy .py from S3

3. Celery calls:
   POST http://go-data:8081/data/historical
     {symbols, start_date, end_date, resolution}
   go-data checks market_data for existing coverage
   go-data fetches only missing date ranges from Alpaca REST API
   go-data writes new bars to market_data (TimescaleDB)
   go-data returns HTTP 200 { bars_ready: N }

4. Celery queries market_data for all bars in requested range
   → materializes to LEAN CSV format:
       /tmp/jobs/<job_id>/data/equity/usa/{resolution}/{symbol}/{date}_trade.zip
   → writes LEAN config.json (backtest mode, FileSystemDataFeed)
   → updates job status: running
   → docker run --rm -v /tmp/jobs/<job_id>:/lean lean-atp:latest
   → waits for container exit (timeout: 2h default)
   → reads /tmp/jobs/<job_id>/Results/<strategy>-summary.json
   → parses: writes performance_metrics + portfolio_metrics rows
   → updates job status: completed (or failed)
   → cleans up /tmp/jobs/<job_id>/

Note: Kafka is NOT used in the backtest data path. market_data is the
      single source of truth for historical bars.
```

### Live paper trading flow
```
1. User submits: {strategyVersionId, symbols, resolution}
   → POST /api/jobs (type=live) (go-app)
   → creates jobs record (status: queued)
   → pushes Celery task: run_lean_live(job_id)

2. Celery picks up task
   → downloads strategy .py from S3
   → POST http://go-data:8081/data/historical
       {symbols, start_date: today-warmup_days, end_date: today, resolution}
   go-data fills gaps in market_data (same as backtest step 3)

3. Celery queries market_data → materializes warmup_days of history to LEAN CSV files
   in /tmp/jobs/<job_id>/data/ (these are warmup files)
   LEAN reads them via SubscriptionDataReaderHistoryProvider before switching
   to real-time KafkaDataFeed

4. Celery writes LEAN live config.json
   → updates job status: running
   → docker run -d --rm -v /tmp/jobs/<job_id>:/lean lean-atp:latest
   → stores container ID

5. LEAN starts:
   → SetWarmUp() fires → reads warmup CSVs via SubscriptionDataReaderHistoryProvider
   → warmup complete → switches to KafkaDataQueueHandler for real-time

6. go-data (already running live Alpaca WebSocket)
   → continuously publishes live bars to Kafka stock_data (no job_id header in live mode)

7. KafkaDataFeed (C# plugin in LEAN container)
   → subscribes to stock_data topic with consumer group lean-live-{job_id}
   → converts JSON bar messages to LEAN TradeBar objects

8. Celery polling loop (every 5s):
   → reads portfolio snapshots from /tmp/jobs/<job_id>/Results/
   → publishes portfolio updates to Kafka portfolio_data topic
   → watches Redis key "job:<job_id>:stop" for stop signal

9. go-app
   → Kafka consumer on portfolio_data topic
   → broadcasts to WebSocket clients subscribed to /api/stream/jobs/:id

10. On stop: Redis key seen → Celery → docker stop <container_id>
    → reads final results, parses, stores → job status: completed
    → cleans up /tmp/jobs/<job_id>/
```

### Stop signal flow
```
1. POST /api/jobs/:id/cancel (go-app)
   → validates job belongs to user
   → sets Redis key "job:<job_id>:stop" = "1"
2. Celery worker polling loop sees the key
3. Celery: docker stop <container_id>
4. Container exits, Celery cleans up, marks job completed/failed
```

## Tech Stack

| Component | Technology | Notes |
|-----------|-----------|-------|
| REST API | Go 1.23 | Gorilla Mux router |
| WebSocket | Go | Gorilla WebSocket |
| Job queue | Redis + Celery | Python 3.11 |
| LEAN execution | Docker (subprocess from Celery) | lean-atp custom image |
| KafkaDataFeed | C# (.NET 6) | Compiled into lean-atp Docker image |
| Frontend | React 18 + TypeScript, Vite | React Query, Zustand, Lightweight Charts |
| Message bus | Kafka (KRaft, no Zookeeper) | Live bars only; not in backtest data path |
| Database | PostgreSQL 14 + TimescaleDB | TimescaleDB for portfolio_metrics, market_data |
| Object storage | AWS S3 | Strategy .py files and CSV uploads |
| Migrations | golang-migrate | SQL files in /migrations |
| Deployment | Kubernetes (kops), ArgoCD | AWS us-east-1 |
| CI/CD | GitHub Actions → ECR → ArgoCD | |
| Secrets | Bitnami Sealed Secrets | |

## Repository Structure

```
.
├── .plan/                      # Planning documents (this directory)
├── documentation/              # Architecture docs (update after implementation)
├── scripts/                    # deploy-cluster.sh, teardown-cluster.sh
├── migrations/                 # golang-migrate SQL files (NEW)
│   ├── 001_create_users.up.sql
│   ├── 002_create_strategies.up.sql
│   ├── 003_create_jobs.up.sql
│   └── ...
├── go-app/                     # REST API + WebSocket (REWRITE)
│   ├── main.go
│   ├── go.mod
│   ├── handlers/
│   │   ├── auth.go             # register, login, refresh, logout
│   │   ├── strategies.go       # CRUD + upload
│   │   ├── jobs.go             # submit, cancel, get
│   │   └── stream.go           # WebSocket handlers
│   ├── middleware/
│   │   └── jwt.go              # JWT validation middleware
│   ├── models/
│   │   └── models.go           # User, Strategy, StrategyVersion, Job structs
│   └── Dockerfile
├── go-data/                    # Alpaca → Kafka + historical data cache (EXTEND)
│   ├── main.go                 # Alpaca WS→Kafka + POST /data/historical + market_data writes
│   ├── go.mod
│   └── Dockerfile
├── python/                     # Celery workers (REWRITE)
│   ├── celery_worker.py        # Task definitions: run_lean_backtest, run_lean_live
│   ├── lean_runner.py          # Docker container lifecycle management
│   ├── results_parser.py       # LEAN JSON → PostgreSQL (90+ metrics)
│   ├── strategy_validator.py   # AST scan + QCAlgorithm class name extraction
│   ├── data_materializer.py    # market_data rows → LEAN CSV zip files
│   └── Dockerfile
├── lean-plugin/                # C# KafkaDataFeed IDataQueueHandler (NEW)
│   ├── KafkaDataQueueHandler.cs
│   ├── KafkaDataQueueHandler.csproj
│   └── Dockerfile              # FROM quantconnect/lean + compile + install plugin
├── web/                        # React frontend (REWRITE)
│   └── src/
│       ├── pages/
│       │   ├── Login.tsx
│       │   ├── Register.tsx
│       │   ├── Overview.tsx    # Dashboard
│       │   ├── Strategies.tsx  # Strategy list
│       │   ├── StrategyDetail.tsx
│       │   ├── Backtests.tsx   # Backtest job list
│       │   ├── Results.tsx     # Backtest results dashboard
│       │   └── Live.tsx        # Live trading monitor
│       ├── components/
│       │   ├── layout/         # Sidebar, Topbar
│       │   ├── results/        # EquityCurve, HeroMetrics, MetricsTabs
│       │   ├── strategy/       # UploadModal, CodeViewer, VersionList
│       │   └── jobs/           # JobCard, StatusBadge, RunBacktestModal
│       ├── hooks/
│       │   ├── useAuth.ts
│       │   ├── useJobStatus.ts # WebSocket job status subscription
│       │   └── usePortfolio.ts # WebSocket portfolio subscription
│       └── lib/
│           ├── api.ts          # Axios instance + interceptors
│           └── store.ts        # Zustand auth store
├── kubernetes/
│   ├── infrastructure/         # Kafka, Redis, PostgreSQL StatefulSets
│   ├── core/
│   │   ├── go-app/
│   │   ├── go-data/
│   │   └── celery-worker/
│   ├── secrets/                # Sealed secrets
│   └── argocd/
├── local-kafka-docker-compose.yml
├── kops.yaml
└── logs/                       # All service logs (mandatory)
```

## Data Model

### users
```sql
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email VARCHAR(255) UNIQUE NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### strategies
```sql
CREATE TABLE strategies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name VARCHAR(255) NOT NULL,
  description TEXT,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_strategies_user_id ON strategies(user_id);
```

### strategy_versions
```sql
CREATE TABLE strategy_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  strategy_id UUID NOT NULL REFERENCES strategies(id) ON DELETE CASCADE,
  version_number INT NOT NULL,
  s3_key VARCHAR(512) NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(strategy_id, version_number)
);
CREATE INDEX idx_strategy_versions_strategy_id ON strategy_versions(strategy_id);
```

### jobs
```sql
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
```

### performance_metrics
50+ columns extracted from LEAN results JSON. Key columns:
```sql
CREATE TABLE performance_metrics (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  job_id UUID NOT NULL UNIQUE REFERENCES jobs(id) ON DELETE CASCADE,
  -- Returns
  total_return_pct DECIMAL(10,4),
  benchmark_return_pct DECIMAL(10,4),
  alpha DECIMAL(10,6),
  beta DECIMAL(10,6),
  -- Risk
  sharpe_ratio DECIMAL(10,4),
  sortino_ratio DECIMAL(10,4),
  max_drawdown_pct DECIMAL(10,4),
  max_drawdown_duration_days INT,
  volatility_annual DECIMAL(10,4),
  -- Trades
  total_trades INT,
  win_rate_pct DECIMAL(10,4),
  avg_win_pct DECIMAL(10,4),
  avg_loss_pct DECIMAL(10,4),
  profit_loss_ratio DECIMAL(10,4),
  total_fees DECIMAL(20,4),
  -- Portfolio
  start_equity DECIMAL(20,4),
  end_equity DECIMAL(20,4),
  -- ... (50+ total columns per DATABASE.md)
  created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### portfolio_metrics (TimescaleDB hypertable)
```sql
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
```

### market_data (TimescaleDB hypertable, owned by go-data)
Historical OHLCV bar cache. go-data is the sole writer. Celery reads for CSV
materialization. Avoids redundant Alpaca API calls across jobs.
```sql
CREATE TABLE market_data (
  time        TIMESTAMPTZ NOT NULL,
  symbol      VARCHAR(20) NOT NULL,
  resolution  VARCHAR(10) NOT NULL,  -- '1m', '1h', '1d'
  open        DECIMAL(20,4),
  high        DECIMAL(20,4),
  low         DECIMAL(20,4),
  close       DECIMAL(20,4),
  volume      BIGINT
);
SELECT create_hypertable('market_data', 'time');
CREATE UNIQUE INDEX ON market_data (symbol, resolution, time DESC);
```

### refresh_tokens
```sql
CREATE TABLE refresh_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash VARCHAR(255) NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
```

## API Contracts

### Authentication
```
POST /api/auth/register
  Body: { email: string, password: string }
  201: { userId: string, accessToken: string, refreshToken: string }
  409: { error: "email already registered" }
  422: { error: "password must be at least 8 characters" }

POST /api/auth/login
  Body: { email: string, password: string }
  200: { accessToken: string, refreshToken: string }
  401: { error: "invalid credentials" }

POST /api/auth/refresh
  Body: { refreshToken: string }
  200: { accessToken: string, refreshToken: string }
  401: { error: "invalid or expired refresh token" }
  Note: old refresh token is invalidated on each call (rotation).
        Client must store and use the newly returned refreshToken.

POST /api/auth/logout
  Auth: Bearer token
  Body: { refreshToken: string }
  204: (no body)
```

### Strategies
```
GET /api/strategies
  Auth: Bearer token
  200: [{ id, name, latestVersion, runCount, bestSharpe, createdAt }]

POST /api/strategies
  Auth: Bearer token
  Body: multipart/form-data { name: string, file: .py file }
  201: { strategyId, versionId, versionNumber: 1 }
  422: { error: "AST violation: import os detected" }
  422: { error: "no QCAlgorithm subclass found" }

GET /api/strategies/:id
  Auth: Bearer token
  200: { id, name, versions: [{id, versionNumber, createdAt}], stats: {runCount, bestSharpe, avgReturn} }
  403: (job belongs to different user)
  404: (not found)

POST /api/strategies/:id/versions
  Auth: Bearer token
  Body: multipart/form-data { file: .py file }
  201: { versionId, versionNumber }
  422: { error: "..." }

GET /api/strategies/:id/versions/:versionId/code
  Auth: Bearer token
  200: { code: string (raw Python source) }

DELETE /api/strategies/:id
  Auth: Bearer token
  204: (no body)
  404: (not found)
```

### Jobs
```
GET /api/jobs
  Auth: Bearer token
  Query: ?type=backtest|live&status=queued|running|completed|failed&page=1&limit=20
  200: { jobs: [{id, strategyName, versionNumber, type, status, createdAt, metrics?: {returnPct, sharpe}}], total: int }

POST /api/jobs
  Auth: Bearer token
  Body (backtest, alpaca): { strategyVersionId: string, type: "backtest", dataSource: "alpaca",
                             symbols: string[], startDate: "YYYY-MM-DD", endDate: "YYYY-MM-DD",
                             resolution: "1d"|"1h"|"1m" }
  Body (backtest, csv):     { strategyVersionId: string, type: "backtest", dataSource: "csv",
                             symbols: string[], startDate: "YYYY-MM-DD", endDate: "YYYY-MM-DD",
                             resolution: "1d"|"1h"|"1m", csvFile: file }
  Body (live):              { strategyVersionId: string, type: "live",
                             symbols: string[], resolution: "1d"|"1h"|"1m",
                             warmupDays: int }  // default 365 if omitted
  202: { jobId: string }
  400: { error: "..." }

GET /api/jobs/:id
  Auth: Bearer token
  200: { id, strategyName, versionNumber, type, status, createdAt, startedAt, completedAt, errorMessage }
  403: (belongs to different user)
  404: (not found)

GET /api/jobs/:id/metrics
  Auth: Bearer token
  200: { ...all 90+ performance_metrics columns... }
  404: (no metrics yet — job not completed)

GET /api/jobs/:id/portfolio
  Auth: Bearer token
  Query: ?from=ISO8601&to=ISO8601
  200: { points: [{time, open, high, low, close}] }

POST /api/jobs/:id/cancel
  Auth: Bearer token
  202: { jobId, status: "cancelling" }
  400: { error: "job is not running" }
```

### WebSocket
```
WS /api/stream/jobs/:id
  Auth: Bearer token (query param ?token=...)
  Messages (server → client):
    { type: "status", status: "running"|"completed"|"failed" }
    { type: "log", level: "INFO"|"ERROR", message: string, timestamp: ISO8601 }

WS /api/stream/portfolio/:jobId
  Auth: Bearer token (query param ?token=...)
  Messages (server → client):
    { type: "snapshot", time: ISO8601, equity: number, unrealized: number, holdings: number, fees: number }
```

### go-data Internal HTTP
```
POST /data/historical
  Body: { symbols: string[], start_date: "YYYY-MM-DD", end_date: "YYYY-MM-DD", resolution: "1d"|"1h"|"1m" }
  200: { bars_ready: int }
  500: { error: "..." }
  Note: Called only by Celery worker. Not exposed externally.
        go-data checks market_data for existing coverage first;
        only missing date ranges are fetched from Alpaca REST.
        job_id is no longer passed — market_data is a shared cache.
```

## LEAN Docker Image (`lean-atp`)

Built from `quantconnect/lean:latest` with:
1. Compiled `KafkaDataQueueHandler.cs` plugin (C#, .NET 6) installed to LEAN's plugin directory
2. `Confluent.Kafka` NuGet package included
3. LEAN auxiliary data files copied from LEAN's sample data:
   - `{data-folder}/symbol-properties/symbol-properties-database.csv`
   - `{data-folder}/equity/usa/map_files/{symbol}.csv` (ticker rename history)
   - `{data-folder}/factor_files/{symbol}.csv` (split/dividend adjustments)

### LEAN config.json — Backtest mode

Written by Celery to `/tmp/jobs/<job_id>/config.json` before `docker run`.
`algorithm-type-name` is the class name extracted from the strategy .py file
by `strategy_validator.py` (the class inheriting from `QCAlgorithm`).

```json
{
  "environment": "backtesting",
  "algorithm-type-name": "<ClassName>",
  "algorithm-language": "Python",
  "algorithm-location": "/lean/algorithm/main.py",
  "data-folder": "/lean/data",
  "results-destination-folder": "/lean/results",
  "environments": {
    "backtesting": {
      "live-mode": false,
      "setup-handler": "QuantConnect.Lean.Engine.Setup.BacktestingSetupHandler",
      "result-handler": "QuantConnect.Lean.Engine.Results.BacktestingResultHandler",
      "data-feed-handler": "QuantConnect.Lean.Engine.DataFeeds.FileSystemDataFeed",
      "real-time-handler": "QuantConnect.Lean.Engine.RealTime.BacktestingRealTimeHandler",
      "history-provider": "QuantConnect.Lean.Engine.HistoricalData.SubscriptionDataReaderHistoryProvider",
      "transaction-handler": "QuantConnect.Lean.Engine.TransactionHandlers.BacktestingTransactionHandler"
    }
  }
}
```

### LEAN config.json — Live paper trading mode

```json
{
  "environment": "live-paper",
  "algorithm-type-name": "<ClassName>",
  "algorithm-language": "Python",
  "algorithm-location": "/lean/algorithm/main.py",
  "data-folder": "/lean/data",
  "results-destination-folder": "/lean/results",
  "job-id": "<job_id>",
  "kafka-bootstrap-servers": "kafka:9092",
  "environments": {
    "live-paper": {
      "live-mode": true,
      "live-mode-brokerage": "PaperBrokerage",
      "setup-handler": "QuantConnect.Lean.Engine.Setup.BrokerageSetupHandler",
      "result-handler": "QuantConnect.Lean.Engine.Results.LiveTradingResultHandler",
      "data-feed-handler": "QuantConnect.Lean.Engine.DataFeeds.LiveTradingDataFeed",
      "data-queue-handler": ["KafkaDataQueueHandler"],
      "real-time-handler": "QuantConnect.Lean.Engine.RealTime.LiveTradingRealTimeHandler",
      "transaction-handler": "QuantConnect.Lean.Engine.TransactionHandlers.BacktestingTransactionHandler",
      "history-provider": ["QuantConnect.Lean.Engine.HistoricalData.SubscriptionDataReaderHistoryProvider"]
    }
  }
}
```

`KafkaDataQueueHandler` responsibilities:
- On `Subscribe(symbols)`: create Kafka consumer for `stock_data` topic
  using consumer group ID `lean-live-{job_id}` (unique per job — prevents
  concurrent live jobs from consuming each other's partitions)
- Convert JSON bar messages to LEAN `TradeBar` objects
- On `Unsubscribe`: close consumer

## LEAN CSV Data Format

Celery's `data_materializer.py` writes market_data bars to the directory
structure that LEAN's `FileSystemDataFeed` expects.

### Directory layout
```
/tmp/jobs/<job_id>/data/equity/usa/{resolution}/{symbol_lowercase}/{date}_trade.zip
```
- `resolution`: `minute`, `hour`, or `daily`
- `symbol_lowercase`: ticker in lowercase (e.g., `spy`)
- `date`: `YYYYMMDD`

### CSV format inside each zip
```
Milliseconds,Open,High,Low,Close,Volume
```
- `Milliseconds`: milliseconds since midnight for intraday data; `0` for daily bars
- Prices are scaled ×10000 and stored as integers (e.g., $150.25 → `1502500`)
- Each zip file contains exactly one CSV file named `{date}_trade.csv`

### Required auxiliary files (shipped in lean-atp image)
```
{data-folder}/symbol-properties/symbol-properties-database.csv
{data-folder}/equity/usa/map_files/{symbol}.csv
{data-folder}/factor_files/{symbol}.csv
```
These must be copied from LEAN's sample data into the lean-atp Dockerfile.
Without them LEAN will error on startup before processing any bars.

## LEAN Results JSON — Parser Guide

Celery's `results_parser.py` reads the results JSON written by LEAN to
`/tmp/jobs/<job_id>/Results/`. Key fields and how to use them:

```
results JSON top-level structure:
  state.Status                         "Completed" | "RuntimeError"
  state.RuntimeError                   error message string (if Status == "RuntimeError")
  state.StartTime / state.EndTime      ISO8601 strings

  totalPerformance.portfolioStatistics  { startEquity, endEquity, sharpeRatio,
                                          drawdown, alpha, beta, ... }
                                        Primary source for performance_metrics.
                                        Fields are numeric (not string-formatted).

  totalPerformance.tradeStatistics      { totalNumberOfTrades, winRate,
                                          totalFees, averageWin, averageLoss, ... }
                                        Source for trade-related columns.

  charts["Strategy Equity"]
        .series["Equity"]
        .values                         [[unix_timestamp_seconds, open, high, low, close], ...]
                                        Write each element to portfolio_metrics hypertable.

  statistics                            { "Sharpe Ratio": "8.854", "Total Orders": "1", ... }
                                        Human-readable strings with % and $ signs.
                                        Prefer totalPerformance fields over this dict.

  runtimeStatistics                     { "Equity": "$101,691.92", "Fees": "-$3.44", ... }
                                        Display-only; do not parse for storage.
```

Error detection: if `state.Status == "RuntimeError"`, mark job as failed and
store `state.RuntimeError` in `jobs.error_message`.

## Auth & Authorization

- JWT access token: RS256 signed, 15-minute expiry, contains `{userId, email}`
- RS256 keys stored as Bitnami Sealed Secret; mounted into go-app pod at
  `/run/secrets/jwt_private.pem` and `/run/secrets/jwt_public.pem`
- Refresh token: random 32-byte token, hashed (SHA-256) before storage in
  `refresh_tokens` table, 7-day expiry
- Refresh token rotation: on each `POST /api/auth/refresh`, the old token row
  is deleted and a new token is inserted. Client must store the new refreshToken
  from the response.
- WebSocket connections: token is validated once at connection time. The connection remains open regardless of token expiry — live trading sessions can last hours. If the client disconnects, it must reconnect with a fresh (possibly refreshed) token.
- All database queries include `WHERE user_id = :authenticated_user_id`
- S3 paths scoped to `{userId}/{strategyId}/...` — Go app enforces this, not S3 bucket policy

## Brownfield Context

**Patterns to preserve:**
- `go-data/main.go` Kafka producer pattern: keep the Alpaca WebSocket → Kafka flow, add HTTP endpoint and DB write alongside it
- `local-kafka-docker-compose.yml`: keep exactly as-is for local dev
- `kubernetes/` manifest structure: keep namespace/directory layout

**Code that must not change:**
- Kafka topic names: `stock_data`, `portfolio_data`
- KRaft-only Kafka (no Zookeeper)
- Log destination: all logs to `/logs`

**Code being replaced entirely:**
- `go-app/main.go` → full rewrite
- `python/celery_worker.py` → full rewrite (remove Backtrader)
- `python/strategy.py` → delete (Backtrader strategy, no longer needed)
- `python/kafka_materializer.py` → replaced by `data_materializer.py` (reads market_data, not Kafka)
- `web/src/` → full rewrite

## Deployment

### Kubernetes Namespaces
| Namespace | Services |
|-----------|---------|
| atp-core | go-app, celery-worker |
| atp-data | go-data |
| atp-db | PostgreSQL + TimescaleDB (Patroni HA) |
| atp-infra | Kafka (KRaft, 3 nodes), Redis |
| argocd | ArgoCD |

### go-data Network Policy
go-data's `POST /data/historical` endpoint is internal-only. A Kubernetes
NetworkPolicy restricts ingress to go-data's HTTP port to pods in the
`atp-core` namespace only. No application-level auth is added to this endpoint.

### Node Sizing
| Role | Instance | Count |
|------|---------|-------|
| Control plane | t3.small | 1 |
| Workers (API, Celery) | t2.micro | 3 |
| Kafka nodes | t3.small | 3 |
| DB nodes | t3.medium | 2 |

### LEAN Containers
LEAN containers are **not** managed by Kubernetes. They are spawned via `docker run` from within the Celery worker pod, which requires the Docker socket to be mounted (`/var/run/docker.sock`). Each container is ephemeral (`--rm`), mounts a job-specific tmpdir, and runs on the same node as the Celery worker.

### CI/CD
1. Push to `main` → GitHub Actions triggers
2. Build Docker images for go-app, go-data, celery-worker, lean-atp (if plugin changed)
3. Push to ECR
4. Update `kubernetes/*/kustomization.yaml` with new image tags
5. Commit → ArgoCD detects change → auto-syncs to cluster

### Local Development

**Prerequisites:** Docker Desktop, Go 1.23+, Python 3.11+, Node.js 18+, .NET 6 SDK

**Generate JWT keys (one-time):**
```bash
openssl genrsa -out go-app/jwt_private.pem 2048
openssl rsa -in go-app/jwt_private.pem -pubout -out go-app/jwt_public.pem
```

**Infrastructure:**
```bash
# Replace local-kafka-docker-compose.yml with KRaft Kafka + Redis + PostgreSQL + MinIO
docker compose -f local-docker-compose.yml up -d
# Provides: Kafka (KRaft) :9092, Redis :6379, PostgreSQL+TimescaleDB :5432, MinIO :9000/:9001

# Run migrations
cd migrations && migrate -database "postgres://postgres:password@localhost:5432/atp?sslmode=disable" up

# Create MinIO bucket (run once after MinIO starts)
mc alias set local http://localhost:9000 minioadmin minioadmin
mc mb local/atp-strategies
```

**Build lean-atp Docker image (one-time, re-run if KafkaDataFeed changes):**
```bash
docker build -t lean-atp:latest lean-plugin/
```

**Services (separate terminals):**
```bash
cd go-app && go run .
cd go-data && go run main.go
cd python && celery -A celery_worker worker --loglevel=info
cd web && npm run dev
```

**Note:** local-kafka-docker-compose.yml uses Zookeeper and must be replaced with a KRaft-mode
compose file (`local-docker-compose.yml`) that also includes PostgreSQL+TimescaleDB and MinIO.
The new file is the single local dev bootstrap command.

## Environment Variables

All env vars are loaded from `.env` files locally (gitignored). In Kubernetes, they are injected via Sealed Secrets or ConfigMaps.

### go-data (`go-data/.env`)
```
ALPACA_API_KEY=...
ALPACA_API_SECRET=...
KAFKA_BOOTSTRAP_SERVERS=localhost:9092       # kafka:9092 in k8s
DATABASE_URL=postgres://postgres:password@localhost:5432/atp?sslmode=disable
HTTP_PORT=8081
```

### go-app (`go-app/.env`)
```
DATABASE_URL=postgres://postgres:password@localhost:5432/atp?sslmode=disable
REDIS_URL=redis://localhost:6379/0
KAFKA_BOOTSTRAP_SERVERS=localhost:9092       # kafka:9092 in k8s
JWT_PRIVATE_KEY_PATH=./jwt_private.pem
JWT_PUBLIC_KEY_PATH=./jwt_public.pem
S3_ENDPOINT=http://localhost:9000           # omit in prod (uses default AWS endpoint)
S3_ACCESS_KEY=minioadmin                    # AWS access key in prod
S3_SECRET_KEY=minioadmin                    # AWS secret key in prod
S3_BUCKET=atp-strategies
S3_REGION=us-east-1
CORS_ORIGINS=http://localhost:5173
PORT=8080
```

### python Celery worker (`python/.env`)
```
DATABASE_URL=postgres://postgres:password@localhost:5432/atp?sslmode=disable
REDIS_URL=redis://localhost:6379/0
S3_ENDPOINT=http://localhost:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_BUCKET=atp-strategies
S3_REGION=us-east-1
LEAN_IMAGE=lean-atp:latest
LEAN_JOB_TMP_DIR=/tmp/atp-jobs
GO_DATA_URL=http://localhost:8081
# Kafka address as seen from inside LEAN Docker containers (not localhost):
LEAN_KAFKA_BOOTSTRAP_SERVERS=host.docker.internal:9092  # Mac/Windows local dev
# In k8s, set to: kafka:9092
```

**Note on LEAN container Kafka networking (local dev):**
Celery spawns LEAN containers via `docker run`. Inside those containers, `localhost`
refers to the container itself, not the host. On Mac/Windows, use
`host.docker.internal:9092` as the Kafka address in the LEAN live config.json.
On Linux local dev, run LEAN containers with `--network=host` and use `localhost:9092`.
In Kubernetes, use `kafka:9092` (service DNS). The `LEAN_KAFKA_BOOTSTRAP_SERVERS` env
var controls which address Celery writes into the generated LEAN config.json.

## Testing Strategy

### Unit Tests
| Target | Framework | What to test |
|--------|-----------|-------------|
| `python/strategy_validator.py` | pytest | Each blocked import/call, valid strategy passes, missing QCAlgorithm class, class name extraction |
| `python/results_parser.py` | pytest | Sample LEAN JSON → correct PostgreSQL rows, RuntimeError detection, missing fields handled |
| `go-app/middleware/jwt.go` | go test | Valid token, expired token, tampered token, missing header |
| `go-app/handlers/` | go test | Each endpoint: happy path, auth failure, 404, user isolation |
| `python/data_materializer.py` | pytest | market_data rows → correct LEAN CSV zip format, price scaling, millisecond conversion |

### Integration Tests
| Scenario | What to verify |
|----------|---------------|
| Full backtest pipeline | Submit job → mock go-data → market_data populated → LEAN runs → metrics in PostgreSQL |
| Full live pipeline | Start live job → warmup CSVs written → WebSocket receives portfolio snapshots → stop signal terminates container |
| Auth flow | Register → login → use token → refresh (new token issued, old invalidated) → expired token → redirect |
| User isolation | User A cannot GET/DELETE User B's strategies or jobs |
| CSV upload | Upload CSV → bars published correctly → materialized to LEAN CSV format |

### E2E Tests
| Journey | Tool | Steps |
|---------|------|-------|
| Register → upload → backtest → results | Playwright | Full browser flow against staging environment |
| Go live → updates → stop | Playwright | WebSocket updates received, stop works |
| Token refresh | Playwright | Simulate 401 mid-session, verify silent refresh + new token stored |
