# Plan: ATP Wave 5 — Review Blocker Resolution
> Generated: 2026-05-29
> Type: brownfield
> Documents: single file
> Archived: [PLANv4.md](archive/PLANv4.md) (Wave 4 — K8s Jobs migration, httpOnly cookies, WebSocket reliability, Python worker hardening)

## Overview
**What:** Fix all 9 blockers and 12 key warnings identified by the Wave 4 post-implementation code review. No new features; no schema changes. Every change is a targeted correction to code already merged on `switch-to-lean`.

**Why:** The Wave 4 implementation passes tests but has concrete security vulnerabilities (S3 creds in plaintext K8s Job specs, rate limit bypass via XFF header, Dockerfile running as root, strategy validator missing sandbox escape primitives), correctness bugs (live pod startup failure is silent, DB connection leaks, `CancelJob` drops errors), and infrastructure gaps (`APP_ENV=production` missing so the `Secure` cookie flag never sets in the cluster).

**Who:** Internal — no user-visible behavior changes.

---

## Definition of Done

### A. Security Blockers

- [ ] `kubectl get job <lean-job> -o yaml` on any LEAN backtest or live job shows `S3_ACCESS_KEY` and `S3_SECRET_KEY` with `valueFrom.secretKeyRef` (no `value:` field); `grep "\"value\".*ACCESS\|\"value\".*SECRET" python/lean_runner.py` → 0 matches
- [ ] Sending `X-Forwarded-For: 1.2.3.4` to `POST /api/auth/login` 11 times from the same TCP peer triggers 429 on the 11th call; `grep "X-Forwarded-For" go-app/handlers/auth.go` → 0 lines; `go test ./go-app/handlers/ -run TestClientIP` passes
- [ ] `kubernetes/go-app/deployment.yaml` env block contains `- name: APP_ENV` / `value: "production"`; go-app pod environment has `APP_ENV=production`; `POST /api/auth/login` response cookie includes `Secure` attribute
- [ ] `docker build -f lean-plugin/Dockerfile lean-plugin/ -t lean-test && docker run --rm lean-test whoami` prints `lean` (non-root uid 1001)
- [ ] `validate_strategy("().__class__.__subclasses__()")` → `{"valid": false, ...}`; `validate_strategy("import requests")` → `{"valid": false, ...}`; `validate_strategy("import threading")` → `{"valid": false, ...}`; `validate_strategy("vars()['__builtins__']")` → `{"valid": false, ...}`; `python3 -m pytest python/test_strategy_validator.py -v` passes including new test cases

### B. Correctness Blockers

- [ ] With `list_namespaced_pod` mocked to always return phase `Pending`: `run_lean_live` raises `RuntimeError` containing "never reached Running"; `delete_namespaced_job` is called before the raise; `python3 -m pytest python/test_lean_runner.py -k test_live_pod_startup_timeout -v` passes
- [ ] `run_lean_backtest_task("not-a-uuid")`: raises `ValueError("invalid job_id format")` with `_get_db` never called (verified by `patch("celery_worker._get_db", side_effect=AssertionError)` — AssertionError is NOT raised); `python3 -m pytest python/test_celery_worker.py -k test_invalid_uuid_no_db_call -v` passes
- [ ] `POST /api/jobs/<queued-job-id>/cancel` → 202; subsequent `GET /api/jobs/<id>` → `status: "failed"`, `error_message: "cancelled by user"`
- [ ] `POST /api/jobs/<running-job-id>/cancel` when Redis is unreachable → 500 `{"error":"failed to cancel job"}`; server log contains "SetStopSignal error"
- [ ] `go test ./go-app/handlers/ -run TestCancelJob` passes

### C. Go API Corrections

- [ ] `GET /api/jobs/:id` with a simulated DB error → 500 (not 404); `go test ./go-app/handlers/ -run TestGetJob_DBError` passes
- [ ] `grep "== pgx.ErrNoRows" go-app/handlers/strategies.go` → 0 lines; all 5 ErrNoRows checks use `errors.Is`
- [ ] Stats query failure in `GetStrategy` produces a server log entry containing "stats query error"; `go test ./go-app/handlers/ -run TestGetStrategy_StatsError` passes
- [ ] Redis failure in `checkRateLimit` produces a `log.Printf` call; `go test ./go-app/... -run TestRateLimit_RedisError` passes
- [ ] `go test ./go-app/...` passes

### D. Infrastructure

- [ ] `grep "value: redis://" kubernetes/celery/celery-worker-deployment.yaml` → 0 lines; `CELERY_BROKER_URL` env uses `secretKeyRef.key: REDIS_URL` from `atp-core-credentials`
- [ ] `grep "KAFKA_NODE_IP" python/lean_runner.py` → 0 lines; `python3 -c "import lean_runner"` succeeds without `KAFKA_NODE_IP` in env; `KAFKA_NODE_IP` env var absent from `celery-worker-deployment.yaml`
- [ ] `poll_live_results` uses `get_paginator("list_objects_v2")`; `grep "list_objects_v2" python/lean_runner.py` → 1 line (no double-call); `python3 -m pytest python/test_lean_runner.py -k test_poll_live_results -v` passes

### E. Code Quality

- [ ] `python -m ruff check python/celery_worker.py` → 0 F401 findings for `tempfile` and `pathlib`
- [ ] All module-level attribute mutations in `test_celery_worker.py` replaced with `monkeypatch.setattr`
- [ ] `grep -E "^moto|^testcontainers" python/requirements.txt` → 2 matching lines; `pip install -r python/requirements.txt` exits 0
- [ ] `python -m black --check python/` exits 0
- [ ] `python3 -m pytest python/ -v` passes all tests

---

## Unchanged Behavior

- WHEN a user submits a valid backtest job THEN the response SHALL continue to be `202 Accepted` with `{"jobId": "<uuid>"}`
- WHEN a backtest completes THEN `performance_metrics` and `portfolio_metrics` rows SHALL continue to be inserted
- WHEN a user uploads a valid strategy THEN the response SHALL continue to return `201 Created`
- WHEN a WebSocket client sends a valid first-message auth token THEN streaming SHALL continue to work
- WHEN any REST endpoint receives `Authorization: Bearer <JWT>` THEN it SHALL continue to accept it
- WHEN `POST /api/auth/login` succeeds THEN the response SHALL continue to set `refresh_token` as an `HttpOnly; SameSite=Lax` cookie (SameSite remains Lax — POST endpoints are equally CSRF-safe under Lax and Strict)
- WHEN a live task is running and no stop signal is set THEN the monitoring loop SHALL continue indefinitely (no wall-clock timeout added — live strategies are intentionally unbounded)

---

## Fixes by Area

### A. Security Blockers

#### A1. `lean_runner.py` — S3 credentials via secretKeyRef

In `_build_lean_job_spec`, replace the two literal env entries with secret references. The `atp-core-credentials` secret already has `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET` keys (verified from `celery-worker-deployment.yaml:27-41`).

```python
# Before (exposes values in kubectl get job -o yaml):
{"name": "S3_ACCESS_KEY", "value": S3_ACCESS_KEY},
{"name": "S3_SECRET_KEY", "value": S3_SECRET_KEY},

# After:
{"name": "S3_ACCESS_KEY", "valueFrom": {"secretKeyRef": {"name": "atp-core-credentials", "key": "S3_ACCESS_KEY"}}},
{"name": "S3_SECRET_KEY", "valueFrom": {"secretKeyRef": {"name": "atp-core-credentials", "key": "S3_SECRET_KEY"}}},
```

The module-level `S3_ACCESS_KEY = os.environ["S3_ACCESS_KEY"]` reads remain — the Celery worker still needs them for its own boto3 client (`_get_s3()`). Only the Job spec injection changes.

#### A2. `auth.go` — Remove XFF trust; use RemoteAddr only

`clientIP()` currently takes the rightmost `X-Forwarded-For` entry, which any client can set. Inside Kubernetes, `r.RemoteAddr` is the actual TCP peer and cannot be spoofed.

```go
// Before:
func clientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        parts := strings.Split(xff, ",")
        return strings.TrimSpace(parts[len(parts)-1])
    }
    ip, _, _ := net.SplitHostPort(r.RemoteAddr)
    if ip == "" {
        return r.RemoteAddr
    }
    return ip
}

// After:
func clientIP(r *http.Request) string {
    ip, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr
    }
    return ip
}
```

Remove `strings` import if no longer used elsewhere. Update `auth_test.go` to remove any test that asserts XFF-based IP extraction.

#### A3. `kubernetes/go-app/deployment.yaml` — Add APP_ENV=production

```yaml
- name: APP_ENV
  value: "production"
```

Add to the `go-app` container's env block. This activates the `Secure` attribute on the `refresh_token` cookie in `auth.go:74`.

#### A4. `lean-plugin/Dockerfile` — Add non-root USER

The `quantconnect/lean` base image runs as root. The K8s Job spec already has `runAsNonRoot: true`, which causes pod admission failure without this fix.

Add after the awscli install and entrypoint setup:
```dockerfile
RUN groupadd --gid 1001 lean && \
    useradd --uid 1001 --gid 1001 --no-create-home --shell /bin/false lean && \
    mkdir -p /lean/Results && \
    chown -R lean:lean /lean /Lean/Launcher
USER lean
```

The `chown` on `/lean/` is required because `entrypoint.sh` does `aws s3 sync .../input/ /lean/` and LEAN writes results to `/lean/Results/`. The `chown` on `/Lean/Launcher` is required because LEAN writes log files to its working directory.

Note: verify `docker run --rm lean-test /Lean/Launcher/bin/Debug/Lean.Launcher --version` exits cleanly as uid=1001 before merging. If LEAN writes to other paths at runtime, add them to the `chown` list.

#### A5. `strategy_validator.py` — Extend blocklists

Add to `BLOCKED_ATTRS`:
```python
BLOCKED_ATTRS = {
    "__import__", "__builtins__", "__loader__",
    # sandbox escape via dunder chain: ().__class__.__subclasses__() or func.__globals__
    "__class__", "__subclasses__", "__globals__", "__dict__", "__mro__",
}
```

Add to `BLOCKED_MODULES`:
```python
BLOCKED_MODULES = {
    # existing ...
    "requests", "urllib3", "http", "http.client",
    "ftplib", "smtplib", "threading", "multiprocessing",
    "concurrent", "asyncio",
}
```

Add new `BLOCKED_BUILTINS` set and check for `ast.Call` nodes where the function is one of:
```python
BLOCKED_BUILTINS = {"vars", "globals", "locals", "dir"}
```

Add new test cases to `test_strategy_validator.py`:
- `().__class__.__subclasses__()` → blocked (BLOCKED_ATTRS: `__class__`)
- `vars()['__builtins__']` → blocked (BLOCKED_BUILTINS: `vars`)
- `import requests` → blocked
- `import threading` → blocked

---

### B. Correctness Blockers

#### B1. `lean_runner.py` — run_lean_live startup failure

Replace the open while loop with a while/else:

```python
deadline = time.time() + 120
while time.time() < deadline:
    pods = k8s_client.CoreV1Api().list_namespaced_pod(
        NAMESPACE, label_selector=f"job-name={job_name}"
    )
    if pods.items and pods.items[0].status.phase == "Running":
        logger.info(f"Live job {job_name} pod is Running")
        break
    time.sleep(5)
else:
    batch_api.delete_namespaced_job(
        job_name, NAMESPACE,
        body=k8s_client.V1DeleteOptions(propagation_policy="Foreground"),
    )
    raise RuntimeError(
        f"LEAN live pod for job {job_id} never reached Running within 120s"
    )
return job_name
```

Add new test `test_live_pod_startup_timeout` to `test_lean_runner.py`: mock `list_namespaced_pod` to always return `Pending`; assert `RuntimeError` raised and `delete_namespaced_job` called.

#### B2. `celery_worker.py` — UUID validation before DB connection

Move the UUID format check to the top of both task functions, before `_get_db()`:

```python
@app.task(name="atp.run_lean_backtest", bind=True)
def run_lean_backtest_task(self, job_id: str):
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    log = _logger(job_id)
    conn = _get_db()
    try:
        ...
    except Exception as e:
        _update_job_status(conn, job_id, "failed", _sanitize_error(str(e)))
        ...
    finally:
        conn.close()
```

Remove the `_validate_job_id(conn, job_id)` call from both tasks (the in-function call is now redundant; the module-level `_validate_job_id` function can remain for any external callers, but the tasks no longer call it). The `_UUID_RE` pattern already exists at module level.

Add new test `test_invalid_uuid_no_db_call` that patches `_get_db` with `side_effect=AssertionError` and confirms that `run_lean_backtest_task("bad")` raises `ValueError` (not `AssertionError`).

#### B3. `jobs.go` — CancelJob: fix dropped error + add queued cancellation

```go
// Accept both running and queued:
if status != "running" && status != "queued" {
    writeError(w, http.StatusBadRequest, "job is not running or queued")
    return
}

if status == "queued" {
    // Mark as failed immediately; Celery task pre-flight check handles the race
    _, err := db.Pool.Exec(r.Context(), `
        UPDATE jobs SET status='failed', error_message='cancelled by user',
        completed_at=NOW() WHERE id=$1
    `, jobID)
    if err != nil {
        log.Printf("CancelJob: DB update error for queued job %s: %v", jobID, err)
        writeError(w, http.StatusInternalServerError, "database error")
        return
    }
} else {
    // running: signal stop
    if err := queue.SetStopSignal(jobID); err != nil {
        log.Printf("CancelJob: SetStopSignal error for job %s: %v", jobID, err)
        writeError(w, http.StatusInternalServerError, "failed to cancel job")
        return
    }
}
writeJSON(w, http.StatusAccepted, map[string]string{
    "jobId":  jobID,
    "status": "cancelling",
})
```

Add pre-flight status check at the top of both Celery task functions (after `_fetch_job`):
```python
job = _fetch_job(conn, job_id)
if job["status"] != "queued":
    log.info("Job %s is in status %s, skipping (likely cancelled)", job_id, job["status"])
    return
```

This means: if a `queued` job is cancelled between enqueueing and pickup, the task fetches the job, sees `failed`, and exits before doing any work.

---

### C. Go API Corrections

#### C1. `jobs.go` — GetJob: distinguish 404 from 500

```go
// Before:
if err != nil {
    writeError(w, http.StatusNotFound, "job not found")
    return
}

// After:
if err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        writeError(w, http.StatusNotFound, "job not found")
    } else {
        writeError(w, http.StatusInternalServerError, "database error")
    }
    return
}
```

Add test `TestGetJob_DBError` that injects a DB error and asserts 500.

#### C2. `stream.go` — rows.Close() before early return

The `return` on WebSocket write error at line 169 currently skips `rows.Close()` at line 175. Fix: add `rows.Close()` immediately before the `return`:

```go
if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
    rows.Close()  // prevent resource leak on client disconnect
    return
}
```

The existing `rows.Close()` at line 175 (after the for loop) is kept as-is for the normal exit path.

#### C3. `strategies.go` — Standardize ErrNoRows checks

Replace all 4 direct equality comparisons with `errors.Is`:
```go
// Before (4 sites: lines 202, 265, 348, 364):
if err == pgx.ErrNoRows {

// After:
if errors.Is(err, pgx.ErrNoRows) {
```

#### C4. `strategies.go` — Log stats query error

```go
var runCount int
var bestSharpe, avgReturn *float64
if err := db.Pool.QueryRow(...).Scan(&runCount, &bestSharpe, &avgReturn); err != nil {
    log.Printf("GetStrategy: stats query error for strategy %s: %v", strategyID, err)
    // continue — partial response is acceptable
}
```

#### C5. `auth.go` — Log rate limiter fail-open

```go
// In checkRateLimit, before `return nil`:
if err != nil {
    log.Printf("rate limit check failed (failing open) for key %s: %v", key, err)
    return nil
}
```

---

### D. Infrastructure

#### D1. `celery-worker-deployment.yaml` — CELERY_BROKER_URL via secret

```yaml
# Before:
- name: CELERY_BROKER_URL
  value: "redis://redis-service:6379/0"

# After:
- name: CELERY_BROKER_URL
  valueFrom:
    secretKeyRef:
      name: atp-core-credentials
      key: REDIS_URL
```

The `atp-core-credentials` secret already contains `REDIS_URL` (confirmed from `celery-worker-deployment.yaml:42-46`). Format is compatible with Celery's broker URL.

#### D2. `lean_runner.py` + `celery-worker-deployment.yaml` — Remove unused KAFKA_NODE_IP

- `lean_runner.py`: Remove line `KAFKA_NODE_IP = os.environ["KAFKA_NODE_IP"]`. The variable is never referenced in any function body; the K8s Job spec uses the hardcoded string `"kafka:9092"`.
- `celery-worker-deployment.yaml`: Remove the `KAFKA_NODE_IP` env var block (lines 47-51). The secret key can remain in `atp-core-credentials`.

#### D3. `lean_runner.py` — poll_live_results: single paginated call

```python
def poll_live_results(job_id: str) -> Optional[dict]:
    s3 = _get_s3()
    prefix = f"jobs/{job_id}/results/"
    paginator = s3.get_paginator("list_objects_v2")
    json_keys = []
    for page in paginator.paginate(Bucket=S3_BUCKET, Prefix=prefix):
        json_keys.extend(
            obj["Key"] for obj in page.get("Contents", [])
            if obj["Key"].endswith(".json")
        )
    if not json_keys:
        return None
    latest_key = sorted(json_keys)[-1]
    buf = io.BytesIO()
    s3.download_fileobj(S3_BUCKET, latest_key, buf)
    buf.seek(0)
    try:
        return json.loads(buf.read())
    except Exception:
        return None
```

Eliminates the double `list_objects_v2` call (existence check + full list) and the 1000-object truncation.

---

### E. Code Quality

#### E1. `celery_worker.py` — Remove unused imports

Remove `import tempfile` and `from pathlib import Path` (ruff F401). Both were left from a prior refactor.

#### E2. `test_celery_worker.py` — Replace direct attribute mutation with monkeypatch

Replace all `celery_worker.X = value` direct assignments in `_run_task` with `monkeypatch.setattr(celery_worker, "X", value)`. This prevents module-level state from persisting across tests on partial failure.

#### E3. `python/requirements.txt` — Add test dependencies

```
moto[s3]>=5.0
testcontainers>=4.0
```

These are imported by `test_celery_worker.py` but missing from the requirements file, making the test suite uninstallable in CI.

#### E4. Black formatting

Run `python -m black python/` across all 6 Python files. No logic changes.

---

## Out of Scope

- `SameSite=Strict` on refresh cookie — Wave 4 explicitly chose Lax; for POST `/api/auth/refresh`, Lax and Strict are equally CSRF-safe. No change.
- `K8S_NAMESPACE=atp-jobs` namespace isolation for LEAN pods — real security improvement, significant RBAC/network-policy complexity. Wave 6.
- `Content-Security-Policy` header — already Wave 4 Out of Scope (belongs on Nginx/CDN, not the JSON API server).
- `while True` live monitoring loop timeout — intentional; live strategies run indefinitely during market hours. `is_container_running` handles pod exits; stop signal handles manual cancel.
- Python type annotations — code quality, no correctness impact, Wave 6.
- f-strings in logging calls — best practice, not blocking, Wave 6.
- gVisor / Kata Containers runtime class for LEAN pods — Wave 6.
- `go-data` service changes.

---

## Assumptions

- `atp-core-credentials` Sealed Secret already contains keys: `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `REDIS_URL` — confirmed from existing deployment manifests.
- The LEAN launcher writes only to `/lean/` and `/Lean/Launcher/` at runtime. If LEAN writes to other paths (e.g., `/tmp`), the `chown` list in the Dockerfile must be extended.
- Redis has no password currently (both services use `redis://redis-service:6379/0`). If a password is added later, updating `REDIS_URL` in the secret propagates to both go-app and celery-worker automatically.
- `queue.SetStopSignal` returns an `error` type. If the current signature is `func SetStopSignal(jobID string)` (no return), update the signature first before adding the error check in `CancelJob`.

## Open Questions

None — all decisions resolved during planning.
