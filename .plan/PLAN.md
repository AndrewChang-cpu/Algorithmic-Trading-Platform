# Plan: ATP Code Quality Remediation
> Generated: 2026-05-27
> Type: brownfield
> Documents: single file
> Archived: [PLANv1.md](archive/PLANv1.md) (original feature plan)

## Overview
**What:** A targeted remediation of security vulnerabilities, functional bugs, and code quality issues identified in a full-codebase review of the Algorithmic Trading Platform. Covers all four service layers: Go API (`go-app`), Go data service (`go-data`), Python Celery workers, and React/TypeScript frontend. Also adds a new Python FastAPI validation service and updates the Kubernetes manifests.

**Why:** The review surfaced critical bugs (broken refresh token rotation, zombie Docker containers on timeout, fundamentally broken deduplication logic in go-data), security vulnerabilities (CORS bypass, XSS via `dangerouslySetInnerHTML`, JWT in WebSocket URL), pervasive missing error handling that causes silent data loss, and multi-tab auth race conditions.

**Who:** Internal — no user-facing feature changes. All public API contracts and UI flows remain identical.

## Definition of Done

### Go backend (`go-app`)
- [ ] `go run ./go-app/` without `CORS_ORIGINS` set exits immediately with a fatal log containing "CORS_ORIGINS must be set"
- [ ] `go test ./go-app/handlers/ -run TestAuth/Refresh` passes a case verifying the old refresh token row is deleted before new tokens are issued; if deletion fails the handler returns 500
- [ ] `middleware.GetUserID` has signature `func GetUserID(ctx context.Context) (string, bool)`; callers that receive `false` return HTTP 401
- [ ] `POST /api/strategies` with `import os` in the uploaded file returns 422 with a violation message; the check calls the Python validation service via `PYTHON_SERVICE_URL`, not the inline Go string scanner
- [ ] WebSocket connections to `/api/stream/jobs/{id}` and `/api/stream/portfolio/{id}` require the client to send `{"type":"auth","token":"<JWT>"}` as the first message within 10 seconds; invalid or missing message closes the connection; token expiry after connection is established does NOT close it
- [ ] `GET /api/auth/login` from the same IP returns HTTP 429 after 10 attempts within 60 seconds; `POST /api/auth/register` returns 429 after 5 attempts within 60 seconds; limits are tracked in Redis
- [ ] All `rows.Scan(...)` calls in `handlers/strategies.go`, `handlers/jobs.go`, and `handlers/stream.go` check the returned error; scan errors return 500 with a log entry
- [ ] `rows.Err()` is checked after every iteration loop across all handlers
- [ ] `s3client.PutObject(...)` error is checked in `UploadStrategy` and `UploadNewVersion`; if S3 upload fails, the DB row is not inserted and the handler returns 500
- [ ] On startup, `go-app` executes `UPDATE jobs SET status='failed', error_message='Server restarted', completed_at=NOW() WHERE status='running'` before accepting requests
- [ ] `strconv.Atoi` errors for `page` and `limit` query params return HTTP 400
- [ ] JWT middleware rejects tokens missing `sub` or `email` claims with 401
- [ ] `stream.go` logs a distinct error and closes the WebSocket with code 1011 when the Kafka broker returns a non-timeout error
- [ ] `go test ./go-app/...` passes

### Go data service (`go-data`)
- [ ] `POST /data/historical` for a range that has no existing rows fetches from Alpaca and inserts all bars; a second identical call does NOT call Alpaca again (`bars_ready` is returned from the DB count)
- [ ] `POST /data/historical` for a range that has a gap (e.g., data exists for Jan and Mar but not Feb) re-fetches the full requested range from Alpaca; `ON CONFLICT DO NOTHING` prevents duplicate inserts; final `bars_ready` reflects the true DB count
- [ ] If an INSERT within the transaction fails, the transaction is rolled back and the handler returns 500; no partial data is committed
- [ ] The final `SELECT COUNT(*)` error is checked; if it fails the handler returns 500
- [ ] Alpaca HTTP client has a 1-minute timeout; a hung Alpaca response does not hang the handler indefinitely
- [ ] Migration `008_create_market_data.up.sql` includes `CREATE UNIQUE INDEX IF NOT EXISTS market_data_symbol_resolution_time_idx ON market_data (symbol, resolution, time DESC)`
- [ ] `go test ./go-data/...` passes

### Python workers (`python/`)
- [ ] `POST http://localhost:8082/validate` (new FastAPI endpoint) with a valid strategy source returns `{"valid": true, "class_name": "MyStrategy"}`; with `import os` returns `{"valid": false, "violation": "import os detected on line N"}`; running under the same Docker image as the Celery worker via supervisord
- [ ] On `run_lean_backtest` timeout, `docker kill <container_id>` is called before raising `TimeoutError`; no zombie LEAN containers remain after a timeout
- [ ] `INSERT INTO performance_metrics` uses a hardcoded tuple of column names, not dynamic `metrics.keys()` string interpolation
- [ ] If LEAN's runtime statistics emit `Equity` (or any other field) as a non-string type, the Kafka message builder does not raise `AttributeError`; `isinstance(v, str)` guard applied before `.replace()`
- [ ] `data_materializer._to_ms(datetime(2024,1,2,9,30,0,500000))` returns `34200500` (microseconds included)
- [ ] Redis connections obtained via `_get_redis()` are explicitly closed in a `finally` block
- [ ] Non-zero return code from `docker stop <container_id>` raises a `RuntimeError` with the container ID in the message
- [ ] If `_fetch_market_data()` returns zero rows for any symbol, the job is immediately marked `failed` with `error_message = "No market data available for <symbol> in requested range"` before LEAN is invoked
- [ ] `results_parser.py` uses `except (ValueError, TypeError)` instead of bare `except` in all catch blocks
- [ ] `python3 -m pytest python/ -v` passes all existing tests
- [ ] `python -c "import celery_worker"` imports without error
- [ ] FastAPI service starts: `curl http://localhost:8082/health` returns 200

### React frontend (`web/`)
- [ ] After login or register, `useAuthStore.getState().user.email` equals the email submitted in the form
- [ ] `CodeViewer.tsx` contains no `dangerouslySetInnerHTML`; syntax highlighting uses `react-syntax-highlighter` with a dark theme
- [ ] No `!` non-null assertions on `accessToken` or `user` in `api.ts`; null cases are handled explicitly
- [ ] Both `useJobStatus` and `usePortfolio` initialize `mountedRef` as `useRef(true)` and set `false` only in the cleanup return; no separate mount-tracking `useEffect`
- [ ] A proactive token refresh fires at 12 minutes (80% of 15-minute lifetime); on refresh, new tokens are broadcast via `BroadcastChannel('auth')`; other open tabs receive the message and update their Zustand store without re-logging in
- [ ] App.tsx wraps all routes in an `ErrorBoundary`; an uncaught render error shows a fallback UI with a "Reload" button instead of a blank screen
- [ ] Every element targeted in E2E specs has a `data-testid` attribute; E2E tests use `getByTestId(...)` for those elements, not placeholder text or label text
- [ ] `INSERT INTO refresh_tokens` sets `expires_at = NOW() + INTERVAL '24 hours'` (auth.go)
- [ ] `StrategyDetail.tsx` `useEffect` that sets `selectedVersionId` lists only `strategy` in its dependency array, not `selectedVersionId`
- [ ] `npm run build` exits 0 with no TypeScript errors
- [ ] `cd web && npx playwright test` passes all 10 E2E specs

### Kubernetes
- [ ] `kubernetes/core/python-service.yaml` (or equivalent) contains a `ClusterIP Service` exposing port 8082 for the Python pod; go-app can reach the validation endpoint at `http://python-service:8082/validate` within the cluster

---

## Unchanged Behavior
- WHEN a user submits a valid strategy THEN the system SHALL continue to return `201 {strategyId, versionId, versionNumber}` (only the validation call source changes — from inline Go to Python service)
- WHEN a job is running THEN the WebSocket streams SHALL continue to send `{"type":"status"}` and `{"type":"log"}` / `{"type":"snapshot"}` messages in the same JSON shape
- WHEN a backtest completes THEN `performance_metrics` and `portfolio_metrics` rows SHALL continue to be inserted with the same schema
- WHEN a user registers or logs in THEN the response SHALL continue to include `{accessToken, refreshToken, userId}`
- WHEN a strategy is deleted THEN cascade deletion of versions, jobs, and S3 objects SHALL continue to work
- WHEN `POST /data/historical` is called and all data is already in the DB THEN the Alpaca API SHALL NOT be called (dedup preserved; only the gap-detection logic changes)

---

## Fixes by Area

### go-app: Security

**CORS fatal on missing env var** (`main.go`)
- Current: logic error allows all origins when `CORS_ORIGINS` is unset
- Fix: on startup, `log.Fatal("CORS_ORIGINS must be set")` if env var is empty

**JWT claims validation** (`middleware/jwt.go`)
- Current: missing `sub` or `email` claims produce empty string, silently bypassing auth
- Fix: return error from `RequireAuth` if either claim is absent; respond 401

**`GetUserID` signature** (`middleware/jwt.go`)
- Current: panics if called without `RequireAuth` in the chain
- Fix: `func GetUserID(ctx context.Context) (string, bool)`; callers return 401 on `false`

**Remove Go inline strategy scanner** (`handlers/strategies.go`)
- Current: string-based scan is bypassable and gives false security
- Fix: remove `validatePythonStrategy()` and all call sites; replace with HTTP call to `PYTHON_SERVICE_URL/validate`; 422 on `valid=false`, pass `class_name` through to S3 key / DB

**Rate limiting** (`handlers/auth.go`, new middleware or inline)
- Fix: Redis key `ratelimit:login:<IP>` incremented per request with 60s TTL; return 429 after 10 hits. `ratelimit:register:<IP>` same pattern, limit 5.

### go-app: Auth

**Refresh token rotation** (`handlers/auth.go`)
- Current: DELETE error is ignored; old token survives if deletion fails
- Fix: check `db.Pool.Exec` error; if deletion fails, return 500 (do not issue new token)

**Refresh token TTL** (`handlers/auth.go`)
- Fix: `expires_at = NOW() + INTERVAL '24 hours'` on all `INSERT INTO refresh_tokens`

### go-app: Error Handling

**`rows.Scan` and `rows.Err()`** (`handlers/strategies.go`, `handlers/jobs.go`, `handlers/stream.go`)
- Fix: check every `rows.Scan(...)` error; check `rows.Err()` after every `for rows.Next()` loop; log and return 500 on failure

**S3 upload error** (`handlers/strategies.go`)
- Fix: check `s3client.PutObject(...)` error; if it fails, do not insert strategy_versions row; return 500

**`strconv.Atoi` for pagination** (`handlers/jobs.go`)
- Fix: if `page` or `limit` parse fails, return 400 `{"error": "invalid page or limit"}`

### go-app: WebSocket Auth

**Replace query-param JWT with first-message auth** (`handlers/stream.go`)
- Current: `?token=<JWT>` in URL exposes token to browser history and server logs
- Fix:
  1. Remove `?token=` extraction; do not validate JWT before upgrade
  2. After upgrade, read first message with 10-second deadline
  3. Expect `{"type":"auth","token":"<JWT>"}`; validate JWT; send `{"type":"auth_ok"}` or close with code 1008
  4. If JWT expires after the session starts, keep the connection open
  5. Remove the fake-request auth pattern (`captureWriter`, `fakeReq`)
- Update frontend `useJobStatus` and `usePortfolio` to send auth message after connect

**Kafka error handling** (`handlers/stream.go` — `PortfolioStream`)
- Current: timeout errors and broker-down errors treated identically (silent continue)
- Fix: distinguish `kafka.ErrTimedOut` (continue loop) from all other errors (log + close WebSocket with code 1011)

### go-app: Startup

**Live job cleanup** (`main.go`)
- Fix: before `http.ListenAndServe`, execute:
  ```sql
  UPDATE jobs SET status='failed', error_message='Server restarted', completed_at=NOW() WHERE status='running'
  ```

---

### go-data: Deduplication Logic

**Gap-aware re-fetch** (`main.go` — `fetchAndInsert`)
- Current: `existingCount > 0` skips Alpaca even for partial ranges
- Fix:
  1. Query `MIN(time), MAX(time), COUNT(*)` for the symbol+resolution+range
  2. If `COUNT=0` OR `MIN > start_date` OR `MAX < end_date`: fetch full range from Alpaca
  3. If all data present (min ≤ start AND max ≥ end): skip Alpaca, return DB count
  4. `ON CONFLICT DO NOTHING` on insert handles any overlapping rows

**Transaction rollback** (`main.go` — `fetchAndInsert`)
- Current: INSERT error is logged and loop continues; partial data committed
- Fix: on any INSERT error, call `tx.Rollback(ctx)` and return the error immediately

**Final count error check** (`main.go`)
- Fix: check `rows.Scan` error on the final `SELECT COUNT(*)`; return error if it fails

**Alpaca HTTP timeout** (`main.go`)
- Fix: create `&http.Client{Timeout: 60 * time.Second}` instead of using `http.DefaultClient`

### go-data: Migration

**Unique index in migration 008** (`migrations/008_create_market_data.up.sql`)
- Fix: append `CREATE UNIQUE INDEX IF NOT EXISTS market_data_symbol_resolution_time_idx ON market_data (symbol, resolution, time DESC);`
- Corresponding `.down.sql`: `DROP INDEX IF EXISTS market_data_symbol_resolution_time_idx;`

---

### Python: FastAPI Validation Service

**New endpoint** (`python/strategy_validator.py` extended, new `python/app.py`)
- Add `python/app.py`:
  ```python
  from fastapi import FastAPI
  from pydantic import BaseModel
  from strategy_validator import validate_strategy

  app = FastAPI()

  class ValidateRequest(BaseModel):
      source: str

  @app.post("/validate")
  def validate(req: ValidateRequest):
      return validate_strategy(req.source)

  @app.get("/health")
  def health():
      return {"status": "ok"}
  ```
- Listen on port 8082 (via uvicorn in supervisord)
- Add `fastapi`, `uvicorn[standard]` to `python/requirements.txt`

**supervisord configuration** (`python/supervisord.conf`)
- `[program:celery]`: `celery -A celery_worker worker --loglevel=info --logfile=/logs/celery.log`
- `[program:api]`: `uvicorn app:app --host 0.0.0.0 --port 8082 --log-config /dev/null`
- Both programs: `autostart=true`, `autorestart=true`, `stopasgroup=true`

**Dockerfile update** (`python/Dockerfile`)
- Install `supervisor`
- `CMD ["supervisord", "-c", "/app/supervisord.conf"]`

### Python: Bug Fixes

**Zombie containers on timeout** (`lean_runner.py`)
- Current: `docker ps -q --filter ...` output is discarded; containers keep running
- Fix:
  ```python
  result = subprocess.run(["docker", "ps", "-q", "--filter", f"ancestor={LEAN_IMAGE}", "--filter", f"label=job_id={job_id}"], capture_output=True, text=True)
  for cid in result.stdout.strip().splitlines():
      subprocess.run(["docker", "kill", cid], check=False)
  ```
  Label containers at launch with `--label job_id={job_id}` so the filter is precise.

**Hardcoded SQL column tuple** (`celery_worker.py`)
- Replace dynamic `", ".join(metrics.keys())` with an explicit tuple constant at module level:
  ```python
  PERFORMANCE_METRICS_COLS = (
      "job_id", "total_return_pct", "annual_return_pct", "sharpe_ratio", ...
  )
  ```
- Build INSERT using only those columns; raise `KeyError` if any expected key is missing from `metrics`

**Isinstance guard for Kafka fields** (`celery_worker.py`)
- Fix `_build_kafka_snapshot()`:
  ```python
  def _strip_currency(v):
      if isinstance(v, str):
          return v.replace("$", "").replace(",", "").replace("-", "").lstrip("-")
      return str(v)
  ```

**Missing microseconds in ms calculation** (`data_materializer.py`)
- Current: `(h*3600 + m*60 + s) * 1000` drops sub-second precision
- Fix: `(h*3600 + m*60 + s) * 1000 + dt.microsecond // 1000`

**Redis connection leak** (`celery_worker.py`)
- Fix: wrap `_get_redis()` usage in `try/finally` with `r.close()` in the `finally` block

**Docker stop return code** (`lean_runner.py`)
- Fix: `subprocess.run(["docker", "stop", container_id], check=True, timeout=30)` — `check=True` raises `CalledProcessError` on non-zero exit

**Empty market data** (`celery_worker.py`)
- Fix: after `_fetch_market_data()`, if any symbol has zero rows, immediately mark job `failed`:
  ```python
  for symbol, rows in rows_by_symbol.items():
      if not rows:
          raise ValueError(f"No market data available for {symbol} in requested range")
  ```

**Bare except clauses** (`results_parser.py`)
- Fix: replace all bare `except:` with `except (ValueError, TypeError, KeyError):`

### Python: K8s Manifest

**ClusterIP Service** (`kubernetes/core/python-service.yaml`)
- New manifest:
  ```yaml
  apiVersion: v1
  kind: Service
  metadata:
    name: python-service
    namespace: atp-core
  spec:
    selector:
      app: celery-worker
    ports:
      - name: api
        port: 8082
        targetPort: 8082
    type: ClusterIP
  ```

---

### React: Auth

**Email stored after login/register** (`web/src/hooks/useAuth.ts`)
- Current: `setAuth({ id: data.userId, email: '' }, ...)`
- Fix: `setAuth({ id: data.userId, email: req.email }, ...)` — capture the email from the request payload before the mutation fires

**Refresh token TTL** (coordinated with go-app fix above; no frontend change needed)

**Proactive refresh + BroadcastChannel** (`web/src/lib/api.ts` or new `web/src/lib/tokenRefresh.ts`)
- On `setAuth(...)`, start a `setTimeout` for 12 minutes that calls `POST /api/auth/refresh`
- On success, call `setAuth(...)` with new tokens AND `channel.postMessage({type:'token_refresh', accessToken, refreshToken})` on `new BroadcastChannel('auth')`
- In App.tsx (or store init), listen: `channel.onmessage = (e) => { if (e.data.type === 'token_refresh') setAuth(...) }`
- Clear the timer on `clearAuth()`

**WebSocket first-message auth** (`web/src/hooks/useJobStatus.ts`, `web/src/hooks/usePortfolio.ts`)
- After `ws.onopen`, immediately send: `ws.send(JSON.stringify({ type: 'auth', token: accessToken }))`
- Remove `?token=...` from the WebSocket URL

### React: Security

**CodeViewer XSS** (`web/src/components/strategy/CodeViewer.tsx`)
- Remove `dangerouslySetInnerHTML` and hand-rolled regex highlighter
- Replace with `react-syntax-highlighter` using `Prism` renderer and `vscDarkPlus` theme
- Add `react-syntax-highlighter` and `@types/react-syntax-highlighter` to `web/package.json`

### React: Correctness

**Non-null assertions** (`web/src/lib/api.ts`)
- Replace `accessToken!` and `user!` with explicit null checks; throw or early-return with a meaningful error if null

**WebSocket hook cleanup race** (`web/src/hooks/useJobStatus.ts`, `web/src/hooks/usePortfolio.ts`)
- Remove the separate mount-tracking `useEffect`
- Initialize: `const mountedRef = useRef(true)`
- Cleanup: single `useEffect(() => () => { mountedRef.current = false }, [])`

**`StrategyDetail.tsx` useEffect** (`web/src/pages/StrategyDetail.tsx`)
- Remove `selectedVersionId` from the dependency array; depend only on `strategy`

**Error boundary** (`web/src/components/ErrorBoundary.tsx`, `web/src/App.tsx`)
- Create class component `ErrorBoundary` with `componentDidCatch` logging and a fallback UI: centered card with "Something went wrong" and a "Reload" button (`window.location.reload()`)
- Wrap `<QueryClientProvider>` children in `<ErrorBoundary>` in `App.tsx`

### React: E2E Tests

**`data-testid` attributes** (components + E2E specs)
- Add `data-testid` to every element currently targeted by E2E tests in `auth.spec.ts`, `strategies.spec.ts`, `backtest.spec.ts`
- Update all `getByPlaceholder(...)`, `getByText(...)` selectors that target form inputs or interactive controls to use `getByTestId(...)`
- Retain `getByText` for asserting visible text content (not for finding interaction targets)

---

## Out of Scope
- Password confirmation field on Register page (intentional UX decision)
- Relative import bypass in `strategy_validator.py` (`from . import os` fails at LEAN runtime before executing)
- WebSocket status polling optimization (deduped sends — see `TODO.md`)
- DB polling interval reduction for `stream.go` (see `TODO.md`)
- Rate limiting on non-auth endpoints
- Email format validation beyond `type="email"` HTML attribute
- Prometheus / Grafana monitoring
- Strategy signing or S3 checksum verification
- Concurrent backtest limits per user
- Test coverage for Docker daemon failures, DST edge cases, or S3 partial failures

## Assumptions
- The Python service (`celery-worker` Deployment) already has a K8s Deployment manifest; only a Service manifest is new
- `supervisord` is installable via `apt-get` in the `python:3.11-slim` base image
- `PYTHON_SERVICE_URL` env var will be set in local `.env` and as a K8s secret/configmap (same pattern as `GO_DATA_URL`)
- Alpaca's runtime statistics always emit currency fields as strings (confirmed from LEAN source `BaseResultsHandler.cs:941`)
- `BroadcastChannel` API is available in all target browsers (Chrome, Firefox, Safari 15.4+, Edge)

## Open Questions
None — all decisions resolved in planning session dated 2026-05-27.
