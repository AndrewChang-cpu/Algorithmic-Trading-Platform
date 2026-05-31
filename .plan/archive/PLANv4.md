# Plan: ATP Wave 4 — Production Hardening
> Generated: 2026-05-29
> Type: brownfield
> Documents: single file
> Archived: [PLANv3.md](archive/PLANv3.md) (Wave 3 security hardening & bug fixes)

## Overview
**What:** A comprehensive fourth wave targeting every remaining issue surfaced by the Wave 3 code review: financial data corruption, data integrity bugs, auth security, WebSocket reliability, K8s architecture (remove Docker socket via K8s Jobs), Python worker reliability, and test quality. Goal is production-ready code across all layers.

**Why:** Wave 3 implementation left three plan-level gaps unimplemented, and the review surfaced new issues including a financial data corruption bug (`_strip_currency` sign inversion), an incomplete S3/DB fix in `UploadStrategy`, K8s node scheduling problems with the current docker-socket approach, and missing auth security (httpOnly cookies, security headers, refresh rate limiting). The Docker socket in the Celery worker pod is the highest-risk architectural gap.

**Who:** Internal — no user-visible behavior changes except auth token handling (invisible to user, more secure).

---

## Definition of Done

### Wave 3 gaps

- [ ] `lean_runner.py` module level: `KAFKA_NODE_IP = os.environ["KAFKA_NODE_IP"]` — no default fallback; `python3 -c "import lean_runner"` raises `KeyError` when `KAFKA_NODE_IP` is absent
- [ ] `run_lean_live_task` in `celery_worker.py`: `producer.flush(timeout=10)` is inside the outer `finally` block (not inside the outer `try`); a simulated inner-block exception does not skip the flush
- [ ] `GET /api/jobs/:id/metrics`: DB query error → HTTP 500 "database error"; no matching row → HTTP 404 "metrics not available yet"; `go test ./go-app/handlers/ -run TestGetJobMetrics` covers both branches

### Financial data integrity

- [ ] `_strip_currency` returns `float`; `_strip_currency("-$1,234.56")` == `-1234.56`; `_strip_currency("$0")` == `0.0`; `_strip_currency("1234.56")` == `1234.56`; `python3 -m pytest python/ -k test_strip_currency -v` passes

### Go API data integrity

- [ ] `POST /api/strategies` when `strategy_versions` INSERT fails after S3 PutObject: handler returns 500 AND the S3 object at `{userID}/{strategyID}/v1/main.py` does NOT exist; `go test ./go-app/handlers/ -run TestUploadStrategy` covers this case
- [ ] `GET /api/strategies/:id` issues exactly one database query (single `SELECT ... JOIN strategy_versions`); ownership is checked before any data is written to the response struct; `go test ./go-app/handlers/ -run TestGetStrategy` passes
- [ ] `SubmitJob`, `GetPortfolio`, `CancelJob`, `DeleteStrategy` — `QueryRow().Scan()` error that is not `ErrNoRows`: handler returns HTTP 500 (not 403/404); `go test ./go-app/...` covers each
- [ ] `GET /api/jobs?limit=201` returns HTTP 400 "limit must be between 1 and 200"
- [ ] `POST /api/auth/refresh` called 21 times in 1 minute from the same IP: 21st call returns 429
- [ ] Go API responses include `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Strict-Transport-Security: max-age=31536000; includeSubDomains` headers on every response
- [ ] `middleware.RequireAuth` 401 response body is exactly `{"error":"invalid or expired token"}` regardless of the underlying JWT error

### httpOnly cookie auth

- [ ] `POST /api/auth/login` response body contains `accessToken` and `userId` but NOT `refreshToken`; response sets `Set-Cookie: refresh_token=...; HttpOnly; SameSite=Lax; Path=/api/auth; Max-Age=86400` (and `Secure` when `APP_ENV=production`)
- [ ] `POST /api/auth/refresh` with no JSON body but a valid `refresh_token` cookie: returns new `accessToken` in body and rotates the cookie; same call with no cookie returns 401
- [ ] `POST /api/auth/logout` reads `refresh_token` from cookie, deletes from DB, clears cookie via `Set-Cookie: refresh_token=; Max-Age=0; HttpOnly; SameSite=Lax; Path=/api/auth`; returns 204
- [ ] `store.ts`: no `persist()` middleware; `accessToken` exists only in Zustand in-memory state; opening DevTools → Application → Local Storage shows no `atp-auth` key
- [ ] `web/vite.config.ts` proxies `/api` → `http://localhost:8080` in dev mode; `npm run dev` fetch to `/api/auth/login` reaches the Go server without a CORS error
- [ ] `npm run build` exits 0

### WebSocket / streaming

- [ ] `JobStatusStream` goroutine exits within 5 seconds of the WebSocket client disconnecting (tested by closing the connection while the job is in `running` state and verifying no goroutine leak via `runtime.NumGoroutine()`)
- [ ] `PortfolioStream`: opening 6 concurrent WebSocket connections for the same `userID` — the 6th connection receives HTTP 429 before the WebSocket upgrade
- [ ] `GET /api/health` public response body is `{"status":"ok"}` only; the `kafka`, `redis`, `db` fields are absent from the public body; Redis connectivity failure causes the readiness probe to return non-200 (pod pulled from load balancer)

### Python worker reliability

- [ ] `run_lean_backtest_task("not-a-uuid", ...)` marks the job row `status='failed'` with `error_message` containing "invalid job_id format" and does not attempt to create any directory or K8s Job
- [ ] `_create_lean_network` subprocess call has `timeout=10`; a `subprocess.TimeoutExpired` is caught and logged as a warning; `python3 -m pytest python/ -k test_create_lean_network -v` passes
- [ ] `run_lean_live_task` exception handler: `stop_lean_live` failure is logged and does NOT replace the original exception; the original exception message reaches `_update_job_status`
- [ ] A bare `logging.getLogger(__name__).info("msg")` call (not via `_logger()`) does NOT raise `KeyError: 'job_id'`; log record contains `"job_id": "-"`
- [ ] `python3 -c "import celery_worker"` with `DATABASE_URL` unset prints `EnvironmentError: Required environment variable 'DATABASE_URL' is not set` (not a bare `KeyError`)
- [ ] `GET /api/jobs/:id` when the job failed due to a LEAN container path error: `error_message` does NOT contain `/tmp/atp-jobs/` or any filesystem path; message is truncated to ≤300 characters

### Strategy validator

- [ ] `validate_strategy("import pty")` returns `{"valid": false, "violation": "...pty..."}`; same for `import pickle`; same for source containing `open("secret.txt")`; same for source containing `breakpoint()`
- [ ] `python3 -m pytest python/test_strategy_validator.py -v` passes including new test cases for all four

### K8s architecture — Docker socket removal

- [ ] `kubernetes/celery/celery-worker-deployment.yaml` contains NO `hostPath` volume entry for Docker socket; contains `serviceAccountName: lean-job-runner`
- [ ] `kubernetes/celery/serviceaccount.yaml` exists: `kind: ServiceAccount`, `name: lean-job-runner`
- [ ] `kubernetes/celery/rbac.yaml` exists: `kind: Role` permitting `create/get/delete/list` on `batch/v1` `jobs` and `get/list` on `pods` in namespace `default`; `kind: RoleBinding` binding it to `lean-job-runner`
- [ ] `lean_runner.py` contains zero calls to `subprocess.run(["docker", ...)` ; `run_lean_backtest`, `run_lean_live`, `stop_lean_live`, and `is_container_running` all reference `kubernetes.client.BatchV1Api` or `CoreV1Api`
- [ ] After a simulated backtest task: `aws s3 ls s3://{bucket}/jobs/{job_id}/input/config.json` returns a result before the K8s Job is created; the `jobs/{job_id}/` prefix is absent after task `finally` block runs
- [ ] `lean-plugin/Dockerfile` adds `awscli` and copies `entrypoint.sh`; `entrypoint.sh` traps SIGTERM, starts LEAN, waits for LEAN to exit, then runs `aws s3 sync /lean/Results s3://${S3_BUCKET}/jobs/${JOB_ID}/results/`
- [ ] K8s Job spec (in `lean_runner.py`): `terminationGracePeriodSeconds: 120`; `nodeSelector: {dedicated: lean-worker}`; `tolerations: [{key: dedicated, value: lean-worker, effect: NoSchedule}]`; `ttlSecondsAfterFinished: 3600`; `backoffLimit: 0`; `resources.requests: {memory: 1Gi, cpu: "1"}`; `resources.limits: {memory: 3Gi, cpu: "2"}`
- [ ] `kops.yaml` contains an InstanceGroup `lean-nodes` with `machineType: t3.medium`, `minSize: 0`, `maxSize: 3`, taint `dedicated=lean-worker:NoSchedule`, label `dedicated: lean-worker`
- [ ] `python3 -m pytest python/test_lean_runner.py -v` passes with K8s API mocked via `unittest.mock.patch`

### K8s security

- [ ] `kubernetes/go-app/deployment.yaml` container spec has `securityContext`: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `capabilities.drop: [ALL]`
- [ ] `kubernetes/celery/celery-worker-deployment.yaml` container spec has `securityContext`: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`
- [ ] `celery-worker-deployment.yaml` env var `LEAN_KAFKA_BOOTSTRAP_SERVERS` is removed (Celery worker no longer needs it — LEAN pod connects to Kafka directly via `kafka:9092` K8s Service); the K8s Job spec in `lean_runner.py` passes `KAFKA_BOOTSTRAP_SERVERS=kafka:9092` as an env var to the LEAN container

### Test quality

- [ ] `python/conftest.py` `_count()` raises `ValueError` for any table name not in `{"performance_metrics", "portfolio_metrics", "job_logs"}`
- [ ] `python/test_lean_runner.py` `test_backtest_timeout_kills_all_containers`: mock for the `docker kill` equivalent uses `returncode=1` (not `side_effect=CalledProcessError`); test asserts on logged error message
- [ ] `python/test_celery_worker.py` `test_lean_runtime_error`: assertion checks `job["error_message"]` contains the string the mock actually raises (not a hardcoded literal that diverges from runtime behavior)
- [ ] `python3 -m pytest python/ -v` passes all tests
- [ ] `go test ./go-app/...` passes

---

## Unchanged Behavior

- WHEN a user submits a valid backtest job THEN the response SHALL continue to be `202 Accepted` with `{"jobId": "<uuid>"}`
- WHEN a backtest completes THEN `performance_metrics` and `portfolio_metrics` rows SHALL continue to be inserted with the same schema
- WHEN a user uploads a valid strategy THEN the response SHALL continue to be `201 Created` with `{"strategyId": ..., "versionId": ..., "versionNumber": 1}`
- WHEN a WebSocket client sends a valid first-message auth `{"type":"auth","token":"<JWT>"}` THEN streaming SHALL continue to work
- WHEN any REST endpoint is called with a valid `Authorization: Bearer <JWT>` THEN it SHALL continue to accept that token (Bearer header auth not removed)

---

## Fixes by Area

### A. Wave 3 gaps

#### A1. `lean_runner.py` — KAFKA_NODE_IP hard env require
```python
KAFKA_NODE_IP = os.environ["KAFKA_NODE_IP"]  # no default; fails fast if missing
```

#### A2. `celery_worker.py` — producer.flush() in outer finally
Restructure `run_lean_live_task` outer block:
```python
producer = None
try:
    # ... main live logic ...
except Exception as e:
    _update_job_status(conn, job_id, "failed", _sanitize_error(str(e)))
    log.error(f"Live job failed: {e}")
    if container_id:
        try:
            stop_lean_live(container_id, job_dir)
        except Exception as stop_err:
            log.error("Failed to stop container/job %s: %s", container_id, stop_err)
finally:
    if producer is not None:
        producer.flush(timeout=10)
    conn.close()
    if os.path.exists(job_dir):
        shutil.rmtree(job_dir, ignore_errors=True)
```

#### A3. `jobs.go` — GetJobMetrics 404/500 split
```go
rows, err := db.Pool.Query(r.Context(), "SELECT * FROM performance_metrics WHERE job_id = $1", jobID)
if err != nil {
    writeError(w, http.StatusInternalServerError, "database error")
    return
}
defer rows.Close()
if !rows.Next() {
    writeError(w, http.StatusNotFound, "metrics not available yet")
    return
}
```

---

### B. Financial data integrity

#### B1. `celery_worker.py` — _strip_currency returns float
```python
def _strip_currency(v) -> float:
    s = str(v)
    negative = s.lstrip("-") != s
    cleaned = s.replace("$", "").replace(",", "").lstrip("+-")
    try:
        result = float(cleaned) if cleaned else 0.0
    except ValueError:
        result = 0.0
    return -result if negative else result
```
Update all call sites in the Kafka snapshot dict to store float values.

---

### C. Go API data integrity

#### C1. `strategies.go` — UploadStrategy S3/DB order
Apply same fix as `UploadNewVersion`: INSERT strategy_versions first → if INSERT fails return 500 (S3 never touched); then S3 PutObject → if S3 fails, DELETE strategy_versions row, return 500.

Full order for `UploadStrategy`:
1. INSERT into `strategies` → capture `strategyID`
2. INSERT into `strategy_versions (strategy_id, version_number, s3_key) VALUES ($1, 1, $2)` → capture `versionID`; on failure return 500
3. `s3client.PutObject(...)` → on failure: `DELETE FROM strategy_versions WHERE id=$1`, return 500

#### C2. `strategies.go` — GetStrategy single query
Replace two `QueryRow` calls with one:
```go
err := db.Pool.QueryRow(r.Context(), `
    SELECT s.id, s.name, COALESCE(s.description,''), s.created_at, s.user_id
    FROM strategies s
    WHERE s.id = $1
`, strategyID).Scan(&s.ID, &s.Name, &s.Description, &s.CreatedAt, &ownerID)
if err == pgx.ErrNoRows {
    writeError(w, http.StatusNotFound, "strategy not found")
    return
} else if err != nil {
    writeError(w, http.StatusInternalServerError, "database error")
    return
}
if ownerID != userID {
    writeError(w, http.StatusForbidden, "forbidden")
    return
}
```
Proceed to fetch versions only after ownership is confirmed.

#### C3. `jobs.go`, `strategies.go` — Unchecked QueryRow sites
All `QueryRow().Scan()` ownership-check sites (SubmitJob, GetPortfolio, CancelJob, DeleteStrategy):
```go
if err := db.Pool.QueryRow(...).Scan(&ownerID); err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        writeError(w, http.StatusNotFound, "resource not found")
    } else {
        writeError(w, http.StatusInternalServerError, "database error")
    }
    return
}
```

#### C4. `jobs.go` — ListJobs limit cap
```go
if err != nil || limit < 1 || limit > 200 {
    writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
    return
}
```

#### C5. `auth.go` — Refresh rate limit
Add after the IP extraction line in `Refresh`:
```go
if err := checkRateLimit(r.Context(), "ratelimit:refresh:"+clientIP(r), 20, time.Minute); err != nil {
    writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
    return
}
```

#### C6. `main.go` — Security headers middleware
```go
func securityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        next.ServeHTTP(w, r)
    })
}
```
Wrap the mux after CORS middleware. Note: CSP belongs on the HTML server (Nginx/Vite), not the JSON API.

#### C7. `middleware/jwt.go` — Generic error message
```go
writeError(w, http.StatusUnauthorized, "invalid or expired token")
```
(Remove the raw `err.Error()` interpolation.)

---

### D. httpOnly cookie auth

#### D1. `auth.go` — setRefreshCookie / clearRefreshCookie helpers
```go
func setRefreshCookie(w http.ResponseWriter, token string) {
    secure := os.Getenv("APP_ENV") == "production"
    http.SetCookie(w, &http.Cookie{
        Name:     "refresh_token",
        Value:    token,
        Path:     "/api/auth",
        MaxAge:   86400,
        HttpOnly: true,
        Secure:   secure,
        SameSite: http.SameSiteLaxMode,
    })
}

func clearRefreshCookie(w http.ResponseWriter) {
    http.SetCookie(w, &http.Cookie{
        Name:     "refresh_token",
        Value:    "",
        Path:     "/api/auth",
        MaxAge:   0,
        HttpOnly: true,
        SameSite: http.SameSiteLaxMode,
    })
}
```

**Login**: call `setRefreshCookie(w, refreshToken)`; response body: `{"accessToken": "...", "userId": "..."}` (no `refreshToken` field).

**Refresh**: read token from cookie:
```go
cookie, err := r.Cookie("refresh_token")
if err != nil {
    writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
    return
}
tokenHash := middleware.HashToken(cookie.Value)
```
After token rotation, call `setRefreshCookie(w, newRefreshToken)`; response body: `{"accessToken": "..."}`.

**Logout**: read token from cookie, DELETE from DB, call `clearRefreshCookie(w)`, return 204. No longer reads from JSON body.

#### D2. `models/models.go` — Update response types
- `AuthResponse`: remove `RefreshToken` field; keep `AccessToken`, `UserID`
- `RefreshRequest`: remove (no longer read from body)
- `LogoutRequest`: remove (no longer read from body)

#### D3. `store.ts` — Remove token persistence
- Remove `persist()` wrapper and `PersistOptions` import
- Keep `accessToken: string | null` in Zustand state (in-memory only)
- Remove `refreshToken` from state entirely
- Proactive refresh: `fetch('/api/auth/refresh', { method: 'POST', credentials: 'include' })` — empty body; server reads cookie

#### D4. `api.ts` — Cookie-aware fetch
- Add `credentials: 'include'` to all fetch calls that hit `/api` endpoints
- Keep `Authorization: Bearer ${accessToken}` header for authenticated REST calls (access token still in-memory state)
- Remove any manual refresh token body injection

#### D5. `vite.config.ts` — Dev proxy
```ts
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      }
    }
  }
})
```
This makes dev API calls same-origin so `SameSite=Lax` cookies work without HTTPS.

---

### E. WebSocket / streaming

#### E1. `stream.go` — JobStatusStream goroutine lifecycle
```go
ctx, cancel := context.WithCancel(r.Context())
defer cancel()
// ... ticker loop uses ctx for all DB calls ...
```
Add a disconnect-pump goroutine:
```go
go func() {
    conn.ReadMessage() // blocks until client closes
    cancel()
}()
```

#### E2. `stream.go` — PortfolioStream connection semaphore
Package-level:
```go
var wsSemaphore sync.Map  // key: userID, value: *int32 (atomic count)
const maxWSPerUser = 5
```
At connection start:
```go
counter, _ := wsSemaphore.LoadOrStore(userID, new(int32))
count := atomic.AddInt32(counter.(*int32), 1)
defer atomic.AddInt32(counter.(*int32), -1)
if count > maxWSPerUser {
    writeError(w, http.StatusTooManyRequests, "too many connections")
    return
}
```

#### E3. `stream.go` — HealthCheck
- Add Redis ping with 100ms timeout:
```go
redisClient := queue.GetClient()
pingCtx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
defer cancel()
if err := redisClient.Ping(pingCtx).Err(); err != nil {
    redisStatus = "error"
}
```
- If `redisStatus == "error"`: return HTTP 503 (K8s readiness probe interprets non-200 as not-ready)
- Public response body: `{"status":"ok"}` on success, `{"status":"error"}` on Redis/DB failure; remove `kafka`, `redis`, `db` fields from the public body

---

### F. Python worker reliability

#### F1. `celery_worker.py` — job_id UUID validation
```python
import re
_UUID_RE = re.compile(
    r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
    re.IGNORECASE
)

def _validate_job_id(conn, job_id: str) -> None:
    if not _UUID_RE.match(job_id):
        try:
            _update_job_status(conn, job_id, "failed", f"invalid job_id format: {job_id!r}")
        except Exception:
            pass
        raise ValueError(f"invalid job_id format: {job_id!r}")
```
Call at the top of both `run_lean_backtest_task` and `run_lean_live_task`.

#### F2. `celery_worker.py` — _create_lean_network timeout + error handling
```python
try:
    result = subprocess.run(
        ["docker", "network", "create", "lean-live-net", "--driver", "bridge"],
        capture_output=True, timeout=10
    )
    if result.returncode != 0 and b"already exists" not in result.stderr:
        logger.warning("docker network create failed: %s", result.stderr.decode())
except subprocess.TimeoutExpired:
    logger.warning("docker network create timed out")
```
Note: this code is vestigial after the K8s Job migration (no Docker socket); keep it guarded by `if os.path.exists("/var/run/docker.sock"):`.

#### F3. `celery_worker.py` — stop_lean_live in exception handler
See A2 above — already captures this in the outer `finally` restructure.

#### F4. `celery_worker.py` — LoggerAdapter default job_id
```python
class _DefaultJobIDFilter(logging.Filter):
    def filter(self, record):
        if not hasattr(record, 'job_id'):
            record.job_id = '-'
        return True

logging.getLogger().addFilter(_DefaultJobIDFilter())
```
Add at module level after `basicConfig`.

#### F5. `celery_worker.py` — _require_env helper
```python
def _require_env(name: str) -> str:
    val = os.environ.get(name)
    if not val:
        raise EnvironmentError(f"Required environment variable '{name}' is not set")
    return val

DATABASE_URL = _require_env("DATABASE_URL")
# ...
```

#### F6. `celery_worker.py` — Sanitize exception messages
```python
def _sanitize_error(msg: str) -> str:
    msg = re.sub(r'/tmp/atp-jobs/[^\s"]+', '<job_dir>', msg)
    msg = re.sub(r'/[a-zA-Z0-9_/.-]+\.py', '<path>', msg)
    return msg[:300]
```
Use `_sanitize_error(str(e))` in all `_update_job_status(..., "failed", ...)` calls.

---

### G. Strategy validator

#### G1. `strategy_validator.py` — Extend blocked lists
```python
BLOCKED_MODULES = {
    "os", "sys", "subprocess", "socket", "eval", "exec",
    "importlib", "importlib.util", "importlib.machinery",
    "ctypes", "builtins", "pickle", "marshal", "pty", "urllib",
}

BLOCKED_BUILTINS = {
    "eval", "exec", "compile", "__import__", "open", "breakpoint",
    "getattr", "setattr", "delattr",
}
```
Add new test cases for `import pty`, `import pickle`, `open("x")`, and `breakpoint()`.

---

### H. K8s architecture — Docker socket removal

#### H1. `lean_runner.py` — Rewrite with K8s client

New module-level constants:
```python
from kubernetes import client, config as k8s_config, watch

LEAN_IMAGE = os.environ.get("LEAN_IMAGE", "lean-atp:latest")
KAFKA_NODE_IP = os.environ["KAFKA_NODE_IP"]  # kept for backwards-compat env var; unused post-K8s
NAMESPACE = os.environ.get("K8S_NAMESPACE", "default")
S3_BUCKET = os.environ["S3_BUCKET"]
S3_ACCESS_KEY = os.environ["S3_ACCESS_KEY"]
S3_SECRET_KEY = os.environ["S3_SECRET_KEY"]

try:
    k8s_config.load_incluster_config()  # inside K8s pod
except k8s_config.ConfigException:
    k8s_config.load_kube_config()  # local dev
```

**`upload_job_inputs(job_id, job_dir)`**: Upload `job_dir/` tree to `s3://S3_BUCKET/jobs/{job_id}/input/` using boto3. Called from `celery_worker.py` before `run_lean_backtest`/`run_lean_live`.

**`download_job_results(job_id, dest_dir)`**: Download `s3://S3_BUCKET/jobs/{job_id}/results/` to `dest_dir`. Called after K8s Job completion.

**`cleanup_job_s3(job_id)`**: Delete all objects under `s3://S3_BUCKET/jobs/{job_id}/`. Called in task `finally` block.

**`_build_lean_job_spec(job_name, job_id, job_type)`** → returns K8s Job dict:
- `terminationGracePeriodSeconds: 120`
- `nodeSelector: {dedicated: lean-worker}`
- `tolerations: [{key: dedicated, value: lean-worker, effect: NoSchedule}]`
- `ttlSecondsAfterFinished: 3600`
- `backoffLimit: 0`
- `restartPolicy: Never`
- Container env: `JOB_ID`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `KAFKA_BOOTSTRAP_SERVERS=kafka:9092` (live only)
- Resources: requests `{memory: 1Gi, cpu: "1"}`, limits `{memory: 3Gi, cpu: "2"}`
- Security context: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`

**`run_lean_backtest(job_id, job_dir, timeout_seconds)`**:
1. Upload inputs via `upload_job_inputs(job_id, job_dir)`
2. Create K8s Job via `BatchV1Api.create_namespaced_job`
3. Wait for completion using `watch.Watch()` on the Job, with `timeout_seconds`
4. On timeout: delete Job, raise `TimeoutError`
5. On failure (pod failed): delete Job, raise `RuntimeError` with pod log excerpt
6. Download results via `download_job_results(job_id, local_results_dir)`
7. Return local results path

**`run_lean_live(job_id, job_dir)`**: Creates K8s Job, returns `job_name` (string). Does NOT upload inputs (caller does that). Wait for pod to enter `Running` phase (max 120 seconds).

**`stop_lean_live(job_name, job_dir)`**: Delete K8s Job via `BatchV1Api.delete_namespaced_job(propagation_policy="Foreground")`; wait for pod deletion (max 60 seconds); call `download_job_results(job_id_from_name, job_dir)`.

**`is_container_running(job_name)`**: List pods by label `job_id=<id>`, return True if any pod is in `Running` phase.

**`poll_live_results(job_id)`**: Download partial results from S3 if `results/` prefix exists; return parsed JSON or None.

#### H2. `lean-plugin/Dockerfile` — Wrapper script

Add to Dockerfile:
```dockerfile
RUN apt-get update && apt-get install -y --no-install-recommends awscli && rm -rf /var/lib/apt/lists/*

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
```

`lean-plugin/entrypoint.sh`:
```bash
#!/bin/bash
set -e

# Download job inputs from S3
aws s3 sync "s3://${S3_BUCKET}/jobs/${JOB_ID}/input/" /lean/

_shutdown() {
    echo "Shutdown signal received, stopping LEAN..."
    kill -TERM "$LEAN_PID" 2>/dev/null
    wait "$LEAN_PID" 2>/dev/null
    echo "Uploading results..."
    aws s3 sync /lean/Results/ "s3://${S3_BUCKET}/jobs/${JOB_ID}/results/" || true
    exit 0
}

trap '_shutdown' SIGTERM SIGINT

# Run LEAN in background so the trap can fire
/Lean/Launcher/bin/Debug/Lean.Launcher &
LEAN_PID=$!
wait "$LEAN_PID"
EXIT_CODE=$?

# Normal exit path: upload results
echo "Uploading results..."
aws s3 sync /lean/Results/ "s3://${S3_BUCKET}/jobs/${JOB_ID}/results/" || true

exit $EXIT_CODE
```

AWS credentials injected via K8s Job env vars (from `atp-core-credentials` Sealed Secret).

#### H3. K8s manifests

**`kubernetes/celery/serviceaccount.yaml`**:
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: lean-job-runner
  namespace: default
```

**`kubernetes/celery/rbac.yaml`**:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: lean-job-runner
  namespace: default
rules:
- apiGroups: ["batch"]
  resources: ["jobs"]
  verbs: ["create", "get", "list", "delete", "watch"]
- apiGroups: [""]
  resources: ["pods", "pods/log"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: lean-job-runner
  namespace: default
subjects:
- kind: ServiceAccount
  name: lean-job-runner
  namespace: default
roleRef:
  kind: Role
  name: lean-job-runner
  apiGroup: rbac.authorization.k8s.io
```

**`celery-worker-deployment.yaml`** changes:
- Remove `volumeMounts` and `volumes` entries for `docker-sock`
- Add `serviceAccountName: lean-job-runner`
- Remove `LEAN_KAFKA_BOOTSTRAP_SERVERS` (LEAN container now receives it directly in Job spec)
- Add `K8S_NAMESPACE: default` env var

#### H4. `kops.yaml` — lean-nodes InstanceGroup

Add new InstanceGroup section:
```yaml
---
apiVersion: kops.k8s.io/v1alpha2
kind: InstanceGroup
metadata:
  name: lean-nodes
  labels:
    kops.k8s.io/cluster: <cluster-fqdn>
spec:
  role: Node
  machineType: t3.medium
  minSize: 0
  maxSize: 3
  taints:
  - dedicated=lean-worker:NoSchedule
  nodeLabels:
    node-role.kubernetes.io/node: ""
    dedicated: lean-worker
  subnets:
  - us-east-1a
```

---

### I. K8s security

#### I1. `go-app/deployment.yaml` — securityContext
```yaml
securityContext:
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
```

Note: if the Go binary writes to disk (e.g., log files), mount an emptyDir at the log path or verify the binary only logs to stdout.

#### I2. `celery-worker-deployment.yaml` — securityContext (after Docker socket removal)
```yaml
securityContext:
  runAsNonRoot: true
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
```
`readOnlyRootFilesystem: true` is NOT set because the Celery worker writes to `/tmp/atp-jobs/` for staging. Add `emptyDir` volume at `/tmp/atp-jobs` if needed.

#### I3. LEAN_KAFKA_BOOTSTRAP_SERVERS env var
Remove from `celery-worker-deployment.yaml` (Celery worker no longer needs it). The LEAN K8s Job container receives `KAFKA_BOOTSTRAP_SERVERS=kafka:9092` directly in the Job spec (env var injected from `lean_runner.py`, not from a manifest). The K8s Service `kafka` on port 9092 already exists.

---

### J. Test quality

#### J1. `conftest.py` — _count whitelist
```python
_ALLOWED_TABLES = frozenset({"performance_metrics", "portfolio_metrics", "job_logs"})

def _count(dsn, table, job_id):
    if table not in _ALLOWED_TABLES:
        raise ValueError(f"Unknown table: {table!r}")
    conn = psycopg2.connect(dsn)
    with conn.cursor() as cur:
        cur.execute(f"SELECT COUNT(*) FROM {table} WHERE job_id = %s", (job_id,))
        return cur.fetchone()[0]
    conn.close()
```

#### J2. `test_lean_runner.py` — Fix fragile kill-loop test
Replace `side_effect=CalledProcessError` with `returncode`-based mocking; assert on the logged `error` call rather than on implicit exception propagation.

#### J3. `test_celery_worker.py` — Fix test_lean_runtime_error assertion
Assert that `job["error_message"]` contains the exact string raised by the mock (whatever `str(MockError(...))` produces), not a hardcoded literal.

---

## Out of Scope

- WebSocket: one Kafka consumer per connection (personal research platform; acceptable until multi-tenant)
- Refresh token in localStorage: already addressed by cookie migration in this wave
- Security response headers for the React SPA HTML (belong on Nginx/CDN, not the Go API)
- Kafka KRaft migration (Kafka manifest still uses Zookeeper; separate concern)
- Celery worker concurrency / live-vs-backtest queue separation (live task holds worker slot; known limitation)
- S3 artifact size limits for large backtests (multi-year tick data may be very large; out of scope for Wave 4)
- gVisor / Kata Containers runtime class for LEAN pods (additional isolation layer; Wave 5)
- `go-data` service changes
- LEAN result data quality gaps (5 nullable columns in `performance_metrics` always NULL)

---

## Assumptions

- The `lean-atp:latest` Docker image build pipeline is accessible (CI/CD or manual `docker build`) to incorporate the wrapper script changes in Wave 4
- `awscli` can be installed into the `lean-atp` Debian/Ubuntu-based image without conflicting with the LEAN runtime
- The `lean-nodes` InstanceGroup in kops.yaml will be applied via `kops update cluster --yes` before LEAN K8s Jobs are submitted; until then backtests will be pending
- The `atp-core-credentials` Sealed Secret already contains `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET` with the values needed for LEAN container S3 I/O
- In local dev, `kubernetes` Python client will use `~/.kube/config` (kops-generated); test isolation uses `unittest.mock.patch`
- `readOnlyRootFilesystem: true` on `go-app` is safe because the Go service logs to stdout (verify before deploying)
- The Go binary in `go-app` produces no files in the container root filesystem at runtime

## Open Questions

None — all decisions resolved during planning.
