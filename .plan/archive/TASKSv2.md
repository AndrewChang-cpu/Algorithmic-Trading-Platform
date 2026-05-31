# Tasks: ATP Code Quality Remediation
> Generated: 2026-05-27
> Source: .plan/PLAN.md
> Total: 22 tasks | Starting points: 16

## Dependency Graph

```
T-01 · go-app/main.go: CORS fatal + startup job cleanup

T-02 · JWT middleware: claims validation + GetUserID signature
├── T-04* · strategies.go: remove Go scanner + Python service call + S3 check + rows.Scan
├── T-05  · jobs.go: rows.Scan + rows.Err + Atoi error handling
└── T-06  · stream.go: WS first-message auth + Kafka errors + rows.Scan
    └── T-18 · WS hooks: cleanup race fix + first-message auth

T-03 · auth.go: refresh token rotation + 24h TTL + Redis rate limiting

T-07 · go-data/main.go: gap-aware dedup + transaction rollback + Alpaca timeout

T-08 · migrations/008: add unique index to market_data

T-09 · python/app.py: FastAPI validation endpoint
└── T-10 · python/Dockerfile + supervisord.conf: run Celery + FastAPI together
    └── T-15 · kubernetes/celery/python-service.yaml: ClusterIP Service

T-11 · lean_runner.py: zombie container fix + docker stop check

T-12 · celery_worker.py: hardcoded SQL cols + isinstance guard + Redis close + empty data

T-13 · data_materializer.py: microseconds precision fix

T-14 · results_parser.py: replace bare except clauses

T-16 · api.ts: remove non-null assertions + proactive refresh + BroadcastChannel

T-17 · useAuth.ts: pass email to setAuth after login/register

T-19 · CodeViewer.tsx: replace dangerouslySetInnerHTML with react-syntax-highlighter

T-20 · StrategyDetail.tsx: fix useEffect dependency array

T-21 · ErrorBoundary.tsx + App.tsx: add global error boundary

T-22 · data-testid attributes + E2E selector updates
```

```
* T-04 also depends on T-09
```

---

## Tasks

### T-01 · go-app/main.go: CORS fatal + startup job cleanup
**Status:** `done`
**Depends on:** none
**Files:** `go-app/main.go`
**What:**
1. **CORS fatal:** In `main()`, read `CORS_ORIGINS` env var before any other setup. If empty, call `log.Fatal("CORS_ORIGINS must be set")`. In the CORS middleware, fix the logic: keep only `origins[origin]` (map lookup); remove the `len(origins) == 0` branch entirely so an unset env var can never fall through to allow-all.
2. **Startup cleanup:** After `db.InitDB(...)` succeeds and before `http.ListenAndServe`, execute:
   ```sql
   UPDATE jobs SET status='failed', error_message='Server restarted', completed_at=NOW() WHERE status='running'
   ```
   via the DB pool. Log the number of rows affected. If the query fails, log the error but do not fatal (server can still start).
**Done when:**
- `go run ./go-app/` without `CORS_ORIGINS` set exits with a log line containing "CORS_ORIGINS must be set"
- A preflight request from an origin not in `CORS_ORIGINS` does not receive `Access-Control-Allow-Origin` in the response
- On startup with DB running, `SELECT status FROM jobs WHERE id=<previously-running-job>` returns `failed`

---

### T-02 · JWT middleware: claims validation + GetUserID signature
**Status:** `done`
**Depends on:** none
**Files:** `go-app/middleware/jwt.go`, `go-app/middleware/jwt_test.go`
**What:**
1. **Claims validation:** In `RequireAuth`, after parsing JWT claims, assert both `claims["sub"].(string)` and `claims["email"].(string)` are non-empty strings. If either is missing or empty, write HTTP 401 and return without calling next.
2. **Extract `ValidateToken`:** Extract the JWT parsing + claims validation logic into `func ValidateToken(tokenStr string) (userID string, email string, err error)`. Both `RequireAuth` and stream.go's first-message auth (T-06) will call this.
3. **`GetUserID` signature:** Change from `func GetUserID(ctx context.Context) string` (panics) to `func GetUserID(ctx context.Context) (string, bool)`. Return `("", false)` if the key is absent.
4. **Tests:** Add to `jwt_test.go`: token with missing `sub` claim → 401; token with empty `email` claim → 401. Update existing tests for the new `GetUserID` signature.
**Done when:**
- `go test ./go-app/middleware/...` passes all tests including the two new claims cases
- `grep -n "GetUserID" go-app/middleware/jwt.go` shows signature `(context.Context) (string, bool)`
- `ValidateToken` is exported and callable from other packages

---

### T-03 · auth.go: refresh rotation + 24h TTL + Redis rate limiting
**Status:** `done`
**Depends on:** none
**Files:** `go-app/handlers/auth.go`, `go-app/handlers/auth_test.go`
**What:**
1. **Refresh rotation fix:** In `Refresh()`, check the error from `db.Pool.Exec("DELETE FROM refresh_tokens WHERE token_hash=$1", tokenHash)`. If `err != nil`, log the error and return HTTP 500. Do NOT issue new tokens.
2. **24h TTL:** In `Register()`, `Login()`, and `Refresh()`, change `expires_at` in `INSERT INTO refresh_tokens` to `NOW() + INTERVAL '24 hours'`.
3. **Rate limiting:** Add helper:
   ```go
   func checkRateLimit(ctx context.Context, rc *redis.Client, key string, max int, window time.Duration) error
   ```
   Uses Redis `INCR` + `EXPIREAT` (set TTL only when count becomes 1). Returns non-nil error if count > max.
   - In `Login()`: call with key `"ratelimit:login:" + clientIP`, max=10, window=1min. Return HTTP 429 `{"error":"too many requests"}` if exceeded.
   - In `Register()`: same with key `"ratelimit:register:" + clientIP`, max=5.
   - Extract `clientIP` from `X-Forwarded-For` header, fall back to `r.RemoteAddr` (strip port).
4. **Tests:** Add: rotation `DELETE` fails → 500 (mock exec error); 11th login attempt from same IP → 429; first 10 → not 429.
**Done when:**
- `go test ./go-app/handlers/ -run TestAuth` passes including new rate limit and rotation failure cases
- Making 11 sequential `POST /api/auth/login` requests from the same IP returns HTTP 429 on the 11th

---

### T-04 · strategies.go: remove Go scanner + Python service call + S3 check + rows.Scan
**Status:** `done`
**Depends on:** T-02, T-09
**Files:** `go-app/handlers/strategies.go`, `go-app/handlers/strategies_test.go`
**What:**
1. **Remove Go scanner:** Delete `validatePythonStrategy()` and all call sites.
2. **Python service call:** Add:
   ```go
   func validateWithPythonService(ctx context.Context, source string) (className string, violation string, err error)
   ```
   POSTs `{"source": source}` as JSON to `os.Getenv("PYTHON_SERVICE_URL") + "/validate"` with a 10-second `http.Client` timeout. On HTTP error or non-200: return `("", "", err)` (caller returns 500). On `valid=false` in response JSON: return `("", violation, nil)`. On `valid=true`: return `(class_name, "", nil)`.
3. Replace all former calls to `validatePythonStrategy` with `validateWithPythonService`. Return 422 on violation, 500 on service error.
4. **`io.ReadAll` error:** In `UploadStrategy()` and `UploadNewVersion()`, check `io.ReadAll(file)` error; return HTTP 400 on failure.
5. **S3 error:** In both upload handlers, check `s3client.PutObject(...)` error. If it fails, do NOT execute the `INSERT INTO strategy_versions` query; return HTTP 500.
6. **`rows.Scan` and `rows.Err()`:** In `ListStrategies()` and `GetStrategy()`, check every `rows.Scan(...)` error (log + return 500 on failure); check `rows.Err()` after every `for rows.Next()` loop.
7. **`GetUserID`:** Update all call sites to `id, ok := middleware.GetUserID(r.Context()); if !ok { writeError(w, 401, "unauthorized"); return }`.
8. **Tests:** Add: Python service returns violation → 422 with violation text; Python service unreachable → 500; S3 upload fails → 500, no row in strategy_versions; GetUserID returns false → 401.
**Done when:**
- `go test ./go-app/handlers/ -run TestStrategies` passes
- `grep "validatePythonStrategy" go-app/handlers/strategies.go` returns no matches
- `POST /api/strategies` with `import os` payload calls `PYTHON_SERVICE_URL/validate` (mock at test time) and returns 422

---

### T-05 · jobs.go: rows.Scan + rows.Err + Atoi error handling
**Status:** `done`
**Depends on:** T-02
**Files:** `go-app/handlers/jobs.go`, `go-app/handlers/jobs_test.go`
**What:**
1. **`rows.Scan` + `rows.Err()`:** In `ListJobs()`, check `rows.Scan(...)` error inside the loop (log + return 500); check `rows.Err()` after the loop. Same in `GetJobMetrics()` and `GetPortfolio()`.
2. **`strconv.Atoi` errors:** In `ListJobs()`, parse `page` and `limit` query params with `strconv.Atoi`. If either parse fails, return HTTP 400 `{"error":"invalid page or limit parameter"}`.
3. **Separate count query error:** Check `db.Pool.QueryRow(...).Scan(&total)` error in `ListJobs()`; return 500 if it fails.
4. **`GetUserID`:** Update all call sites to the new `(string, bool)` signature; return 401 if false.
5. **Tests:** Add: `?page=notanumber` → 400; `?limit=abc` → 400; scan error → 500 (use mock or verify via testhelper).
**Done when:**
- `go test ./go-app/handlers/ -run TestJobs` passes
- `GET /api/jobs?page=notanumber` returns HTTP 400 with JSON error body

---

### T-06 · stream.go: WS first-message auth + Kafka errors + rows.Scan
**Status:** `done`
**Depends on:** T-02
**Files:** `go-app/handlers/stream.go`
**What:**
1. **Remove old auth pattern:** Delete `validateWebSocketToken()` function and `captureWriter` type entirely.
2. **First-message auth — both handlers:** After `conn, err := upgrader.Upgrade(...)`, in both `JobStatusStream` and `PortfolioStream`:
   - Set read deadline: `conn.SetReadDeadline(time.Now().Add(10 * time.Second))`
   - Read one message; expect JSON `{"type":"auth","token":"<JWT>"}`
   - Call `middleware.ValidateToken(token)` → get `userID`, `email`, `err`
   - On any failure (deadline exceeded, bad JSON, invalid token): `conn.CloseHandler()(websocket.ClosePolicyViolation, "unauthorized")` and return
   - On success: clear deadline with `conn.SetReadDeadline(time.Time{})`, proceed
   - Send `{"type":"auth_ok"}` back to client
   - After auth, verify job ownership via DB query; close with 1008 if job doesn't belong to user
3. **JWT expiry after auth:** Do NOT re-validate the token on subsequent messages or poll ticks; once authenticated, the connection stays open.
4. **`rows.Scan` + `rows.Err()`:** In `JobStatusStream` polling loop, check `rows.Scan(...)` errors; check `rows.Err()` after the log-fetch loop; log errors but continue polling.
5. **Kafka error handling in `PortfolioStream`:** After `consumer.ReadMessage(500 * time.Millisecond)`, check if err is a timeout (`kafka.IsTimeout(err)` or equivalent for confluent-kafka-go) — if timeout, continue the loop. Any other error: log `"kafka consumer error: %v"`, close WebSocket with code 1011, return.
6. **`GetUserID`:** No longer needed post-auth-refactor; use `userID` returned from `ValidateToken` directly.
**Done when:**
- `go build ./go-app/...` exits 0
- WebSocket connection that sends no auth message within 10s closes with close code 1008
- WebSocket connection that sends valid JWT as first message receives `{"type":"auth_ok"}` and then proceeds to stream data
- WebSocket connection where JWT expires after auth succeeds stays open and continues streaming

---

### T-07 · go-data/main.go: gap-aware dedup + transaction rollback + Alpaca timeout
**Status:** `done`
**Depends on:** none
**Files:** `go-data/main.go`, `go-data/historical_test.go`
**What:**
1. **Gap-aware dedup:** Replace the `existingCount > 0` early-return with:
   ```go
   var minTime, maxTime *time.Time
   err := db.QueryRow(ctx,
       `SELECT MIN(time), MAX(time) FROM market_data
        WHERE symbol=$1 AND resolution=$2
        AND time >= $3::timestamptz AND time <= $4::timestamptz`,
       symbol, resolution, startDate, endDate,
   ).Scan(&minTime, &maxTime)
   ```
   Skip Alpaca only if `minTime != nil && maxTime != nil && !minTime.After(parsedStart) && !maxTime.Before(parsedEnd)`. Otherwise, fetch the full requested range from Alpaca (even if some rows already exist — `ON CONFLICT DO NOTHING` handles dups).
2. **Transaction rollback:** In the INSERT loop, if `tx.Exec(...)` returns an error: call `tx.Rollback(ctx)`, log the error, return the error to the caller (handler responds 500). Stop processing remaining bars.
3. **Final count error:** Check `rows.Scan` error on the final `SELECT COUNT(*)` query; if it fails, return the error (handler responds 500).
4. **Alpaca HTTP timeout:** Replace `http.DefaultClient` with `&http.Client{Timeout: 60 * time.Second}` for all Alpaca requests.
5. **Tests:** Update `TestDeduplication` to: after first fetch (4 rows inserted), delete 2 rows from DB, call the endpoint again → mock Alpaca server call count should be 2 (re-fetched because gap detected); DB row count still 4 (ON CONFLICT). Add `TestTransactionRollback`: mock an INSERT failure → handler returns 500, DB has 0 rows for that symbol.
**Done when:**
- `go test ./go-data/...` passes all tests including updated dedup and new rollback test
- Second call with all data present does NOT call Alpaca; call after partial deletion DOES call Alpaca

---

### T-08 · migrations/008: add unique index to market_data
**Status:** `done`
**Depends on:** none
**Files:** `migrations/008_create_market_data.up.sql`, `migrations/008_create_market_data.down.sql`
**What:**
Append to `008_create_market_data.up.sql`:
```sql
CREATE UNIQUE INDEX IF NOT EXISTS market_data_symbol_resolution_time_idx
  ON market_data (symbol, resolution, time DESC);
```
Prepend to `008_create_market_data.down.sql` (before the `DROP TABLE`):
```sql
DROP INDEX IF EXISTS market_data_symbol_resolution_time_idx;
```
**Done when:**
- With TimescaleDB running: `migrate -database "postgres://postgres:password@localhost:5432/atp?sslmode=disable" -path migrations up` exits 0
- `psql -U postgres -d atp -c "\di market_data*"` shows `market_data_symbol_resolution_time_idx`
- `INSERT INTO market_data ... ON CONFLICT DO NOTHING` does not insert duplicate rows for the same (symbol, resolution, time)

---

### T-09 · python/app.py: FastAPI validation endpoint
**Status:** `done`
**Depends on:** none
**Files:** `[new] python/app.py`, `python/requirements.txt`
**What:**
Create `python/app.py`:
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
Add to `python/requirements.txt`: `fastapi>=0.110.0` and `uvicorn[standard]>=0.27.0`.
**Done when:**
- `cd python && pip install -r requirements.txt` exits 0
- `cd python && uvicorn app:app --port 8082` starts without error
- `curl -s http://localhost:8082/health` returns `{"status":"ok"}`
- `curl -s -X POST http://localhost:8082/validate -H "Content-Type: application/json" -d '{"source":"class S(QCAlgorithm): pass"}' | jq .valid` returns `true`
- `curl -s -X POST http://localhost:8082/validate -H "Content-Type: application/json" -d '{"source":"import os\nclass S(QCAlgorithm): pass"}' | jq .valid` returns `false`

---

### T-10 · python/Dockerfile + supervisord.conf: run Celery + FastAPI together
**Status:** `done`
**Depends on:** T-09
**Files:** `python/Dockerfile`, `[new] python/supervisord.conf`, `python/requirements.txt`
**What:**
1. Create `python/supervisord.conf`:
```ini
[supervisord]
nodaemon=true
logfile=/logs/supervisord.log

[program:celery]
command=celery -A celery_worker worker --loglevel=info --logfile=/logs/celery.log
directory=/app
autostart=true
autorestart=true
stopasgroup=true
killasgroup=true
stdout_logfile=/dev/null
stderr_logfile=/dev/null

[program:api]
command=uvicorn app:app --host 0.0.0.0 --port 8082
directory=/app
autostart=true
autorestart=true
stopasgroup=true
killasgroup=true
stdout_logfile=/dev/null
stderr_logfile=/dev/null
```
2. Update `python/Dockerfile`:
   - After the `pip install` line, add: `RUN apt-get update && apt-get install -y --no-install-recommends supervisor && rm -rf /var/lib/apt/lists/*`
   - Add: `COPY supervisord.conf /app/supervisord.conf`
   - Change `CMD` to: `CMD ["supervisord", "-c", "/app/supervisord.conf"]`
3. Add `supervisor>=4.2.0` to `python/requirements.txt`.
**Done when:**
- `docker build -t atp-python:local python/` exits 0
- `docker run --rm atp-python:local supervisord --version` exits 0
- `docker run -d --name test-atp-python atp-python:local && sleep 5 && docker exec test-atp-python ps aux | grep -E "celery|uvicorn"` shows both processes running; `docker rm -f test-atp-python`

---

### T-11 · lean_runner.py: zombie container fix + docker stop check
**Status:** `done`
**Depends on:** none
**Files:** `python/lean_runner.py`, `python/test_lean_runner.py`
**What:**
1. **Label containers at launch:** In `run_lean_backtest()` and `run_lean_live()`, add `--label`, `f"job_id={job_id}"` to the `docker run` command list so containers can be found by label.
2. **Kill on timeout:** In the `except subprocess.TimeoutExpired` block of `run_lean_backtest()`, replace the no-op with:
   ```python
   kill_result = subprocess.run(
       ["docker", "ps", "-q", "--filter", f"label=job_id={job_id}"],
       capture_output=True, text=True
   )
   for cid in kill_result.stdout.strip().splitlines():
       if cid:
           subprocess.run(["docker", "kill", cid], check=False)
   raise TimeoutError(f"LEAN container for job {job_id} timed out after {timeout_seconds}s")
   ```
3. **Docker stop check:** In `stop_lean_live()`, change `subprocess.run(["docker", "stop", container_id], timeout=30)` to `subprocess.run(["docker", "stop", container_id], timeout=30, check=True)`. This raises `subprocess.CalledProcessError` on non-zero exit.
4. **Tests:** Add to `test_lean_runner.py`: test that timeout handler calls `docker kill <container_id>` (mock `docker ps` to return a container ID, assert `docker kill` is called with it); test that `stop_lean_live()` raises `CalledProcessError` when `docker stop` exits non-zero.
**Done when:**
- `python3 -m pytest python/test_lean_runner.py -v` passes all tests including the two new cases
- The timeout handler test verifies `docker kill` is called with the container ID returned by `docker ps`

---

### T-12 · celery_worker.py: hardcoded SQL cols + isinstance + Redis close + empty data
**Status:** `done`
**Depends on:** none
**Files:** `python/celery_worker.py`
**What:**
1. **Hardcoded SQL column tuple:** Define a module-level constant `PERFORMANCE_METRICS_COLS` as an explicit tuple of all column names in the `performance_metrics` table (matching the migration schema). Replace the dynamic `", ".join(metrics.keys())` with `", ".join(PERFORMANCE_METRICS_COLS)`. Build the values list as `[metrics[col] for col in PERFORMANCE_METRICS_COLS]`. If any expected key is missing from `metrics`, this raises `KeyError` which is caught by the outer exception handler and marks the job failed.
2. **isinstance guard:** In `_build_kafka_snapshot()` (or wherever `runtime.get("Equity", "0").replace(...)` appears), add:
   ```python
   def _strip_currency(v):
       s = v if isinstance(v, str) else str(v)
       return s.replace("$", "").replace(",", "").lstrip("-")
   ```
   Apply to all runtime fields: `Equity`, `Unrealized`, `Holdings`, `Fees`.
3. **Redis connection close:** Wrap every `r = _get_redis()` usage in `try/finally` with `r.close()` in the `finally` block. If `_get_redis()` is called in multiple places, apply the pattern to each.
4. **Empty market data fail-fast:** After `rows_by_symbol = _fetch_market_data(...)`, iterate over the requested symbols list. For each symbol where `rows_by_symbol.get(symbol, [])` is empty, raise `ValueError(f"No market data available for {symbol} in requested range")` before invoking LEAN. This exception is caught by the outer `except Exception` in the task body that sets `job.status = 'failed'` and writes `error_message`.
**Done when:**
- `python -c "import celery_worker"` imports without error
- `grep "metrics.keys()" python/celery_worker.py` returns no matches
- `python3 -m pytest python/test_celery_worker.py -v` passes all existing tests

---

### T-13 · data_materializer.py: milliseconds precision fix
**Status:** `done`
**Depends on:** none
**Files:** `python/data_materializer.py`, `python/test_data_materializer.py`
**What:**
In `_to_ms(dt: datetime) -> int` (or the equivalent inline expression), change:
```python
# Before
(dt.hour * 3600 + dt.minute * 60 + dt.second) * 1000
# After
(dt.hour * 3600 + dt.minute * 60 + dt.second) * 1000 + dt.microsecond // 1000
```
Add test in `test_data_materializer.py`:
```python
def test_to_ms_includes_microseconds():
    dt = datetime(2024, 1, 2, 9, 30, 0, 500000)  # 9:30:00.500
    assert _to_ms(dt) == 34200500  # 9*3600000 + 30*60000 + 500
```
**Done when:**
- `python3 -m pytest python/test_data_materializer.py -v` passes all tests including the new microseconds test
- `_to_ms(datetime(2024, 1, 2, 9, 30, 0, 500000))` returns `34200500`

---

### T-14 · results_parser.py: replace bare except clauses
**Status:** `done`
**Depends on:** none
**Files:** `python/results_parser.py`
**What:**
Find every bare `except:` clause in `results_parser.py` and replace with `except (ValueError, TypeError, KeyError):`. Verify no other bare excepts remain in the file.
**Done when:**
- `python3 -m pytest python/test_results_parser.py -v` passes all tests
- `grep -n "except:" python/results_parser.py` returns no output

---

### T-15 · kubernetes/celery/python-service.yaml: ClusterIP Service
**Status:** `done`
**Depends on:** T-10
**Files:** `[new] kubernetes/celery/python-service.yaml`
**What:**
Create `kubernetes/celery/python-service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: python-service
  namespace: default
spec:
  selector:
    app: celery-worker
  ports:
    - name: api
      port: 8082
      targetPort: 8082
  type: ClusterIP
```
The selector `app: celery-worker` matches the label in `kubernetes/celery/celery-worker-deployment.yaml`. Within the cluster, go-app reaches the validation endpoint at `http://python-service:8082/validate`.
**Done when:**
- `kubectl apply -f kubernetes/celery/python-service.yaml --dry-run=client` exits 0
- `selector.app: celery-worker` matches `metadata.labels.app` in `kubernetes/celery/celery-worker-deployment.yaml` (verified by visual inspection)

---

### T-16 · api.ts: remove non-null assertions + proactive refresh + BroadcastChannel
**Status:** `done`
**Depends on:** none
**Files:** `web/src/lib/api.ts`, `web/src/lib/store.ts`, `web/src/App.tsx`
**What:**
1. **Non-null assertions:** In `api.ts`, replace every `accessToken!` with an explicit null check:
   ```typescript
   const token = useAuthStore.getState().accessToken
   if (!token) throw new Error('Not authenticated')
   ```
   Same for any `user!` assertions. The response interceptor's refresh path already has a null check — verify it.
2. **Proactive refresh timer:** In `store.ts`, add `refreshTimer: ReturnType<typeof setTimeout> | null` to state. In `setAuth()`, clear any existing timer, then schedule:
   ```typescript
   const timer = setTimeout(async () => {
     try {
       const { data } = await apiClient.post('/api/auth/refresh', { refreshToken: get().refreshToken })
       get().setAuth({ id: get().user!.id, email: get().user!.email }, data.accessToken, data.refreshToken)
     } catch {
       get().clearAuth()
     }
   }, 12 * 60 * 1000)
   set({ refreshTimer: timer })
   ```
   In `clearAuth()`, clear the timer with `clearTimeout(get().refreshTimer)`.
3. **BroadcastChannel broadcast:** Inside the proactive refresh success path, after calling `setAuth(...)`:
   ```typescript
   const ch = new BroadcastChannel('auth')
   ch.postMessage({ type: 'token_refresh', accessToken: data.accessToken, refreshToken: data.refreshToken })
   ch.close()
   ```
4. **BroadcastChannel listener:** In `App.tsx`, add a `useEffect` at the root level:
   ```typescript
   useEffect(() => {
     const ch = new BroadcastChannel('auth')
     ch.onmessage = (e) => {
       if (e.data.type === 'token_refresh') {
         const { user, setAuth } = useAuthStore.getState()
         if (user) setAuth(user, e.data.accessToken, e.data.refreshToken)
       }
     }
     return () => ch.close()
   }, [])
   ```
**Done when:**
- `cd web && npm run build` exits 0 with no TypeScript errors
- `grep -n "!" web/src/lib/api.ts` shows no non-null assertions on `accessToken` or `user`
- `BroadcastChannel` appears in both `store.ts` and `App.tsx`

---

### T-17 · useAuth.ts: pass email to setAuth
**Status:** `done`
**Depends on:** none
**Files:** `web/src/hooks/useAuth.ts`
**What:**
In `useLogin`, capture `req` in `onSuccess` and pass `req.email` to `setAuth`:
```typescript
const useLogin = () => {
  const { setAuth } = useAuthStore()
  return useMutation({
    mutationFn: (req: { email: string; password: string }) =>
      apiClient.post('/api/auth/login', req).then(r => r.data),
    onSuccess: (data, req) =>
      setAuth({ id: data.userId, email: req.email }, data.accessToken, data.refreshToken),
  })
}
```
Apply the same pattern to `useRegister` (same structure, `req.email`).
**Done when:**
- `cd web && npm run build` exits 0
- After `useLogin` mutation resolves, `useAuthStore.getState().user?.email` equals the email passed in the request (verifiable in a unit test or via browser console)

---

### T-18 · WS hooks: cleanup race fix + first-message auth
**Status:** `done`
**Depends on:** T-06
**Files:** `web/src/hooks/useJobStatus.ts`, `web/src/hooks/usePortfolio.ts`
**What:** Apply identically to both hooks:
1. **Cleanup race fix:**
   - Remove the separate `useEffect` that sets `mountedRef.current = true`
   - Change initialization to `const mountedRef = useRef(true)`
   - Single cleanup effect: `useEffect(() => () => { mountedRef.current = false }, [])`
2. **First-message auth:**
   - Remove `?token=${encodeURIComponent(accessToken!)}` from the WebSocket URL. URL becomes `${WS_BASE}/api/stream/jobs/${jobId}` (no query params).
   - In `ws.onopen`: immediately send `ws.send(JSON.stringify({ type: 'auth', token: accessToken }))`
   - In `ws.onmessage`: check if `parsed.type === 'auth_ok'` and if so, return early (do not pass auth_ok to state). All other message types continue to existing handling.
   - If `accessToken` is null when the hook tries to connect, close the WebSocket immediately rather than sending a null token.
**Done when:**
- `cd web && npm run build` exits 0 with no TypeScript errors
- `grep -n "token=" web/src/hooks/useJobStatus.ts web/src/hooks/usePortfolio.ts` returns no matches containing `?token=`
- Both hooks send `{"type":"auth",...}` as the first WebSocket message (verifiable via browser devtools Network → WS frames)

---

### T-19 · CodeViewer.tsx: replace dangerouslySetInnerHTML with react-syntax-highlighter
**Status:** `done`
**Depends on:** none
**Files:** `web/src/components/strategy/CodeViewer.tsx`, `web/package.json`, `web/package-lock.json`
**What:**
1. In `web/`, run: `npm install react-syntax-highlighter @types/react-syntax-highlighter`
2. Rewrite `CodeViewer.tsx` entirely:
```tsx
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism'

interface Props { code: string }

export function CodeViewer({ code }: Props) {
  return (
    <SyntaxHighlighter
      language="python"
      style={vscDarkPlus}
      showLineNumbers
      customStyle={{ background: '#0d1117', margin: 0, fontSize: 13, borderRadius: 6 }}
    >
      {code}
    </SyntaxHighlighter>
  )
}
```
3. Delete all prior regex-based highlighting logic and `dangerouslySetInnerHTML` usage.
**Done when:**
- `cd web && npm run build` exits 0
- `grep -r "dangerouslySetInnerHTML" web/src/components/strategy/CodeViewer.tsx` returns no matches
- `react-syntax-highlighter` is listed in `web/package.json` dependencies

---

### T-20 · StrategyDetail.tsx: fix useEffect dependency array
**Status:** `done`
**Depends on:** none
**Files:** `web/src/pages/StrategyDetail.tsx`
**What:**
Find the `useEffect` that initializes `selectedVersionId` from the loaded strategy. Remove `selectedVersionId` from the dependency array so the effect depends only on `strategy`:
```typescript
useEffect(() => {
  if (strategy?.versions?.length && !selectedVersionId) {
    setSelectedVersionId(strategy.versions[0].id)
  }
  // eslint-disable-next-line react-hooks/exhaustive-deps
}, [strategy])
```
The eslint-disable comment is justified because `selectedVersionId` is intentionally excluded: the effect's purpose is "set once when strategy first loads" not "re-run whenever selectedVersionId changes".
**Done when:**
- `cd web && npm run build` exits 0 with no TypeScript errors
- The `useEffect` dependency array contains only `strategy`, not `selectedVersionId`

---

### T-21 · ErrorBoundary.tsx + App.tsx: global error boundary
**Status:** `done`
**Depends on:** none
**Files:** `[new] web/src/components/ErrorBoundary.tsx`, `web/src/App.tsx`
**What:**
1. Create `web/src/components/ErrorBoundary.tsx`:
```tsx
import { Component, ErrorInfo, ReactNode } from 'react'

interface Props { children: ReactNode }
interface State { hasError: boolean }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false }

  static getDerivedStateFromError(): State {
    return { hasError: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[ErrorBoundary]', error, info.componentStack)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', background: '#0d1117', color: '#c9d1d9', gap: 16 }}>
          <p style={{ color: '#f85149', margin: 0 }}>Something went wrong.</p>
          <button
            onClick={() => window.location.reload()}
            style={{ padding: '8px 20px', background: '#388bfd', color: '#fff', border: 'none', borderRadius: 6, cursor: 'pointer' }}
          >
            Reload
          </button>
        </div>
      )
    }
    return this.props.children
  }
}
```
2. In `App.tsx`, wrap the outermost JSX content inside `<ErrorBoundary>`:
```tsx
import { ErrorBoundary } from './components/ErrorBoundary'

export default function App() {
  return (
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          {/* existing routes */}
        </BrowserRouter>
      </QueryClientProvider>
    </ErrorBoundary>
  )
}
```
**Done when:**
- `cd web && npm run build` exits 0
- `web/src/components/ErrorBoundary.tsx` exports `ErrorBoundary`
- `App.tsx` imports and wraps content in `<ErrorBoundary>`

---

### T-22 · data-testid attributes + E2E selector updates
**Status:** `done`
**Depends on:** none
**Files:** `web/src/pages/Login.tsx`, `web/src/pages/Register.tsx`, `web/src/components/strategy/UploadModal.tsx`, `web/src/pages/Strategies.tsx`, `web/src/pages/Backtests.tsx`, `web/src/components/jobs/RunBacktestModal.tsx`, `web/src/components/jobs/StatusBadge.tsx`, `web/e2e/auth.spec.ts`, `web/e2e/strategies.spec.ts`, `web/e2e/backtest.spec.ts`
**What:**
1. **Add `data-testid` attributes** to every element targeted by interaction in E2E specs:
   - `Login.tsx`: `data-testid="email-input"`, `data-testid="password-input"`, `data-testid="login-submit"`, `data-testid="auth-error"`
   - `Register.tsx`: `data-testid="register-email-input"`, `data-testid="register-password-input"`, `data-testid="register-submit"`, `data-testid="auth-error"`
   - `Strategies.tsx`: `data-testid="strategies-empty-state"`, `data-testid="strategy-row"`, `data-testid="upload-strategy-btn"`
   - `UploadModal.tsx`: `data-testid="upload-file-input"`, `data-testid="strategy-name-input"`, `data-testid="upload-submit"`, `data-testid="upload-success"`, `data-testid="upload-error"`
   - `RunBacktestModal.tsx`: `data-testid="symbols-input"`, `data-testid="start-date-input"`, `data-testid="end-date-input"`, `data-testid="resolution-select"`, `data-testid="submit-backtest"`, `data-testid="job-queued-confirmation"`
   - `Backtests.tsx`: `data-testid="backtests-empty-state"`, `data-testid="job-row"`, `data-testid="status-badge"`
   - `StatusBadge.tsx`: `data-testid="status-badge"`
2. **Update E2E specs:** Replace every `getByPlaceholder(...)`, `getByRole('textbox')`, or similar selector that targets an element receiving `.fill()` or `.click()` with `page.getByTestId(...)`. Keep `getByText(...)` only for asserting visible text content, not for finding interaction targets.
**Done when:**
- `cd web && npx playwright test` passes all 10 E2E tests
- `grep -n "getByPlaceholder\|getByRole.*textbox" web/e2e/*.spec.ts` returns no lines that precede a `.fill()` or `.click()` call

---

## Open Questions
None — all ambiguities resolved or inferred from codebase probe and planning session.
