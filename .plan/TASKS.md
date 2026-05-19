# Tasks: Algorithmic Trading Platform
> Generated: 2026-05-19
> Source: .plan/
> Total: 27 tasks | Starting points: 9

## Dependency Graph

```
T-01 · local-docker-compose.yml

T-02 · DB migrations
└── T-10 · go-data extension

T-03 · KafkaDataQueueHandler.cs
└── T-04 · lean-plugin Dockerfile

T-05 · strategy_validator.py
└── T-09* · celery_worker.py

T-06 · data_materializer.py
T-07 · results_parser.py
T-08 · lean_runner.py

T-11 · go-app models
└── T-12 · JWT middleware
    ├── T-13 · auth handlers
    ├── T-14 · strategy handlers
    ├── T-15 · jobs handlers
    │   └── T-16 · stream.go
    └── T-17* · go-app main.go

T-18 · Web foundation
├── T-19 · Login + Register pages
│   └── T-21 · App.tsx + layout
│       ├── T-22 · Strategies page + UploadModal
│       │   └── T-23 · Strategy detail
│       ├── T-24 · Job modals + components
│       │   ├── T-25* · Backtests list
│       │   └── T-27* · Live + Overview
│       └── T-26** · Results page
└── T-20 · Real-time WebSocket hooks
```

```
* T-09  also depends on T-06, T-07, T-08
* T-17  also depends on T-13, T-14, T-15, T-16
* T-25  also depends on T-24
* T-26  also depends on T-25, T-20
* T-27  also depends on T-24, T-20
```

---

## Tasks

### T-01 · Local docker-compose (KRaft + TimescaleDB + MinIO)
**Status:** `pending`
**Depends on:** none
**Files:** `[new] local-docker-compose.yml`
**What:** Replace `local-kafka-docker-compose.yml` (which uses Zookeeper) with a new `local-docker-compose.yml` that runs four services:
- **Kafka** (KRaft mode, no Zookeeper): `confluentinc/cp-kafka:7.6.1`, single-node with `CLUSTER_ID` set, port 9092, auto-creates topics `stock_data` and `portfolio_data`
- **Redis**: `redis:7-alpine`, port 6379
- **PostgreSQL + TimescaleDB**: `timescale/timescaledb:latest-pg14`, port 5432, `POSTGRES_DB=atp`, `POSTGRES_PASSWORD=password`
- **MinIO**: `minio/minio:latest`, ports 9000 (API) and 9001 (console), `MINIO_ROOT_USER=minioadmin`, `MINIO_ROOT_PASSWORD=minioadmin`, command: `server /data --console-address :9001`

All services on a shared `atp-local` bridge network.
**Done when:** `docker compose -f local-docker-compose.yml up -d` exits 0; `docker compose -f local-docker-compose.yml ps` shows all 4 containers with status `running`; `docker exec <kafka-container> kafka-topics --bootstrap-server localhost:9092 --list` shows `stock_data` and `portfolio_data`

---

### T-02 · Database migrations
**Status:** `pending`
**Depends on:** none
**Files:** `[new] migrations/001_create_users.up.sql`, `[new] migrations/001_create_users.down.sql`, `[new] migrations/002_create_strategies.up.sql`, `[new] migrations/002_create_strategies.down.sql`, `[new] migrations/003_create_strategy_versions.up.sql`, `[new] migrations/003_create_strategy_versions.down.sql`, `[new] migrations/004_create_jobs.up.sql`, `[new] migrations/004_create_jobs.down.sql`, `[new] migrations/005_create_job_logs.up.sql`, `[new] migrations/005_create_job_logs.down.sql`, `[new] migrations/006_create_performance_metrics.up.sql`, `[new] migrations/006_create_performance_metrics.down.sql`, `[new] migrations/007_create_portfolio_metrics.up.sql`, `[new] migrations/007_create_portfolio_metrics.down.sql`, `[new] migrations/008_create_market_data.up.sql`, `[new] migrations/008_create_market_data.down.sql`, `[new] migrations/009_create_refresh_tokens.up.sql`, `[new] migrations/009_create_refresh_tokens.down.sql`
**What:** Write SQL migration files for all 9 tables exactly as specified in SYSTEM-DESIGN.md. Key points:
- `users`: UUID PK, email UNIQUE, password_hash, timestamps
- `strategies`: UUID PK, user_id FK, name, description, timestamps + index on user_id
- `strategy_versions`: UUID PK, strategy_id FK, version_number INT, s3_key, UNIQUE(strategy_id, version_number)
- `jobs`: UUID PK, user_id FK, strategy_version_id FK, type/status/data_source CHECK constraints, `symbols TEXT[]`, `resolution VARCHAR(10)`, `start_date DATE`, `end_date DATE`, `warmup_days INT`, `csv_s3_key`, `error_message`, `timeout_seconds DEFAULT 7200`, timestamps
- `job_logs`: SERIAL PK, job_id FK, timestamp, level, message
- `performance_metrics`: UUID PK, job_id FK UNIQUE, 50+ DECIMAL columns for returns/risk/trade/portfolio stats
- `portfolio_metrics`: TimescaleDB hypertable on `time`, job_id FK, OHLC columns, `SELECT create_hypertable('portfolio_metrics', 'time')`
- `market_data`: TimescaleDB hypertable on `time`, symbol, resolution, OHLCV, `UNIQUE INDEX ON market_data(symbol, resolution, time DESC)`
- `refresh_tokens`: UUID PK, user_id FK, token_hash VARCHAR(255), expires_at, created_at

Each `.down.sql` drops the table (CASCADE where needed).
**Done when:** With T-01 running, `migrate -database "postgres://postgres:password@localhost:5432/atp?sslmode=disable" -path migrations up` exits 0; `psql -U postgres -d atp -c "\dt"` lists all 9 tables; `psql -U postgres -d atp -c "SELECT hypertable_name FROM timescaledb_information.hypertables"` shows `portfolio_metrics` and `market_data`

---

### T-03 · KafkaDataQueueHandler C# plugin
**Status:** `pending`
**Depends on:** none
**Files:** `[new] lean-plugin/KafkaDataQueueHandler.cs`, `[new] lean-plugin/KafkaDataQueueHandler.csproj`
**What:** Implement a C# .NET 6 class `KafkaDataQueueHandler` that implements LEAN's `IDataQueueHandler` interface. Read `kafka-bootstrap-servers` and `job-id` from LEAN's config dict.

Responsibilities:
- `Subscribe(IEnumerable<Symbol> symbols)`: create a `Confluent.Kafka.IConsumer<string,string>` with `group.id = lean-live-{job_id}`, subscribe to `stock_data` topic, start background thread calling `consumer.Consume()`
- Background loop: deserialize each Kafka message (JSON `{"S": "SPY", "o": 1.0, "h": 1.1, "l": 0.9, "c": 1.0, "v": 100, "t": "2024-01-02T09:30:00Z"}`) into a LEAN `TradeBar` object; enqueue into a `ConcurrentQueue<BaseData>`
- `GetNextTicks()`: dequeue and return all available bars from the queue as `IEnumerable<BaseData>`
- `Unsubscribe(IEnumerable<Symbol> symbols)`: close consumer
- `IsConnected`: return true if consumer is non-null and subscription active

`.csproj` targets `net6.0`, references `QuantConnect.Lean.Engine` and `Confluent.Kafka` NuGet packages.
**Done when:** `dotnet build lean-plugin/` exits 0 with no errors; `KafkaDataQueueHandler.cs` exports a public class implementing `IDataQueueHandler`

---

### T-04 · lean-atp Docker image
**Status:** `pending`
**Depends on:** T-03
**Files:** `[new] lean-plugin/Dockerfile`
**What:** Multi-stage Dockerfile that builds `lean-atp:latest`:

Stage 1 (build): `FROM mcr.microsoft.com/dotnet/sdk:6.0 AS build`, copy `lean-plugin/` files, `dotnet publish -c Release -o /app/publish`

Stage 2 (final): `FROM quantconnect/lean:latest`, copy compiled plugin from stage 1 into LEAN's Launcher directory (typically `/Lean/Launcher/bin/Debug/` or check `quantconnect/lean` image for exact path), copy required auxiliary data files from the same base image:
- `/Lean/Data/symbol-properties/symbol-properties-database.csv` → `$DATA_FOLDER/symbol-properties/`
- `/Lean/Data/equity/usa/map_files/` → `$DATA_FOLDER/equity/usa/map_files/`
- `/Lean/Data/equity/usa/factor_files/` → `$DATA_FOLDER/equity/usa/factor_files/`

Set `ENTRYPOINT ["dotnet", "QuantConnect.Lean.Launcher.dll"]`
**Done when:** `docker build -t lean-atp:latest lean-plugin/` exits 0; `docker run --rm lean-atp:latest echo ok` exits 0; `docker run --rm lean-atp:latest ls /Lean/Data/symbol-properties/symbol-properties-database.csv` exits 0

---

### T-05 · strategy_validator.py
**Status:** `pending`
**Depends on:** none
**Files:** `[new] python/strategy_validator.py`, `[new] python/test_strategy_validator.py`
**What:** Implement `validate_strategy(source_code: str) -> dict` in `python/strategy_validator.py`:
- Scan for blocked patterns using Python's `ast` module: `os`, `subprocess`, `socket`, `sys`, `shutil`, `pathlib`, `eval`, `exec`, `__import__`, `compile`. Walk the AST looking for `Import`, `ImportFrom`, `Call` nodes matching these names.
- Scan for `class X(QCAlgorithm)` using AST: walk `ClassDef` nodes, check bases for `id == "QCAlgorithm"`.
- Return `{"valid": False, "violation": "import os detected on line 3"}` on first blocked pattern found.
- Return `{"valid": False, "violation": "no QCAlgorithm subclass found"}` if no qualifying class found.
- Return `{"valid": True, "class_name": "MyStrategy"}` on success (class_name = the first `QCAlgorithm` subclass name).

Write `python/test_strategy_validator.py` with pytest tests covering:
- Valid strategy with `QCAlgorithm` subclass → `valid=True`, correct `class_name`
- Strategy with `import os` → `valid=False`, violation mentions `os`
- Strategy with `subprocess.run(...)` call → `valid=False`
- Strategy with no `QCAlgorithm` subclass → `valid=False`, violation mentions `QCAlgorithm`
- Strategy with multiple classes, only one `QCAlgorithm` → returns that class name
**Done when:** `cd python && pytest test_strategy_validator.py -v` passes all tests

---

### T-06 · data_materializer.py
**Status:** `pending`
**Depends on:** none
**Files:** `[new] python/data_materializer.py`, `[new] python/test_data_materializer.py`
**What:** Implement `materialize_lean_csv(rows: list[dict], output_dir: str, symbol: str, resolution: str)` in `python/data_materializer.py`.

Input `rows`: list of dicts with keys `time` (datetime), `open`, `high`, `low`, `close`, `volume` (all from market_data query).

For each unique date in rows:
1. Create directory: `{output_dir}/equity/usa/{resolution_lean}/{symbol_lower}/` where `resolution_lean` maps `1d→daily`, `1h→hour`, `1m→minute`
2. Build CSV string with format `Milliseconds,Open,High,Low,Close,Volume`:
   - `Milliseconds`: for daily bars, use `0`; for intraday, milliseconds since midnight
   - Prices multiplied by 10000 and cast to int (e.g., $150.25 → `1502500`)
3. Write to a zip file at `{date_YYYYMMDD}_trade.zip` containing one file `{date_YYYYMMDD}_trade.csv`

Write `python/test_data_materializer.py` with pytest tests:
- Daily bar: price $150.25 → CSV value 1502500, Milliseconds=0
- Minute bar at 09:30:00 → Milliseconds=34200000
- Output zip file contains correctly named CSV
- Directory structure matches expected LEAN path
**Done when:** `cd python && pytest test_data_materializer.py -v` passes all tests

---

### T-07 · results_parser.py
**Status:** `pending`
**Depends on:** none
**Files:** `[new] python/results_parser.py`, `[new] python/test_results_parser.py`
**What:** Implement two functions in `python/results_parser.py`:

`parse_performance_metrics(results: dict) -> dict`: reads `results["totalPerformance"]["portfolioStatistics"]` and `results["totalPerformance"]["tradeStatistics"]`, returns a flat dict mapping to `performance_metrics` column names (e.g., `sharpe_ratio`, `total_return_pct`, `max_drawdown_pct`, `total_trades`, `win_rate_pct`, `total_fees`, `start_equity`, `end_equity`, etc.)

`parse_equity_curve(results: dict) -> list[dict]`: reads `results["charts"]["Strategy Equity"]["series"]["Equity"]["values"]`, returns `[{"time": datetime, "open": float, "high": float, "low": float, "close": float}]` for each `[unix_ts, o, h, l, c]` element.

`is_runtime_error(results: dict) -> tuple[bool, str]`: returns `(True, state.RuntimeError message)` if `results["state"]["Status"] == "RuntimeError"`, else `(False, "")`.

Write `python/test_results_parser.py` using the actual sample JSON structure from `research/lean/lean-cli-test/My Project/backtests/2026-02-13_22-59-35/1535553589-summary.json` as a fixture. Tests: correct Sharpe mapping, equity curve length matches input, RuntimeError detection.
**Done when:** `cd python && pytest test_results_parser.py -v` passes all tests

---

### T-08 · lean_runner.py
**Status:** `pending`
**Depends on:** none
**Files:** `[new] python/lean_runner.py`
**What:** Implement Docker container lifecycle functions in `python/lean_runner.py`:

`run_lean_backtest(job_id: str, job_dir: str, timeout_seconds: int) -> str`:
- Calls `docker run --rm -v {job_dir}:/lean {LEAN_IMAGE}`, waits for exit
- Enforces `timeout_seconds` using `subprocess.run(..., timeout=timeout_seconds)`
- Returns path to results JSON file: `{job_dir}/Results/*.json`
- Raises `TimeoutError` if container exceeds timeout
- Raises `RuntimeError` if exit code non-zero

`run_lean_live(job_id: str, job_dir: str) -> str`:
- Calls `docker run -d --rm -v {job_dir}:/lean --add-host=host.docker.internal:host-gateway {LEAN_IMAGE}`
- Returns container ID string

`stop_lean_live(container_id: str)`:
- Calls `docker stop {container_id}` with 30-second timeout
- Reads final results JSON path and returns it

`poll_live_results(job_dir: str) -> dict | None`:
- Reads `{job_dir}/Results/*.json` if it exists, returns parsed dict; else returns None

All Docker calls use `subprocess.run` or `subprocess.Popen`. `LEAN_IMAGE` read from `os.environ["LEAN_IMAGE"]`. All log output written to `/logs/lean_runner.log` via Python's `logging` module.
**Done when:** `cd python && python -c "from lean_runner import run_lean_backtest, run_lean_live, stop_lean_live, poll_live_results; print('ok')"` prints `ok`; function signatures match the spec above

---

### T-09 · celery_worker.py + python/Dockerfile
**Status:** `pending`
**Depends on:** T-05, T-06, T-07, T-08
**Files:** `python/celery_worker.py`, `python/Dockerfile`, `python/requirements.txt`
**What:** Rewrite `python/celery_worker.py`. Delete `python/strategy.py` and `python/consumer_test.py`.

`celery_worker.py` defines two Celery tasks using `app = Celery('atp', broker=os.environ['REDIS_URL'])` with Redis result backend:

`run_lean_backtest(job_id: str)`:
1. Fetch job record from PostgreSQL (psycopg2): get symbols, start_date, end_date, resolution, strategy_version_id
2. Download strategy .py from S3/MinIO using boto3 to `/tmp/atp-jobs/{job_id}/algorithm/main.py`
3. `validate_strategy(source)` — double-check; mark job failed if invalid
4. `POST {GO_DATA_URL}/data/historical` with `{symbols, start_date, end_date, resolution}`, assert 200
5. Query market_data from PostgreSQL for the requested range/symbols
6. `materialize_lean_csv(rows, output_dir, symbol, resolution)` for each symbol
7. Write LEAN `config.json` (backtest mode, algorithm-type-name from validate result)
8. Update job status to `running` in DB
9. `run_lean_backtest(job_id, job_dir, timeout_seconds)` from lean_runner
10. `parse_performance_metrics(results_json)` → INSERT into performance_metrics
11. `parse_equity_curve(results_json)` → batch INSERT into portfolio_metrics
12. Update job status to `completed`; on any exception, set status `failed` + error_message

`run_lean_live(job_id: str)`:
1–3. Same as backtest (fetch, download, validate)
4. `POST /data/historical` with `{symbols, start_date: today-warmup_days, end_date: today, resolution}`
5–6. Materialize warmup CSVs
7. Write LEAN `config.json` (live-paper mode, `kafka-bootstrap-servers: {LEAN_KAFKA_BOOTSTRAP_SERVERS}`, `job-id: {job_id}`)
8. Update job status to `running`
9. `run_lean_live(job_id, job_dir)` → container_id stored in Redis key `job:{job_id}:container`
10. Polling loop (5s interval): `poll_live_results()` → publish snapshot to Kafka `portfolio_data` topic; check Redis `job:{job_id}:stop` → call `stop_lean_live()` → parse + store final results → status `completed`

Update `python/requirements.txt` (remove Backtrader, add): `celery[redis]`, `psycopg2-binary`, `boto3`, `confluent-kafka`, `pytest`

Update `python/Dockerfile`: `FROM python:3.11-slim`, install Docker CLI (needed for subprocess calls), `pip install -r requirements.txt`, `CMD ["celery", "-A", "celery_worker", "worker", "--loglevel=info", "--logfile=/logs/celery.log"]`

All log output to `/logs/celery.log` and `/logs/lean_runner.log`.
**Done when:** `cd python && pip install -r requirements.txt` exits 0; `celery -A celery_worker inspect registered 2>/dev/null` (with Redis running) lists `atp.run_lean_backtest` and `atp.run_lean_live`; `python -c "import celery_worker"` imports without error

---

### T-10 · go-data: HTTP historical endpoint + TimescaleDB writes
**Status:** `pending`
**Depends on:** T-02
**Files:** `go-data/main.go`, `go-data/go.mod`
**What:** Extend `go-data/main.go`. Keep the existing Alpaca WebSocket → Kafka producer loop. Add:

1. **TimescaleDB client**: add `github.com/jackc/pgx/v5` to `go.mod`. On startup, open a `pgxpool.Pool` using `DATABASE_URL` env var.

2. **HTTP server** (port from `HTTP_PORT` env, default 8081): register `POST /data/historical` handler.

3. **`POST /data/historical` handler**:
   - Parse body: `{symbols: []string, start_date: "YYYY-MM-DD", end_date: "YYYY-MM-DD", resolution: "1d"|"1h"|"1m"}`
   - For each symbol: `SELECT MAX(time), MIN(time), COUNT(*) FROM market_data WHERE symbol=$1 AND resolution=$2 AND time BETWEEN $3 AND $4`
   - Determine missing date ranges (dates where no row exists in market_data)
   - For missing ranges: call Alpaca REST API `GET https://data.alpaca.markets/v2/stocks/{symbol}/bars?start={}&end={}&timeframe={}` using `ALPACA_API_KEY`/`ALPACA_API_SECRET` headers
   - Bulk INSERT new bars into market_data: `INSERT INTO market_data (time, symbol, resolution, open, high, low, close, volume) VALUES ... ON CONFLICT DO NOTHING`
   - Respond `200 {"bars_ready": N}` where N = total bar count in range after upsert

4. Kafka topic creation (if not exists) on startup for KRaft mode using Sarama admin client.

5. Keep existing WebSocket → Kafka producer unchanged (still uses `KAFKA_BOOTSTRAP_SERVERS` env). Load `.env` via `godotenv.Load()` in local dev.

**Done when:** With T-01 running and migrations applied: `curl -s -X POST http://localhost:8081/data/historical -H "Content-Type: application/json" -d '{"symbols":["SPY"],"start_date":"2024-01-02","end_date":"2024-01-05","resolution":"1d"}' | jq .bars_ready` returns an integer ≥ 0; subsequent identical call returns same integer without making Alpaca API requests (verified by checking market_data row count in DB stays the same)

---

### T-11 · go-app/models/models.go
**Status:** `pending`
**Depends on:** none
**Files:** `[new] go-app/models/models.go`, `go-app/go.mod`
**What:** Write `go-app/models/models.go` with Go structs for all DB entities and API request/response types. Add required dependencies to `go.mod`.

Structs:
- `User{ID, Email, PasswordHash, CreatedAt, UpdatedAt}`
- `Strategy{ID, UserID, Name, Description, CreatedAt, UpdatedAt}`
- `StrategyVersion{ID, StrategyID, VersionNumber, S3Key, CreatedAt}`
- `Job{ID, UserID, StrategyVersionID, Type, Status, DataSource, Symbols []string, Resolution, StartDate, EndDate, WarmupDays, CsvS3Key, ErrorMessage, TimeoutSeconds, CreatedAt, StartedAt, CompletedAt}`
- `JobLog{ID, JobID, Timestamp, Level, Message}`
- `RefreshToken{ID, UserID, TokenHash, ExpiresAt, CreatedAt}`
- Request types: `RegisterRequest`, `LoginRequest`, `RefreshRequest`, `SubmitJobRequest`, `UploadStrategyRequest`
- Response types: `AuthResponse{AccessToken, RefreshToken}`, `JobResponse`, `StrategyResponse`

Add to `go.mod`: `github.com/gorilla/mux`, `github.com/golang-jwt/jwt/v5`, `golang.org/x/crypto` (bcrypt), `github.com/jackc/pgx/v5`, `github.com/aws/aws-sdk-go-v2` (S3), `github.com/go-redis/redis/v9`, `github.com/joho/godotenv`, `github.com/gorilla/websocket`, `github.com/confluentinc/confluent-kafka-go/v2`
**Done when:** `go build ./go-app/...` exits 0; `go-app/models/models.go` exports all structs listed above

---

### T-12 · go-app/middleware/jwt.go
**Status:** `pending`
**Depends on:** T-11
**Files:** `[new] go-app/middleware/jwt.go`, `[new] go-app/middleware/jwt_test.go`
**What:** Implement RS256 JWT middleware in `go-app/middleware/jwt.go`:

`LoadKeys(privatePath, publicPath string) error`: reads PEM files, parses `rsa.PrivateKey` and `rsa.PublicKey`, stores in package-level vars.

`GenerateAccessToken(userID, email string) (string, error)`: creates JWT with claims `{sub: userID, email, exp: now+15min}`, signs with RS256 private key.

`GenerateRefreshToken() (string, error)`: generates 32 random bytes via `crypto/rand`, returns hex string.

`HashToken(token string) string`: SHA-256 hash of token, returns hex string.

`RequireAuth(next http.Handler) http.Handler`: HTTP middleware that reads `Authorization: Bearer <token>` header, validates JWT with public key, rejects with 401 if missing/invalid/expired, sets `userID` and `email` in request context via `context.WithValue`.

`GetUserID(ctx context.Context) string`: extracts userID from context (panics if not set — only call after RequireAuth).

Write `go-app/middleware/jwt_test.go`:
- Valid token → middleware calls next handler
- Expired token → 401
- Token signed with wrong key → 401
- Missing Authorization header → 401
**Done when:** `go test ./go-app/middleware/...` passes all 4 tests

---

### T-13 · go-app/handlers/auth.go
**Status:** `pending`
**Depends on:** T-11, T-12
**Files:** `[new] go-app/handlers/auth.go`, `[new] go-app/handlers/auth_test.go`, `[new] go-app/db/db.go`
**What:** Create `go-app/db/db.go` with `InitDB(databaseURL string) (*pgxpool.Pool, error)` and a package-level pool. All handlers use this pool.

Implement handlers in `go-app/handlers/auth.go`:

`Register(w, r)`: parse `{email, password}`, validate password ≥ 8 chars (422 if not), check email uniqueness (409 if exists), bcrypt hash with cost 12, INSERT user, generate access + refresh tokens, hash refresh token, INSERT refresh_tokens row, respond `201 {accessToken, refreshToken, userId}`.

`Login(w, r)`: parse `{email, password}`, fetch user by email (401 if not found), `bcrypt.CompareHashAndPassword` (401 if mismatch), generate tokens, INSERT refresh_tokens row, respond `200 {accessToken, refreshToken}`.

`Refresh(w, r)`: parse `{refreshToken}`, hash it, SELECT row from refresh_tokens WHERE token_hash = hash AND expires_at > NOW() (401 if not found/expired), DELETE that row, generate new access + refresh tokens, INSERT new refresh_tokens row, respond `200 {accessToken, refreshToken}`.

`Logout(w, r)`: parse `{refreshToken}` from body, hash it, DELETE from refresh_tokens, respond `204`.

Write `go-app/handlers/auth_test.go` using `httptest` and a test DB or mock:
- Register 201, duplicate email 409, short password 422
- Login 200, wrong password 401
- Refresh 200 (new tokens returned, old row deleted), invalid token 401
- Logout 204
**Done when:** `go test ./go-app/handlers/ -run TestAuth` passes all tests

---

### T-14 · go-app/handlers/strategies.go
**Status:** `pending`
**Depends on:** T-11, T-12
**Files:** `[new] go-app/handlers/strategies.go`, `[new] go-app/handlers/strategies_test.go`, `[new] go-app/s3/s3.go`
**What:** Create `go-app/s3/s3.go` with `InitS3(endpoint, accessKey, secretKey, bucket, region string)` using `aws-sdk-go-v2`. Supports MinIO via custom endpoint resolver.

Implement handlers in `go-app/handlers/strategies.go`. All routes protected by `RequireAuth` middleware; all DB queries scoped to authenticated `userID`.

`ListStrategies(w, r)`: SELECT strategies for user with latest version number + run count + best Sharpe from joined tables. Respond `200 [{id, name, latestVersion, runCount, bestSharpe, createdAt}]`.

`UploadStrategy(w, r)`: parse `multipart/form-data` (`name`, `file`). Read file bytes. Run inline AST scan (reuse logic equivalent to `strategy_validator.py` — implement in Go using `go/parser` or simple string scanning for blocked imports + regex for `QCAlgorithm` subclass). Return `422 {error: "AST violation: ..."}` or `422 {error: "no QCAlgorithm subclass found"}` on failure. On success: PUT file to S3 at `{userID}/{strategyID}/v1/main.py`, INSERT strategies row, INSERT strategy_versions row (version_number=1). Respond `201 {strategyId, versionId, versionNumber: 1}`.

`GetStrategy(w, r)`: SELECT strategy + all versions + aggregate stats. 403 if wrong user. 404 if not found.

`UploadNewVersion(w, r)`: same scan + S3 put, INSERT strategy_versions with incremented version_number. `201 {versionId, versionNumber}`.

`GetVersionCode(w, r)`: GET object from S3, stream content. `200 {code: string}`.

`DeleteStrategy(w, r)`: verify ownership, DELETE strategy (cascades), delete S3 objects with prefix `{userID}/{strategyID}/`. `204`.

Write `go-app/handlers/strategies_test.go`:
- Upload valid strategy → 201
- Upload with `import os` → 422 with violation message
- Upload with no QCAlgorithm class → 422
- Delete → 204; subsequent GET → 404
- Another user's strategy → 403
**Done when:** `go test ./go-app/handlers/ -run TestStrategies` passes all tests

---

### T-15 · go-app/handlers/jobs.go
**Status:** `pending`
**Depends on:** T-11, T-12
**Files:** `[new] go-app/handlers/jobs.go`, `[new] go-app/handlers/jobs_test.go`, `[new] go-app/queue/queue.go`
**What:** Create `go-app/queue/queue.go` with `InitRedis(url string) *redis.Client` and `EnqueueBacktest(jobID string) error` / `EnqueueLive(jobID string) error` that push Celery-format task messages to the `celery` Redis key (JSON with `task`, `id`, `args` fields matching Celery's protocol).

Also add `SetStopSignal(jobID string) error` that sets Redis key `job:{jobID}:stop = "1"` with 1-hour TTL.

Implement handlers in `go-app/handlers/jobs.go`:

`SubmitJob(w, r)`: parse body (see API contract — symbols, dates, resolution, type, dataSource, warmupDays). Verify strategy_version_id belongs to authenticated user. INSERT jobs row (status=queued). Enqueue Celery task. `202 {jobId}`.

`GetJob(w, r)`: SELECT job. 403 if wrong user. `200 {id, strategyName, versionNumber, type, status, ...}`.

`ListJobs(w, r)`: paginated SELECT with optional `?type=` and `?status=` filters. `200 {jobs: [...], total}`.

`GetJobMetrics(w, r)`: SELECT from performance_metrics WHERE job_id. `404` if no metrics yet. `200 {...all columns}`.

`GetPortfolio(w, r)`: SELECT from portfolio_metrics WHERE job_id AND time BETWEEN from AND to. `200 {points: [{time, open, high, low, close}]}`.

`CancelJob(w, r)`: verify ownership + status=running (400 if not running). `SetStopSignal(jobID)`. `202 {jobId, status: "cancelling"}`.

Write `go-app/handlers/jobs_test.go`:
- Submit backtest → 202, job record in DB with status=queued
- Submit with non-owned strategy_version → 403
- Cancel running job → 202; cancel non-running job → 400
- GetMetrics before completion → 404
**Done when:** `go test ./go-app/handlers/ -run TestJobs` passes all tests

---

### T-16 · go-app/handlers/stream.go
**Status:** `pending`
**Depends on:** T-11, T-15
**Files:** `[new] go-app/handlers/stream.go`
**What:** Implement WebSocket handlers in `go-app/handlers/stream.go`:

`JobStatusStream(w, r)`: upgrade to WebSocket. Read JWT from `?token=` query param (validate at connect time only — connection stays open if token later expires). Get `jobId` from URL. Verify job belongs to authenticated user. Poll DB every 2s for new `job_logs` entries and job status changes. Send JSON messages: `{"type": "status", "status": "running"|"completed"|"failed"}` and `{"type": "log", "level": "INFO", "message": "...", "timestamp": "ISO8601"}`. Close when job reaches terminal state.

`PortfolioStream(w, r)`: upgrade to WebSocket. Read JWT from `?token=` query param, validate at connect time. Get `jobId` from URL. Start Kafka consumer on `portfolio_data` topic with unique group ID. Filter messages by `job_id` field. Forward matching messages as `{"type": "snapshot", "time": "ISO8601", "equity": N, "unrealized": N, "holdings": N, "fees": N}`. Close on client disconnect.

Use `gorilla/websocket` upgrader with `CheckOrigin` reading allowed origins from `CORS_ORIGINS` env var.
**Done when:** `go build ./go-app/...` exits 0; WebSocket connection to `ws://localhost:8080/api/stream/jobs/test-id?token=<valid-jwt>` upgrades successfully (101 response); invalid token returns 401 before upgrade

---

### T-17 · go-app/main.go rewrite
**Status:** `pending`
**Depends on:** T-12, T-13, T-14, T-15, T-16
**Files:** `go-app/main.go`, `go-app/Dockerfile`
**What:** Rewrite `go-app/main.go` as the application entry point:
- Load `.env` via `godotenv.Load()` (no-op if missing, for K8s)
- Call `middleware.LoadKeys(JWT_PRIVATE_KEY_PATH, JWT_PUBLIC_KEY_PATH)`
- Call `db.InitDB(DATABASE_URL)`
- Call `s3.InitS3(S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET, S3_REGION)`
- Call `queue.InitRedis(REDIS_URL)`
- Register routes on `gorilla/mux` router:
  - `POST /api/auth/register` → `handlers.Register`
  - `POST /api/auth/login` → `handlers.Login`
  - `POST /api/auth/refresh` → `handlers.Refresh`
  - `POST /api/auth/logout` → `handlers.Logout`
  - Protected (wrapped in `middleware.RequireAuth`):
    - `GET /api/strategies` → `handlers.ListStrategies`
    - `POST /api/strategies` → `handlers.UploadStrategy`
    - `GET /api/strategies/{id}` → `handlers.GetStrategy`
    - `POST /api/strategies/{id}/versions` → `handlers.UploadNewVersion`
    - `GET /api/strategies/{id}/versions/{versionId}/code` → `handlers.GetVersionCode`
    - `DELETE /api/strategies/{id}` → `handlers.DeleteStrategy`
    - `GET /api/jobs` → `handlers.ListJobs`
    - `POST /api/jobs` → `handlers.SubmitJob`
    - `GET /api/jobs/{id}` → `handlers.GetJob`
    - `GET /api/jobs/{id}/metrics` → `handlers.GetJobMetrics`
    - `GET /api/jobs/{id}/portfolio` → `handlers.GetPortfolio`
    - `POST /api/jobs/{id}/cancel` → `handlers.CancelJob`
    - `GET /api/stream/jobs/{id}` → `handlers.JobStatusStream`
    - `GET /api/stream/portfolio/{jobId}` → `handlers.PortfolioStream`
- Add CORS middleware allowing origins from `CORS_ORIGINS` env (comma-separated)
- `log.Fatal(http.ListenAndServe(":"+PORT, router))`

Update `go-app/Dockerfile`: `FROM golang:1.23-alpine AS build`, `go build -o /app/server`, `FROM alpine:latest`, copy binary, `CMD ["/app/server"]`
**Done when:** `go build ./go-app/` exits 0; `go run ./go-app/ &` starts; `curl -s -X POST http://localhost:8080/api/auth/register -H "Content-Type: application/json" -d '{"email":"test@example.com","password":"password123"}' -o /dev/null -w "%{http_code}"` returns `201`

---

### T-18 · Web frontend foundation
**Status:** `pending`
**Depends on:** none
**Files:** `web/package.json`, `[new] web/src/lib/api.ts`, `[new] web/src/lib/store.ts`, `[new] web/src/hooks/useAuth.ts`
**What:** Update `web/package.json` to add dependencies: `react-router-dom@6`, `@tanstack/react-query@5`, `zustand@4`, `lightweight-charts@4`, `axios`. Remove any conflicting defaults from the Vite scaffold.

`web/src/lib/api.ts`: create Axios instance with `baseURL: import.meta.env.VITE_API_URL ?? "http://localhost:8080"`. Add request interceptor that injects `Authorization: Bearer {accessToken}` from Zustand store. Add response interceptor: on 401, call `POST /api/auth/refresh` with stored refreshToken, update store with new tokens, retry original request. If refresh also fails, call `clearAuth()` and redirect to `/login`.

`web/src/lib/store.ts`: Zustand store with `{ user: {id, email} | null, accessToken: string | null, refreshToken: string | null, setAuth(user, accessToken, refreshToken): void, clearAuth(): void }`. Persist to `localStorage` using `zustand/middleware`'s `persist`.

`web/src/hooks/useAuth.ts`: React Query mutations wrapping `apiClient.post('/api/auth/login')` and `apiClient.post('/api/auth/register')`, both calling `setAuth()` on success. Export `useLogin()` and `useRegister()` hooks.
**Done when:** `cd web && npm install` exits 0; `npm run build` exits 0 (TypeScript compiles); `web/src/lib/api.ts` exports `apiClient`; `web/src/lib/store.ts` exports `useAuthStore`; `web/src/hooks/useAuth.ts` exports `useLogin` and `useRegister`

---

### T-19 · Login and Register pages
**Status:** `pending`
**Depends on:** T-18
**Files:** `[new] web/src/pages/Login.tsx`, `[new] web/src/pages/Register.tsx`
**What:** `Login.tsx`: full-page centered card (no sidebar). Email + password inputs. Submit button (full width, primary, shows spinner while loading). Error message inline below form on 401. On success: call `useLogin()`, redirect to `/overview`. Link to `/register` below the form.

`Register.tsx`: same layout as Login. Email + password inputs (no confirm password field). On success: call `useRegister()`, redirect to `/overview`. Link to `/login` below form. Shows 409 "email already registered" and 422 "password too short" inline errors.

Both pages: inputs disabled during submission. Match design system from UI-SPEC.md (dark background `#0d1117`, card on `#161b22`, blue primary button `#388bfd`, 13px system-sans base font).
**Done when:** `cd web && npm run dev` starts; `http://localhost:5173/login` renders a login form with email/password inputs and submit button without console errors; `http://localhost:5173/register` renders register form

---

### T-20 · Real-time WebSocket hooks
**Status:** `pending`
**Depends on:** T-18
**Files:** `[new] web/src/hooks/useJobStatus.ts`, `[new] web/src/hooks/usePortfolio.ts`
**What:** `useJobStatus(jobId: string | null)`: opens WebSocket to `ws://localhost:8080/api/stream/jobs/{jobId}?token={accessToken}`. Parses incoming JSON messages. Returns `{ status: string, logs: LogEntry[] }`. Reconnects on close (unless jobId is null). Tears down on unmount.

`usePortfolio(jobId: string | null)`: opens WebSocket to `ws://localhost:8080/api/stream/portfolio/{jobId}?token={accessToken}`. Collects `snapshot` messages into array. Returns `{ snapshots: SnapshotEntry[], latestEquity: number | null }`. Reconnects on close. Tears down on unmount.

Both hooks: get `accessToken` from Zustand store. Only connect when `jobId` is non-null. Handle WebSocket close codes gracefully (1000 = normal, others = reconnect after 2s).
**Done when:** `cd web && npm run build` exits 0 (TypeScript compiles with no errors); `web/src/hooks/useJobStatus.ts` and `usePortfolio.ts` both export their respective hooks with correct TypeScript return types

---

### T-21 · App.tsx routing + Layout components
**Status:** `pending`
**Depends on:** T-19
**Files:** `web/src/App.tsx`, `[new] web/src/components/layout/Sidebar.tsx`, `[new] web/src/components/layout/Topbar.tsx`
**What:** Rewrite `web/src/App.tsx` with React Router v6 `<BrowserRouter>`. Define a `<PrivateRoute>` wrapper that reads `accessToken` from Zustand store and redirects to `/login` if null. Routes:
- `/login` → `<Login>` (public)
- `/register` → `<Register>` (public)
- `/` → redirect to `/overview`
- All other routes wrapped in `<PrivateRoute>` + `<AppLayout>`

`Sidebar.tsx`: fixed 220px left column, background `#161b22`. Nav items: Overview (`/overview`), Strategies (`/strategies`), Backtests (`/backtests`), Live (`/live`). Active item highlighted in `#388bfd`. Live item shows a pulsing green dot if any live job is running (query React Query cache). User email + logout button at bottom.

`Topbar.tsx`: 46px height, `#0d1117` background, border-bottom `#21262d`. Shows page title (passed as prop). Right side: logged-in user email chip.

`AppLayout`: renders `<Sidebar>` + `<Topbar>` + scrollable main content area. Passes children into main area.
**Done when:** `cd web && npm run dev`; navigating to `http://localhost:5173/` redirects to `/login` when unauthenticated; after login, layout renders with sidebar showing 4 nav items + topbar

---

### T-22 · Strategies page + Upload modal
**Status:** `pending`
**Depends on:** T-21
**Files:** `[new] web/src/pages/Strategies.tsx`, `[new] web/src/components/strategy/UploadModal.tsx`
**What:** `Strategies.tsx`: uses React Query to fetch `GET /api/strategies`. Shows a table of strategies: name, latest version badge (`v3`), last run P&L, validation status, action buttons (Run Backtest, View, Delete). Empty state: centered card "No strategies yet. Upload your first strategy." with upload button. "+ Upload Strategy" button in Topbar action area opens `<UploadModal>`. Delete action: `window.confirm()` → `DELETE /api/strategies/:id` → invalidate query.

`UploadModal.tsx`: multi-step modal overlay (dark scrim). Step 1: drag-drop zone for `.py` files — shows file name + size on select, "Browse" fallback. Step 2: text input for strategy name (required). Submit triggers `POST /api/strategies`. Step 3 (loading): spinner + "Scanning strategy for security violations...". Step 4a (success): green checkmark, "Strategy uploaded (v1)", Close → navigate to `/strategies/:id`. Step 4b (error): red alert with violation text from 422 response, Back button returns to Step 1.
**Done when:** `cd web && npm run dev`; `/strategies` renders empty state; upload modal opens; dragging a `.py` file shows file name; submitting a strategy with `import os` shows the violation error message

---

### T-23 · Strategy detail page
**Status:** `pending`
**Depends on:** T-22
**Files:** `[new] web/src/pages/StrategyDetail.tsx`, `[new] web/src/components/strategy/CodeViewer.tsx`, `[new] web/src/components/strategy/VersionSelector.tsx`
**What:** `StrategyDetail.tsx`: fetches `GET /api/strategies/:id`. Shows aggregate stats bar (total runs, best Sharpe, best return, avg return). Two tabs: Code and Runs. Header: strategy name, version badge, "Upload New Version" button (opens UploadModal in update mode), "Delete Strategy" button (danger, confirms before delete).

`VersionSelector.tsx`: dropdown showing `v3 (latest)`, `v2`, `v1` with upload dates. Changing version re-fetches the code view.

**Code tab**: `<CodeViewer>` renders strategy source code with syntax highlighting for Python. Use `highlight.js` or `prism-react-renderer` with dark theme. Read-only.

**Runs tab**: table of past runs fetched via `GET /api/jobs?strategyVersionId=...`. Columns: Run #, version, type badge (Backtest/Live), status badge, date, Net P&L, Sharpe. Clicking a row navigates to `/results/:jobId` (backtest) or `/live` (live). Empty state: "No runs yet." Primary "Run Backtest" button and green "Go Live" button in header open the respective modals (imported from T-24).
**Done when:** `/strategies/:id` renders; Code tab shows syntax-highlighted Python; Runs tab shows empty state "No runs yet."; version selector dropdown renders with available versions

---

### T-24 · Job modals + job components
**Status:** `pending`
**Depends on:** T-21
**Files:** `[new] web/src/components/jobs/RunBacktestModal.tsx`, `[new] web/src/components/jobs/GoLiveModal.tsx`, `[new] web/src/components/jobs/StatusBadge.tsx`, `[new] web/src/components/jobs/JobCard.tsx`
**What:** `RunBacktestModal.tsx`: Step 1 — fields: Symbols (text input, placeholder "SPY, QQQ"), Start Date (date picker input), End Date (date picker input), Resolution (select: Daily/Hourly/Minute), Data Source radio cards (Alpaca default, CSV Upload). Validate all fields on submit; start date must be before end date. If CSV selected, Step 2 shows dropzone for `.csv` file (max 50MB). Calls `POST /api/jobs` on submit. Step 3: "Job queued. Job ID: abc123. View job status →" link to `/results/:jobId`. Close button on each step.

`GoLiveModal.tsx`: fields: Symbols (text input), Resolution (select), Warmup Period (number input, default 365, label "Warmup period (days)", min=1). Calls `POST /api/jobs` (type=live) on submit. Confirmation step: "Live job queued. Job ID: abc123. View live monitor →" link to `/live`.

`StatusBadge.tsx`: colored chip for `queued` (gray), `running` (yellow pulse), `completed` (green), `failed` (red). Accepts `status: string` prop.

`JobCard.tsx`: card used in Live page left panel. Shows strategy name, current equity (or "--"), runtime (HH:MM:SS), status dot. Accepts `job: JobResponse` prop.
**Done when:** `cd web && npm run build` exits 0; `RunBacktestModal` renders with all 5 form fields visible; submitting with empty fields shows validation errors; `StatusBadge` renders correct colors for each status value

---

### T-25 · Backtests list page
**Status:** `pending`
**Depends on:** T-21, T-24
**Files:** `[new] web/src/pages/Backtests.tsx`
**What:** `Backtests.tsx`: uses React Query to fetch `GET /api/jobs?type=backtest` (paginated, 20 per page). Shows filter bar with status buttons: All / Queued / Running / Completed / Failed. Renders a list of `<JobCard>` variants (table row style): strategy name, version, status badge, data source, date created, net P&L (if completed), Sharpe (if completed). Clicking a row navigates to `/results/:jobId`. Empty state: "No backtests yet. Select a strategy and run your first backtest." Pagination controls at bottom. Loading state: skeleton rows.
**Done when:** `cd web && npm run dev`; `/backtests` renders with status filter bar; empty state shown when no jobs; filter buttons change the query `?status=` param visible in the React Query devtools

---

### T-26 · Results page + chart components
**Status:** `pending`
**Depends on:** T-25, T-20
**Files:** `[new] web/src/pages/Results.tsx`, `[new] web/src/components/results/EquityCurve.tsx`, `[new] web/src/components/results/HeroMetrics.tsx`, `[new] web/src/components/results/MetricsTabs.tsx`, `[new] web/src/components/results/LogStream.tsx`
**What:** `EquityCurve.tsx`: renders Lightweight Charts `CandlestickSeries` using data from `GET /api/jobs/:id/portfolio`. Chart background `#0d1117`, grid lines `#21262d`, up candles `#3fb950`, down candles `#f85149`. Auto-fits content. While job is running, subscribes to `useJobStatus` snapshot updates to append new candles in real time.

`HeroMetrics.tsx`: renders 4 metric cards: Net P&L (green if positive, red if negative), Sharpe Ratio, Max Drawdown (red), Win Rate. Plus "Total Fees" chip below. While running, shows `--` with a pulsing gray indicator.

`MetricsTabs.tsx`: tab panels — Risk Metrics (Alpha, Beta, Sharpe, Sortino, VaR 95/99, Tracking Error, Info Ratio), Trade Stats (Total Orders, Win Rate, Avg Win, Avg Loss, P/L Ratio, Total Fees), Portfolio Details (Start/End Equity, Annualized Return, Volatility, Drawdown, Treynor Ratio).

`LogStream.tsx`: auto-scrolling `<pre>` panel showing log lines from `useJobStatus`. Last 200 lines. Level-colored: INFO=gray, ERROR=red, WARN=yellow.

`Results.tsx`: orchestrates the above. Three states:
- **Running**: partial equity chart + hero metrics with `--` + spinner badge + LogStream
- **Completed**: full chart + metrics + tabs + "Export JSON" and "Export CSV" download buttons
- **Failed**: red banner "Backtest failed: [error_message]" + no chart + "View Logs" toggle for LogStream
**Done when:** `cd web && npm run dev`; `/results/:jobId` for a completed job renders equity chart with candles, 4 hero metric cards, and 3 tabs with metric values; failed job shows red banner; running job shows spinner badge

---

### T-27 · Overview + Live trading monitor pages
**Status:** `pending`
**Depends on:** T-21, T-24, T-20
**Files:** `[new] web/src/pages/Overview.tsx`, `[new] web/src/pages/Live.tsx`
**What:** `Overview.tsx`: system health bar showing Kafka / Redis / PostgreSQL status indicators (green/warn/down) — poll `GET /api/health` every 30s (add this endpoint to go-app: returns 200 with `{kafka: "ok"|"down", redis: "ok"|"down", db: "ok"|"down"}`). Summary cards: Active Jobs (links to `/backtests?status=running`), Total Strategies, Total Backtests. Recent jobs table (last 10): strategy name, type badge, status badge, created date — rows link to results or live.

`Live.tsx`: split panel layout (280px left + flexible right).
- **Left panel**: list of running live jobs as `<JobCard>` components fetched from `GET /api/jobs?type=live&status=running`. "+ Go Live" button at bottom opens `<GoLiveModal>` (opens strategy selector first: `GET /api/strategies` → user picks one, then GoLiveModal opens). Polling every 5s via React Query.
- **Right panel**: shows detail for the selected job. Stats bar: Equity, Unrealized P&L, Holdings, Fees (from `usePortfolio` hook latest snapshot). Real-time equity chart (last 4 hours, scrolling, using `usePortfolio` snapshots). Positions table (placeholder — LEAN doesn't emit per-position data in MVP: show "Position data not available in MVP"). Log stream from `useJobStatus`. "Stop" danger button → `window.confirm()` → `POST /api/jobs/:id/cancel`.
- **Empty state**: centered card "No live strategies running."
**Done when:** `/overview` renders health bar with 3 status indicators + summary cards + recent jobs table; `/live` renders empty state "No live strategies running" with "+ Go Live" button; clicking the button opens a strategy selector then GoLiveModal

---

## Open Questions
- **Strategy validation bridge (Go vs Python):** SYSTEM-DESIGN.md places `strategy_validator.py` in Python, but go-app (Go) needs to scan at upload time synchronously. T-14 implements the scan inline in Go to avoid inter-service calls. `python/strategy_validator.py` (T-05) serves as a pre-execution double-check in Celery and for unit-tested reference behavior. If strict Python AST parity is required (e.g., Python-specific import aliasing), consider exposing a lightweight HTTP endpoint from the Python container — but this adds deployment complexity not warranted for MVP.
- **Health endpoint:** T-27 references `GET /api/health`. This is a small addition to T-17 (go-app main.go). Implementer of T-27 should coordinate with T-17 implementer, or it can be added as a one-liner to T-17.
