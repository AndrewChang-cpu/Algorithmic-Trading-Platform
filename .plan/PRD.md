# PRD: Algorithmic Trading Platform

## Functional Requirements

### Authentication
| ID | Requirement |
|----|-------------|
| FR-01 | User can register with email + password. Duplicate email returns 409. |
| FR-02 | User can log in; receives a 15-minute JWT access token and a 7-day refresh token. |
| FR-03 | Access token can be refreshed silently using the refresh token before expiry. On each `POST /api/auth/refresh`, the old refresh token is invalidated and a new refresh token is returned alongside the new access token (rotation). The client must store and use the new refresh token for subsequent refreshes. |
| FR-04 | All API endpoints except `/api/auth/*` require a valid JWT. Expired token returns 401. |
| FR-05 | Password is hashed with bcrypt (min cost 12). Minimum password length: 8 characters. |

### Strategy Management
| ID | Requirement |
|----|-------------|
| FR-06 | User can upload a Python `.py` file with a strategy name. Upload is rejected if the file contains imports or calls to: `os`, `subprocess`, `socket`, `sys`, `shutil`, `pathlib`, `eval`, `exec`, `__import__`, `compile`. Rejection returns a 422 with the specific violation. |
| FR-07 | Upload must verify the file defines at least one class inheriting from `QCAlgorithm`. Files failing this check are rejected with 422. The validator also extracts and returns the class name for use in LEAN config generation. |
| FR-08 | Each upload creates a new `strategy_version`. Versions are numbered sequentially (v1, v2, ...). |
| FR-09 | User can view a strategy's detail page: read-only code preview (syntax-highlighted Python), list of all versions, list of all past runs across versions, and aggregate stats (best Sharpe, average return, total runs). |
| FR-10 | User can delete a strategy. This cascade-deletes all versions, all jobs, all metrics, and removes S3 files. Requires explicit confirmation in the UI. |

### Backtest Jobs
| ID | Requirement |
|----|-------------|
| FR-11 | User can run a backtest on a specific strategy version (default: latest). The Run Backtest modal requires: symbols (comma-separated tickers), start date, end date, and resolution (daily/hourly/minute). User is responsible for matching these to the strategy's `AddEquity`/`SetStartDate`/`SetEndDate` calls. |
| FR-12 | User selects data source at job submission: **Alpaca** (default) or **CSV upload** (OHLCV format: `timestamp,open,high,low,close,volume`). |
| FR-13 | On submission with Alpaca data: Celery calls `POST /data/historical` on go-data. go-data checks the `market_data` TimescaleDB cache for existing coverage, fetches only missing date ranges from Alpaca REST, writes new bars to `market_data`, and returns `{ bars_ready: N }`. Celery then queries `market_data` directly to materialize LEAN CSV files. Kafka is not used in the backtest data path. |
| FR-14 | On submission with CSV: Celery parses the uploaded file, validates format (timestamp, open, high, low, close, volume), and materializes bars directly to LEAN CSV files. |
| FR-15 | LEAN executes in Docker (backtest mode, `FileSystemDataFeed`). Results JSON written to a shared bind-mounted volume. |
| FR-16 | After LEAN exits, Celery parses results: stores 90+ metrics in `performance_metrics` and equity curve in `portfolio_metrics` (TimescaleDB). Metrics are sourced from `totalPerformance.portfolioStatistics` and `totalPerformance.tradeStatistics` (structured numeric fields). Equity curve data from `charts["Strategy Equity"].series["Equity"].values`. If `state.Status == "RuntimeError"`, job is marked failed with `state.RuntimeError` as the error message. |
| FR-17 | Backtests auto-terminate after a configurable max duration (default: 2 hours). Job is marked `failed` with reason `timeout`. |
| FR-18 | User can view the results dashboard for a completed backtest: equity curve chart (zoomable/interactive), hero metrics (Net P&L, Sharpe, Max Drawdown, Win Rate), and tabbed detail panels (Risk Metrics, Trade Stats, Portfolio Details). |

### Historical Data Caching
| ID | Requirement |
|----|-------------|
| FR-26 | go-data serves an internal HTTP endpoint `POST /data/historical` accepting `{symbols, start_date, end_date, resolution}`. It checks `market_data` for existing coverage, fetches only missing ranges from Alpaca REST, writes new bars to `market_data`, and returns `{ bars_ready: N }`. |
| FR-27 | Historical bars for a given symbol/resolution/date range that already exist in `market_data` are not re-fetched from Alpaca. This cache is shared across all jobs and users. |

### Live Paper Trading Jobs
| ID | Requirement |
|----|-------------|
| FR-19 | User can start a live paper trading job from the strategy detail page. The Go Live modal requires: symbols (comma-separated tickers), resolution (daily/hourly/minute), and warmup period in days (default 365). No data source selector — live data always flows through go-data → Kafka; go-data's underlying data provider is a deployment concern, not a per-job choice. |
| FR-20 | LEAN executes in Docker (live mode) using the custom `KafkaDataQueueHandler` C# `IDataQueueHandler`. Before starting, Celery calls `POST /data/historical` on go-data with `{symbols, start_date: today-warmupDays, end_date: today, resolution}`, then materializes the returned bars as warmup CSV files. LEAN reads these warmup files via `SubscriptionDataReaderHistoryProvider` on startup, then switches to `KafkaDataQueueHandler` for real-time bars. go-data continuously publishes live bars to Kafka `stock_data` topic. |
| FR-21 | LEAN simulates order fills locally (PaperBrokerage). No external broker connections. |
| FR-22 | LEAN writes interim portfolio snapshots to a shared volume periodically. Celery polls these and publishes `portfolio_data` events to Kafka. go-app's WebSocket handler streams these to the browser. |
| FR-23 | User can stop a live job from the UI. Backend sends SIGTERM to the LEAN Docker container; job is marked `completed`. |
| FR-24 | If a live job's Docker container exits unexpectedly (non-zero exit), it is marked `failed`. |
| FR-25 | On Celery worker restart, any job in `running` state is marked `failed`. User must restart manually. |

## Critical User Journeys

### CUJ-1: Register and First Backtest

```
1. User navigates to /login → clicks "Create account"
2. Fills email + password (min 8 chars) → submits
3. Lands on /overview (Dashboard)
4. Clicks "+ Upload Strategy" → drag-drop .py file → gives it a name → uploads
   - AST scan runs; if violation found → error shown inline
   - On success → strategy created (v1), user navigates to strategy detail
5. On strategy detail page → clicks "Run Backtest"
6. Modal: choose data source (Alpaca default) → confirm
7. Job card appears in "Runs" list with status: Queued → Running
8. [Backend] Celery calls POST /data/historical on go-data
          → go-data checks market_data cache, fetches missing bars from Alpaca
          → Celery reads market_data, materializes LEAN CSV files
          → starts LEAN Docker container (FileSystemDataFeed)
9. Job status updates to Running (WebSocket push)
10. LEAN exits → Celery parses results → stores in PostgreSQL
11. Job status updates to Completed
12. User clicks job → navigates to /results/:jobId
13. Sees equity curve, hero metrics, tabbed panels
```

### CUJ-2: Live Paper Trading

```
1. User navigates to strategy detail page for a live-mode strategy
2. Clicks "Go Live"
3. [Backend] Celery pre-fetches 1 year of history → materializes warmup CSV files
4. Job created, LEAN container starts in live mode
   - Reads warmup CSVs (SubscriptionDataReaderHistoryProvider)
   - Switches to KafkaDataQueueHandler for real-time bars
5. User navigates to /live → sees the strategy's live job card
6. Clicks the job → sees real-time P&L, equity updates, and position table
7. Updates arrive every N seconds via WebSocket
8. User clicks "Stop" → container receives SIGTERM → job marked Completed
9. User can navigate to the completed job's results page
```

### CUJ-3: CSV Data Source Backtest

```
1. User on strategy detail page → clicks "Run Backtest"
2. In the modal, selects "CSV" as data source → uploads OHLCV CSV file
3. Celery parses CSV, validates format (timestamp, open, high, low, close, volume)
   - Invalid format → job fails immediately with descriptive error
4. Bars materialized to LEAN CSV files → LEAN runs
5. Same results flow as CUJ-1 from step 10
```

### CUJ-4: Token Refresh

```
1. User's 15-min access token expires mid-session
2. Next API call returns 401
3. React app's Axios interceptor silently calls POST /api/auth/refresh with refresh token
4. New access token AND new refresh token returned
5. Client stores both tokens → original request retried → succeeds
6. If refresh token is also expired → user redirected to /login
```

## Non-Functional Requirements

| ID | Category | Requirement |
|----|----------|-------------|
| NFR-perf | Performance | API p99 response < 200ms (excluding job submission and file uploads). Equity curve chart renders <100k data points without blocking UI. |
| NFR-sec | Security | AST scan before strategy storage. bcrypt cost ≥ 12. JWT (RS256) in Authorization header (Bearer); private/public keys stored as Sealed Secret. No secrets in Git. Strategy files executed in isolated Docker containers with no host network access. go-data `POST /data/historical` restricted to atp-core namespace via Kubernetes NetworkPolicy (no application-level auth needed). |
| NFR-rely | Reliability | Celery tasks are idempotent (safe to retry on crash). Failed LEAN containers surface error logs to the user. Job queue backed by Redis with persistence. |
| NFR-logging | Logging | All services write structured JSON logs to `/logs`. Log lines include: timestamp, service, level, job_id (if applicable), message. No logs outside `/logs`. |
| NFR-data | Data isolation | Every database query and S3 operation is scoped to the authenticated user's ID. Users cannot access other users' data through any API endpoint. market_data table is a shared infrastructure cache and is not user-scoped. |
| NFR-test | Testing | Unit tests: AST scanner (including class name extraction), results parser, JWT middleware, all API handlers (mocked DB), data_materializer CSV format. Integration tests: full backtest pipeline (market_data cache + LEAN + PostgreSQL), auth flow including token rotation. E2E tests: register→upload→backtest→results; go live→see updates→stop. |
