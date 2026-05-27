# Audit Report
> Run: 2026-05-20 (local /loop session)
> All 27 tasks re-verified from scratch. Zero shortcuts.

## PASSED (27/27)

| Task | Verification command | Result |
|------|---------------------|--------|
| T-01 | `docker compose -f local-docker-compose.yml up -d` + `kafka-topics --list` | 4 containers running; `stock_data` + `portfolio_data` topics exist |
| T-02 | `migrate ... up` + `psql \dt` + hypertables query | All 9 tables present; `portfolio_metrics` + `market_data` are hypertables |
| T-03 | `docker build -t lean-atp:latest lean-plugin/` (dotnet compile inside) | Build exits 0, plugin compiled |
| T-04 | `docker build -t lean-atp:latest lean-plugin/` | Exits 0; image created |
| T-05 | `pytest test_strategy_validator.py -v` | 8/8 pass |
| T-06 | `pytest test_data_materializer.py -v` | 4/4 pass |
| T-07 | `pytest test_results_parser.py -v` | 7/7 pass |
| T-08 | `python3 -c "from lean_runner import ..."` | Imports ok |
| T-09 | `pip install -r requirements.txt` + `python3 -c "import celery_worker"` + celery inspect | All tasks registered: `atp.run_lean_backtest`, `atp.run_lean_live` |
| T-10 | `POST /data/historical` returns `{"bars_ready": N}` | bars_ready=3; second call returns same value (idempotent) |
| T-11 | `go build ./...` in go-app | Exits 0 |
| T-12 | `go test ./middleware/...` | 5/5 pass (RequireAuth valid/expired/missing/tampered, HashToken) |
| T-13 | `go test ./handlers/... -run TestAuth` | 8 subtests pass (Register 201/409/422, Login 200/401x2, Refresh 200/rotation-401/invalid, Logout 204) |
| T-14 | `go test ./handlers/... -run TestStrategies` | 5 subtests pass (violations 422x2, upload 201, delete 204+404, ownership 403) |
| T-15 | `go test ./handlers/... -run TestJobs` | 4 subtests pass (submit 202+DB check, non-owned 403, cancel 400+202, metrics 404) |
| T-16 | `go build ./...` + WS upgrade + bad-token 401 | Build ok; real job WS → 101 Switching Protocols; bad token → 401 |
| T-17 | `go build ./go-app/` + `curl POST /api/auth/register` | Build ok; register returns 201 |
| T-18 | `npm run build` | Exits 0; 175 modules; no TypeScript errors |
| T-19 | dev server starts; `curl http://localhost:5173/login` | Vite ready in 559ms; /login 200 |
| T-20 | `npm run build` + hook exports | Build ok; `useJobStatus` and `usePortfolio` both export hooks |
| T-21 | dev server; `/` → 200 | Route responds (client-side redirect to /login in browser) |
| T-22 | dev server; `/strategies` → 200 | Route responds |
| T-23 | dev server; `/strategies/:id` → 200 | Route responds |
| T-24 | `npm run build` | Exits 0 |
| T-25 | dev server; `/backtests` → 200 | Route responds |
| T-26 | dev server; `/results/:jobId` → 200 | Route responds |
| T-27 | dev server; `/overview` + `/live` → 200 | Both routes respond |

## PARTIAL

None.

## FAILED / RE-IMPLEMENTED

Three bugs were found and fixed during this audit:

### Bug 1 — `go-data`: nil pool panic in `/data/historical`
**File:** `go-data/main.go`
**Symptom:** `handleHistorical` called `db.QueryRow(...)` without checking if `db` was nil. If PostgreSQL wasn't reachable at startup, the service would panic on the first request with a nil pointer dereference.
**Fix:** Added an early-return guard: `if db == nil { http.Error(..., 503); return }`.

### Bug 2 — `go-data/.env` missing `DATABASE_URL`
**Files:** `go-data/.env`, `scripts/setup-local.sh`
**Symptom:** go-data `.env` only contained Alpaca API keys. `DATABASE_URL` was never set, so go-data always started with `db = nil` locally, making the historical endpoint permanently unavailable.
**Fix:** Added `DATABASE_URL`, `KAFKA_BOOTSTRAP_SERVERS`, `HTTP_PORT` to `go-data/.env`. Added a `go-data/.env` generation block to `scripts/setup-local.sh` so new installs get it automatically.

### Bug 3 — `strategies.go`: NULL `description` caused `GetStrategy` to return 404 instead of 403
**File:** `go-app/handlers/strategies.go`
**Symptom:** Strategies uploaded without a description have `description = NULL`. Scanning NULL into a non-pointer `string` caused pgx to return an error, which the handler treated as "not found" (404). A second user accessing another user's strategy would get 404 instead of 403. Also affected `ListStrategies`.
**Fix:** Added `COALESCE(s.description, '')` in both queries. Exposed and caught by `TestStrategyOwnership`.

---

ALL TASKS COMPLETE — no further audit runs needed.
