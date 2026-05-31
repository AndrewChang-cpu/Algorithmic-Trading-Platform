# Tasks: ATP Wave 5 — Review Blocker Resolution
> Generated: 2026-05-29
> Source: .plan/
> Total: 15 tasks | Starting points: 10

## Dependency Graph

```
T-01 · lean_runner.py — S3 secretKeyRef
└── T-06 · lean_runner.py — run_lean_live startup failure + test
    └── T-13 · lean_runner.py — KAFKA_NODE_IP removal + poll paginator + test
        └── T-15* · Black formatting (all Python files)

T-02 · auth.go — Remove XFF + rate limiter logging

T-03 · deployment.yaml — APP_ENV=production

T-04 · Dockerfile — Non-root USER directive

T-05 · strategy_validator.py — Extend blocklists + tests
    └── T-15*

T-07 · celery_worker.py — UUID validation before _get_db()
└── T-09 · celery_worker.py — Pre-flight status check + remove unused imports
    └── T-14 · test_celery_worker.py monkeypatch + requirements.txt test deps
        └── T-15*

T-08 · jobs.go — CancelJob queued support + GetJob 404/500 split

T-10 · stream.go — rows.Close() before early return

T-11 · strategies.go — errors.Is + stats query logging

T-12 · celery-worker-deployment.yaml — CELERY_BROKER_URL secret + KAFKA_NODE_IP removal
```

`* T-15 also depends on T-05, T-13, T-14`

---

## Tasks

### T-01 · lean_runner.py — S3 secretKeyRef in Job spec
**Status:** `reviewed`
**Depends on:** none
**Files:** `python/lean_runner.py`
**What:** In `_build_lean_job_spec`, replace the two literal env entries:
```python
{"name": "S3_ACCESS_KEY", "value": S3_ACCESS_KEY},
{"name": "S3_SECRET_KEY", "value": S3_SECRET_KEY},
```
with secretKeyRef references:
```python
{"name": "S3_ACCESS_KEY", "valueFrom": {"secretKeyRef": {"name": "atp-core-credentials", "key": "S3_ACCESS_KEY"}}},
{"name": "S3_SECRET_KEY", "valueFrom": {"secretKeyRef": {"name": "atp-core-credentials", "key": "S3_SECRET_KEY"}}},
```
The module-level `S3_ACCESS_KEY = os.environ["S3_ACCESS_KEY"]` reads at lines 27–28 remain unchanged — the Celery worker's `_get_s3()` still needs them for its own boto3 client. Only the Job spec dict entries change.
**Done when:** `grep '"value".*ACCESS\|"value".*SECRET' python/lean_runner.py` → 0 matches; both env entries in `_build_lean_job_spec` use `"valueFrom"` with `"secretKeyRef"`

---

### T-02 · auth.go — Remove XFF trust + rate limiter fail-open logging
**Status:** `reviewed`
**Depends on:** none
**Files:** `go-app/handlers/auth.go`, `go-app/handlers/auth_test.go`
**What:** Two changes to `auth.go`:

1. **`clientIP()` — remove XFF branch.** The current implementation takes the rightmost `X-Forwarded-For` entry, which any client can set to bypass IP-keyed rate limits. Replace with RemoteAddr-only:
```go
func clientIP(r *http.Request) string {
    ip, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr
    }
    return ip
}
```
Remove the `strings` import if it is no longer used elsewhere in the file.

2. **`checkRateLimit()` — log Redis failure before failing open.** Add a `log.Printf` before the `return nil` on the Redis error path:
```go
if err != nil {
    log.Printf("rate limit check failed (failing open) for key %s: %v", key, err)
    return nil
}
```

Update `auth_test.go` to remove any test that asserts XFF-based IP extraction from `clientIP`. Add a test `TestClientIP_UsesRemoteAddr` that confirms `clientIP` returns the host portion of `RemoteAddr` regardless of any `X-Forwarded-For` header value.
**Done when:** `grep "X-Forwarded-For" go-app/handlers/auth.go` → 0 lines; `grep "log.Printf.*failing open" go-app/handlers/auth.go` → 1 match; `go test ./go-app/handlers/ -run TestClientIP -v` passes

---

### T-03 · kubernetes/go-app/deployment.yaml — Add APP_ENV=production
**Status:** `reviewed`
**Depends on:** none
**Files:** `kubernetes/go-app/deployment.yaml`
**What:** Add the following env entry to the `go-app` container's `env` block in `kubernetes/go-app/deployment.yaml`:
```yaml
- name: APP_ENV
  value: "production"
```
This activates the `Secure` attribute on the `refresh_token` cookie in `auth.go:74` (`if os.Getenv("APP_ENV") == "production"`), which is currently missing in the cluster and causes the cookie to be issued without `Secure`.
**Done when:** `grep -A1 "APP_ENV" kubernetes/go-app/deployment.yaml` shows `name: APP_ENV` followed by `value: "production"`

---

### T-04 · lean-plugin/Dockerfile — Add non-root USER directive
**Status:** `reviewed`
**Depends on:** none
**Files:** `lean-plugin/Dockerfile`
**What:** The `quantconnect/lean` base image runs as root. The K8s Job spec already sets `runAsNonRoot: true`, which causes pod admission failure without a non-root user in the image. Add after the `RUN chmod +x /entrypoint.sh` line:
```dockerfile
RUN groupadd --gid 1001 lean && \
    useradd --uid 1001 --gid 1001 --no-create-home --shell /bin/false lean && \
    mkdir -p /lean/Results && \
    chown -R lean:lean /lean /Lean/Launcher
USER lean
```
The `chown /lean` is required because `entrypoint.sh` runs `aws s3 sync .../input/ /lean/` and LEAN writes results to `/lean/Results/`. The `chown /Lean/Launcher` is required because LEAN writes log files to its working directory.
**Done when:** `docker build -f lean-plugin/Dockerfile lean-plugin/ -t lean-test && docker run --rm lean-test whoami` prints `lean` (exits 0)

---

### T-05 · strategy_validator.py — Extend blocklists + new tests
**Status:** `reviewed`
**Depends on:** none
**Files:** `python/strategy_validator.py`, `python/test_strategy_validator.py`
**What:** Extend the three blocklists in `strategy_validator.py` to cover sandbox escape primitives and network modules that are currently unblocked.

**`BLOCKED_ATTRS`** — add dunder-chain escape attributes:
```python
BLOCKED_ATTRS = {
    "__import__", "__builtins__", "__loader__",
    "__class__", "__subclasses__", "__globals__", "__dict__", "__mro__",
}
```

**`BLOCKED_MODULES`** — add network and concurrency modules:
```python
# append to existing set:
"requests", "urllib3", "http", "http.client",
"ftplib", "smtplib", "threading", "multiprocessing", "concurrent", "asyncio",
```

**`BLOCKED_BUILTINS`** — add introspection builtins (already checked at `ast.Call` nodes with `ast.Name` func id):
```python
# append to existing set:
"vars", "globals", "locals", "dir",
```

Add new test cases to `test_strategy_validator.py` (one test per new vector):
- `test_blocked_dunder_class`: source `"().__class__.__subclasses__()"` → `valid == False`, violation mentions `__class__`
- `test_blocked_dunder_globals`: source `"def f(): return f.__globals__"` → `valid == False`, violation mentions `__globals__`
- `test_blocked_builtin_vars`: source `"x = vars()"` → `valid == False`, violation mentions `vars`
- `test_blocked_module_requests`: source `"import requests"` → `valid == False`
- `test_blocked_module_threading`: source `"import threading"` → `valid == False`
**Done when:** `python3 -m pytest python/test_strategy_validator.py -v` passes, including all five new test cases listed above

---

### T-06 · lean_runner.py — run_lean_live startup failure + test
**Status:** `reviewed`
**Depends on:** T-01
**Files:** `python/lean_runner.py`, `python/test_lean_runner.py`
**What:** In `run_lean_live`, convert the open while loop (which silently returns `job_name` even if the pod never starts) to a while/else that raises and cleans up on timeout:
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

Add test `test_live_pod_startup_timeout` in `test_lean_runner.py`: mock `list_namespaced_pod` to always return an empty `items` list (simulating a pod stuck in Pending); assert `RuntimeError` is raised with "never reached Running" in the message; assert `delete_namespaced_job` was called exactly once.
**Done when:** `python3 -m pytest python/test_lean_runner.py -k test_live_pod_startup_timeout -v` passes

---

### T-07 · celery_worker.py — UUID validation before _get_db()
**Status:** `reviewed`
**Depends on:** none
**Files:** `python/celery_worker.py`, `python/test_celery_worker.py`
**What:** In both `run_lean_backtest_task` and `run_lean_live_task`, add a UUID format check as the very first statement — before `_get_db()` is called. Currently `conn = _get_db()` is called at line 305 (backtest) and 379 (live) before the format check, causing a DB connection leak when `_validate_job_id` raises on a malformed UUID.

```python
@app.task(name="atp.run_lean_backtest", bind=True)
def run_lean_backtest_task(self, job_id: str):
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    log = _logger(job_id)
    conn = _get_db()
    try:
        ...
```

Apply the same pattern to `run_lean_live_task`. Remove the `_validate_job_id(conn, job_id)` call from both tasks — the format check above makes it redundant (a malformed UUID cannot exist as a DB row, so no DB update is needed).

Add test `test_invalid_uuid_no_db_call` in `test_celery_worker.py`: patch `celery_worker._get_db` with `side_effect=AssertionError`; call `run_lean_backtest_task.run("not-a-uuid")`; assert `ValueError` is raised (not `AssertionError`), confirming `_get_db` was never reached.
**Done when:** `python3 -m pytest python/test_celery_worker.py -k test_invalid_uuid_no_db_call -v` passes

---

### T-08 · jobs.go — CancelJob queued support + GetJob 404/500 split
**Status:** `reviewed`
**Depends on:** none
**Files:** `go-app/handlers/jobs.go`, `go-app/handlers/jobs_test.go`
**What:** Two corrections to `jobs.go`:

1. **`CancelJob` — fix dropped error + add queued cancellation.** `queue.SetStopSignal` returns `error` (verified in `queue/queue.go:56`) but the return value is currently discarded. Additionally, the handler returns 400 for `queued` jobs — which can and should be cancelled. Replace the current status check and `SetStopSignal` call:
```go
if status != "running" && status != "queued" {
    writeError(w, http.StatusBadRequest, "job is not running or queued")
    return
}
if status == "queued" {
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
    if err := queue.SetStopSignal(jobID); err != nil {
        log.Printf("CancelJob: SetStopSignal error for job %s: %v", jobID, err)
        writeError(w, http.StatusInternalServerError, "failed to cancel job")
        return
    }
}
```

2. **`GetJob` — distinguish 404 from 500.** The current `if err != nil { writeError 404 }` masks DB timeouts and transient errors. Replace with:
```go
if err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        writeError(w, http.StatusNotFound, "job not found")
    } else {
        writeError(w, http.StatusInternalServerError, "database error")
    }
    return
}
```

Add tests to `jobs_test.go`:
- `TestCancelJob_QueuedJob`: job in `queued` state → 202; subsequent status is `failed`
- `TestCancelJob_RedisError`: job in `running` state, `SetStopSignal` returns error → 500 with `"failed to cancel job"`
- `TestGetJob_DBError`: simulated DB error (not ErrNoRows) → 500
**Done when:** `go test ./go-app/handlers/ -run "TestCancelJob|TestGetJob_DBError" -v` passes

---

### T-09 · celery_worker.py — Pre-flight status check + remove unused imports
**Status:** `reviewed`
**Depends on:** T-07
**Files:** `python/celery_worker.py`
**What:** Two changes to `celery_worker.py`:

1. **Pre-flight status check** — add immediately after `_fetch_job(conn, job_id)` in both `run_lean_backtest_task` and `run_lean_live_task`. This handles the race where a `queued` job is cancelled (DB row set to `failed`) between enqueueing and Celery pickup:
```python
job = _fetch_job(conn, job_id)
if job["status"] != "queued":
    log.info("Job %s is in status %s, skipping (cancelled before pickup)", job_id, job["status"])
    return
```

2. **Remove unused imports** — delete `import tempfile` and `from pathlib import Path` (ruff F401 — these were left over from a prior refactor).
**Done when:** `python -m ruff check python/celery_worker.py` → 0 F401 findings for `tempfile` and `pathlib`; `python3 -m pytest python/test_celery_worker.py -v` passes

---

### T-10 · stream.go — rows.Close() before early return
**Status:** `reviewed`
**Depends on:** none
**Files:** `go-app/handlers/stream.go`, `go-app/handlers/stream_test.go`
**What:** In `JobStatusStream`, the `rows.Close()` call at line 175 is skipped when `conn.WriteMessage` fails at line 169 (which calls `return` directly). Add `rows.Close()` immediately before that `return` to prevent a resource leak on client disconnect:

```go
if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
    rows.Close()  // prevent leak: this return bypasses the Close at line 175
    return
}
```

The existing `rows.Close()` after the `for rows.Next()` loop is kept as the normal-exit path.

Add or update a test in `stream_test.go` (`TestJobStatusStream_WriteError` or equivalent) that simulates a WebSocket write error mid-stream and verifies the handler exits cleanly.
**Done when:** `go test ./go-app/handlers/ -run TestJobStatusStream -v` passes

---

### T-11 · strategies.go — errors.Is + stats query logging
**Status:** `reviewed`
**Depends on:** none
**Files:** `go-app/handlers/strategies.go`, `go-app/handlers/strategies_test.go`
**What:** Two corrections to `strategies.go`:

1. **Standardize `ErrNoRows` checks** — four sites (lines 202, 265, 348, 364) use direct equality `err == pgx.ErrNoRows` while the fifth (line 391) already uses `errors.Is`. Replace all four with `errors.Is(err, pgx.ErrNoRows)` for consistency and correctness if pgx ever wraps errors.

2. **Log stats query error in `GetStrategy`** — the stats `QueryRow(...).Scan(...)` call currently silently ignores errors, causing callers to receive zero-valued stats with no indication of failure. Add:
```go
if err := db.Pool.QueryRow(...).Scan(&runCount, &bestSharpe, &avgReturn); err != nil {
    log.Printf("GetStrategy: stats query error for strategy %s: %v", strategyID, err)
    // continue — partial response with zero stats is acceptable
}
```

Add test `TestGetStrategy_StatsError` to `strategies_test.go` that injects a stats query error and asserts the handler returns 200 (not 500) with the strategy data and zero-valued stats.
**Done when:** `grep "== pgx.ErrNoRows" go-app/handlers/strategies.go` → 0 lines; `go test ./go-app/handlers/ -run TestGetStrategy -v` passes

---

### T-12 · celery-worker-deployment.yaml — CELERY_BROKER_URL secret + remove KAFKA_NODE_IP
**Status:** `reviewed`
**Depends on:** none
**Files:** `kubernetes/celery/celery-worker-deployment.yaml`
**What:** Two changes to the Celery worker deployment manifest:

1. **`CELERY_BROKER_URL` — reference secret instead of hardcoded value.** The `atp-core-credentials` secret already contains `REDIS_URL` (confirmed from lines 42–46 of this same manifest). Replace:
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

2. **`KAFKA_NODE_IP` env var — remove.** The variable is read at module level in `lean_runner.py` but never referenced in any function. After T-13 removes the module-level read, the env var in the deployment is also unused. Remove the entire `KAFKA_NODE_IP` env block (lines 47–51):
```yaml
# Delete these 5 lines:
        - name: KAFKA_NODE_IP
          valueFrom:
            secretKeyRef:
              name: atp-core-credentials
              key: KAFKA_NODE_IP
```
**Done when:** `grep "value: redis://" kubernetes/celery/celery-worker-deployment.yaml` → 0 lines; `grep "KAFKA_NODE_IP" kubernetes/celery/celery-worker-deployment.yaml` → 0 lines

---

### T-13 · lean_runner.py — Remove KAFKA_NODE_IP + poll_live_results paginator + test
**Status:** `reviewed`
**Depends on:** T-06
**Files:** `python/lean_runner.py`, `python/test_lean_runner.py`
**What:** Two changes to `lean_runner.py`:

1. **Remove `KAFKA_NODE_IP` module-level read** — delete line `KAFKA_NODE_IP = os.environ["KAFKA_NODE_IP"]` (currently line 24). The variable is never referenced in any function; the K8s Job spec hardcodes `"kafka:9092"`. Its presence causes `ImportError` / `KeyError` in any environment where the env var is absent.

2. **`poll_live_results` — single paginated S3 call.** The current implementation makes two `list_objects_v2` calls (an existence check then a full list) and truncates at 1000 objects. Replace with a single paginator call:
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

Add or update test `test_poll_live_results` in `test_lean_runner.py` to mock `get_paginator` (not `list_objects_v2`) and assert it returns the latest JSON content.
**Done when:** `grep "KAFKA_NODE_IP" python/lean_runner.py` → 0 lines; `python3 -c "import lean_runner"` succeeds without `KAFKA_NODE_IP` in env; `python3 -m pytest python/test_lean_runner.py -k test_poll_live_results -v` passes

---

### T-14 · test_celery_worker.py — monkeypatch + requirements.txt test deps
**Status:** `reviewed`
**Depends on:** T-09
**Files:** `python/test_celery_worker.py`, `python/requirements.txt`
**What:** Two changes:

1. **`test_celery_worker.py` — replace direct attribute mutation with `monkeypatch`.** The `_run_task` helper currently patches module-level variables via direct assignment (`celery_worker.DATABASE_URL = dsn`), which leaves module state dirty if a test fails mid-run. Replace all such assignments with `monkeypatch.setattr(celery_worker, "VAR_NAME", value)`. The `_run_task` helper must accept `monkeypatch` as a parameter (or use pytest fixtures internally).

2. **`requirements.txt` — add missing test dependencies.** `test_celery_worker.py` imports `moto` and `testcontainers.postgres` which are absent from `requirements.txt`, making the suite uninstallable in CI. Add:
```
moto[s3]>=5.0
testcontainers>=4.0
```
**Done when:** `grep "celery_worker\." python/test_celery_worker.py | grep " = "` → 0 lines of direct attribute assignment; `grep -E "^moto|^testcontainers" python/requirements.txt` → 2 matching lines; `python3 -m pytest python/test_celery_worker.py -v` passes

---

### T-15 · Python — Black formatting across all files
**Status:** `reviewed`
**Depends on:** T-05, T-13, T-14
**Files:** `python/celery_worker.py`, `python/lean_runner.py`, `python/strategy_validator.py`, `python/test_celery_worker.py`, `python/test_lean_runner.py`, `python/test_strategy_validator.py`
**What:** Run `python -m black python/` to apply consistent formatting across all six Python files. This is a pure formatting pass — no logic changes. All prior Python tasks (T-05, T-06, T-07, T-09, T-13, T-14) must be complete before this runs so that black formats the final code, not intermediate states.
**Done when:** `python -m black --check python/` exits 0; `python3 -m pytest python/ -v` passes all tests

---

## Open Questions

None — all ambiguities resolved or inferred.
