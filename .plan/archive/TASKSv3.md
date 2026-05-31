# Tasks: ATP Wave 3 — Security Hardening & Bug Fixes
> Generated: 2026-05-29
> Source: .plan/
> Total: 16 tasks | Starting points: 12

## Dependency Graph

```
T-01 · jobs.go error handling & rows fixes
T-02 · strategies.go UploadNewVersion order & scan errors
T-03 · auth_test.go TestRefreshDeleteFailure trigger
T-04 · stream.go + main.go WebSocket CORS & stale comment
T-05 · auth.go clientIP() XFF last-hop
T-06 · go-app deployment.yaml env vars, JWT volume, probes
T-07 · go-data service.yaml + NetworkPolicy
T-08 · strategy_validator.py BLOCKED_MODULES extension
└── T-09 · test_strategy_validator.py new test cases
T-10 · lean_runner.py Docker hardening + kill loop fix
└── T-11 · test_lean_runner.py Docker flag assertions
T-12 · celery_worker.py fixes + conftest.py env fixtures
└── T-13 · test_celery_worker.py runtime error test fix
T-14 · celery-worker-deployment.yaml secretKeyRef entries
T-15 · api.ts BASE_URL export
└── T-16 · store.ts proactive refresh URL fix
```

---

## Tasks

### T-01 · jobs.go error handling & rows fixes
**Status:** `done`
**Depends on:** none
**Files:** `go-app/handlers/jobs.go`, `go-app/handlers/jobs_test.go`

**What:**

_EnqueueBacktest/EnqueueLive atomicity_ — The two `queue.Enqueue*` calls at `jobs.go:74-76` return errors that are currently ignored. Change both call sites to capture the error. On error, run:
```go
db.Pool.Exec(r.Context(),
    "UPDATE jobs SET status='failed', error_message=$1, completed_at=NOW() WHERE id=$2",
    "failed to queue job: "+enqueueErr.Error(), jobID)
writeError(w, http.StatusInternalServerError, "failed to queue job")
return
```

_GetJobMetrics resource leak & panic_ — At `jobs.go:257`, move `defer rows.Close()` to immediately after `db.Pool.Query(...)` (before the `rows.Next()` check) so the connection is always released. At `jobs.go:264`, change `vals, _ := rows.Values()` to check the error and return 500 on failure.

_rows.Err() checks_ — After every `for rows.Next()` loop in `jobs.go` (`ListJobs`, `GetJobMetrics`), add:
```go
if err := rows.Err(); err != nil {
    writeError(w, http.StatusInternalServerError, "reading results")
    return
}
```

_Test_ — In `jobs_test.go`, add `TestSubmitJob_RedisFailure`: use `testRedis.SetError("forced failure")` before calling `SubmitJob`, assert HTTP 500, then query the DB and assert `status='failed'` and `error_message != ''`. Restore with `testRedis.SetError("")` via `defer`.

**Done when:**
- `go test ./go-app/handlers/ -run TestSubmitJob` passes including the `RedisFailure` sub-case
- `go test ./go-app/handlers/ -run TestGetJobMetrics` passes (existing tests unbroken)
- `go test ./go-app/...` passes with no new failures

---

### T-02 · strategies.go UploadNewVersion order & scan errors
**Status:** `done`
**Depends on:** none
**Files:** `go-app/handlers/strategies.go`, `go-app/handlers/strategies_test.go`

**What:**

_Reverse S3/INSERT order in UploadNewVersion_ — Currently the order is: (1) QueryRow nextVersion, (2) S3 PutObject, (3) INSERT strategy_versions RETURNING id. Reverse to: (1) QueryRow nextVersion (check error → 500), (2) INSERT strategy_versions RETURNING id (check error → 500), (3) S3 PutObject (on failure: `DELETE FROM strategy_versions WHERE id=$versionID`, return 500). Use the RETURNING `versionID` for the response body.

_Fix all four QueryRow().Scan() silent error sites:_
- Line 253 (`ownerID` scan in `UploadNewVersion`): check error; on `pgx.ErrNoRows` return 404; on other errors return 500. Add `"github.com/jackc/pgx/v5"` import.
- Line 288 (`nextVersion` scan): check error → 500
- Line 299 (`versionID` scan, now post-INSERT): check error → 500 (covered by the order reversal above — ensure the check is explicit)
- Line 321 (`s3Key` scan in `GetVersionCode`): check error; on `pgx.ErrNoRows` return 404; on other errors return 500

_rows.Err() checks_ — After every `for rows.Next()` loop in `strategies.go` (`ListStrategies`, `GetStrategy` versions loop), add the same `rows.Err()` pattern as T-01.

_Tests_ — In `strategies_test.go`, add:
- `TestUploadNewVersion_DBInsertFailure`: close the testcontainer Postgres connection pool and assert the handler returns 500 without an S3 object being created (mock/verify via the fake S3 used in testhelper)
- `TestGetVersionCode_NotFound`: request a nonexistent `versionId` and assert 404 (not 500 or 403)

**Done when:**
- `go test ./go-app/handlers/ -run TestUploadNewVersion` passes including the DB-insert-failure sub-case
- `go test ./go-app/handlers/ -run TestGetVersionCode` passes including the not-found sub-case
- `go test ./go-app/...` passes with no new failures

---

### T-03 · auth_test.go TestRefreshDeleteFailure trigger
**Status:** `done`
**Depends on:** none
**Files:** `go-app/handlers/auth_test.go`

**What:**

Replace the auto-updatable view approach in `TestRefreshDeleteFailure` (lines 187–219) with a `BEFORE DELETE` trigger that raises an exception. The trigger must fire on the `refresh_tokens` table (not a renamed backing table), so SELECT still succeeds (token is found and validated) while DELETE fails.

Replace the current `ALTER TABLE ... RENAME` + `CREATE VIEW` block with:
```sql
CREATE FUNCTION _block_refresh_delete()
  RETURNS trigger LANGUAGE plpgsql AS $$
  BEGIN RAISE EXCEPTION 'delete blocked by test fixture'; END;
$$;
CREATE TRIGGER _block_delete
  BEFORE DELETE ON refresh_tokens
  FOR EACH ROW EXECUTE FUNCTION _block_refresh_delete();
```

In the `defer` cleanup, drop both objects:
```sql
DROP TRIGGER IF EXISTS _block_delete ON refresh_tokens;
DROP FUNCTION IF EXISTS _block_refresh_delete();
```

Remove the `t.Skipf` path — the trigger approach does not fail to create and does not need a skip guard.

**Done when:**
- `go test ./go-app/handlers/ -run TestRefreshDeleteFailure` passes (returns HTTP 500) on PostgreSQL 16
- No `t.Skip` path is reachable in the test

---

### T-04 · stream.go + main.go WebSocket CORS & stale comment
**Status:** `done`
**Depends on:** none
**Files:** `go-app/handlers/stream.go`, `go-app/main.go`

**What:**

_Shared origin parsing_ — In `stream.go`, extract the `allowedOrigins` list into a package-level variable populated at startup rather than re-read from the environment on every WebSocket connection. The variable should be set by calling `handlers.InitAllowedOrigins(os.Getenv("CORS_ORIGINS"))` from `main.go` before `http.ListenAndServe`.

`InitAllowedOrigins` should split the comma-separated env var into a `[]string`, trim whitespace from each entry, and store it in a package-level `var allowedOrigins []string` in `stream.go`. If the env var is empty, `allowedOrigins` should remain `nil`.

Update `CheckOrigin` in the WebSocket upgrader:
```go
CheckOrigin: func(r *http.Request) bool {
    if len(allowedOrigins) == 0 {
        return false  // deny all when not configured — no dev fallback
    }
    origin := r.Header.Get("Origin")
    for _, o := range allowedOrigins {
        if o == origin {
            return true
        }
    }
    return false
},
```

The existing `log.Fatal("CORS_ORIGINS must be set")` in `main.go` already fires before `ListenAndServe`, so the `allowedOrigins == nil → false` path only covers programmatic misuse, not normal startup.

_Stale comment_ — In `main.go:133`, change:
```
// WebSocket streams (auth via ?token= query param, handled inside handler)
```
to:
```
// WebSocket streams: auth via first message {"type":"auth","token":"<JWT>"}
```

**Done when:**
- `go test ./go-app/handlers/ -run TestStream` passes (or all existing stream tests pass if there are no named ones)
- The string `"?token="` does not appear in `main.go:133`
- `allowedOrigins` is not re-read from `os.Getenv` inside `CheckOrigin`

---

### T-05 · auth.go clientIP() XFF last-hop
**Status:** `done`
**Depends on:** none
**Files:** `go-app/handlers/auth.go`, `go-app/handlers/auth_test.go`

**What:**

Add a package-level helper in `auth.go`:
```go
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
```

Replace the two inline `r.Header.Get("X-Forwarded-For")` / `r.RemoteAddr` + `strings.LastIndex` blocks in the login rate-limit function (line 49) and register rate-limit function (line 116) with `clientIP(r)`.

Add import `"net"` if not already present.

_Test_ — In `auth_test.go`, add `TestClientIP`: call `clientIP` with a request where `X-Forwarded-For: 1.2.3.4, 5.6.7.8` and assert the result is `"5.6.7.8"`. Add a sub-case where the header is absent and assert the result equals the parsed `r.RemoteAddr` host.

**Done when:**
- `go test ./go-app/handlers/ -run TestClientIP` passes both sub-cases
- `go test ./go-app/handlers/ -run TestRateLimit` passes (existing rate-limit tests unbroken)
- The literal string `"X-Forwarded-For"` appears only in `clientIP`, not in the rate-limit functions directly

---

### T-06 · go-app deployment.yaml env vars, JWT volume, probes
**Status:** `done`
**Depends on:** none
**Files:** `kubernetes/go-app/deployment.yaml`

**What:**

Update the container spec in `kubernetes/go-app/deployment.yaml` with the following changes:

1. **Fix probe paths** — change both `livenessProbe.httpGet.path` and `readinessProbe.httpGet.path` from `/test` to `/api/health`.

2. **Add env vars** — under `containers[0].env`, add the following entries (all via `secretKeyRef` from a Sealed Secret named `atp-core-credentials` unless otherwise noted):
   - `DATABASE_URL` — `secretKeyRef: {name: atp-core-credentials, key: DATABASE_URL}`
   - `JWT_PRIVATE_KEY_PATH` — plain value `"/etc/jwt/private.pem"`
   - `JWT_PUBLIC_KEY_PATH` — plain value `"/etc/jwt/public.pem"`
   - `S3_ACCESS_KEY` — `secretKeyRef: {name: atp-core-credentials, key: S3_ACCESS_KEY}`
   - `S3_SECRET_KEY` — `secretKeyRef: {name: atp-core-credentials, key: S3_SECRET_KEY}`
   - `S3_BUCKET` — `secretKeyRef: {name: atp-core-credentials, key: S3_BUCKET}`
   - `REDIS_URL` — `secretKeyRef: {name: atp-core-credentials, key: REDIS_URL}`
   - `CORS_ORIGINS` — `secretKeyRef: {name: atp-core-credentials, key: CORS_ORIGINS}`
   - `PYTHON_SERVICE_URL` — plain value `"http://python-service:8082"`

3. **Add JWT volume mount and volume**:
   ```yaml
   volumeMounts:
   - name: jwt-keypair
     mountPath: /etc/jwt
     readOnly: true
   volumes:
   - name: jwt-keypair
     secret:
       secretName: jwt-keypair
   ```
   The Sealed Secret `jwt-keypair` must have keys `private.pem` and `public.pem` — its creation is an operational prerequisite (run via `kubernetes/initialize.sh`) not tracked here.

**Done when:**
- `kubernetes/go-app/deployment.yaml` contains no reference to `/test` in probe paths
- `kubernetes/go-app/deployment.yaml` contains `secretKeyRef` entries for all 7 secret-backed vars listed above
- `kubernetes/go-app/deployment.yaml` contains the `jwt-keypair` volume and volumeMount
- `kubectl apply --dry-run=client -f kubernetes/go-app/deployment.yaml` exits 0 (optional local verify)

---

### T-07 · go-data service.yaml + NetworkPolicy
**Status:** `done`
**Depends on:** none
**Files:** `[new] kubernetes/go-data/service.yaml`, `[new] kubernetes/go-data/network-policy.yaml`

**What:**

Create `kubernetes/go-data/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: go-data-service
spec:
  selector:
    app: go-data
  ports:
  - name: http
    port: 8081
    targetPort: 8081
  type: ClusterIP
```

Create `kubernetes/go-data/network-policy.yaml`:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: go-data-allow-celery
spec:
  podSelector:
    matchLabels:
      app: go-data
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: celery-worker
    ports:
    - port: 8081
      protocol: TCP
  policyTypes:
  - Ingress
```

**Done when:**
- `kubernetes/go-data/service.yaml` exists with `name: go-data-service`, `port: 8081`, `selector.app: go-data`
- `kubernetes/go-data/network-policy.yaml` exists with ingress restricted to pods with `app: celery-worker` on port 8081
- `kubectl apply --dry-run=client -f kubernetes/go-data/service.yaml -f kubernetes/go-data/network-policy.yaml` exits 0

---

### T-08 · strategy_validator.py BLOCKED_MODULES extension
**Status:** `done`
**Depends on:** none
**Files:** `python/strategy_validator.py`

**What:**

1. Extend the `BLOCKED_MODULES` set to include `"importlib"`, `"importlib.util"`, `"importlib.machinery"`, `"ctypes"`, `"builtins"`. The existing members (`"os"`, `"sys"`, `"subprocess"`, `"socket"`) are unchanged.

2. Add a new `BLOCKED_ATTRS` constant:
   ```python
   BLOCKED_ATTRS = {"__import__", "__builtins__", "__loader__"}
   ```

3. In the AST walk loop, add a check for `ast.Attribute` nodes:
   ```python
   if isinstance(node, ast.Attribute) and node.attr in BLOCKED_ATTRS:
       return {"valid": False, "violation": f"use of blocked attribute '{node.attr}' on line {node.lineno}"}
   ```
   This must run alongside (not replace) the existing `ast.Import` / `ast.ImportFrom` / `ast.Call` checks.

4. Add a module-level comment above the `BLOCKED_MODULES` definition:
   ```python
   # Security note: The AST scan is a first-line UX check, not a security boundary.
   # Docker container isolation (--network none / lean-live-net, --cap-drop ALL) is enforced separately.
   ```

5. Fix the error message for blocked builtin calls (e.g. `eval`): change `f"import {name} detected on line {node.lineno}"` to `f"use of blocked builtin '{name}' on line {node.lineno}"`.

**Done when:**
- `python3 -c "from strategy_validator import validate_strategy; print(validate_strategy('import importlib'))"` prints `{'valid': False, 'violation': '...'}`
- Same for `'import ctypes'` and `'import builtins'`
- `python3 -m pytest python/test_strategy_validator.py -v` passes all existing tests

---

### T-09 · test_strategy_validator.py new test cases
**Status:** `done`
**Depends on:** T-08
**Files:** `python/test_strategy_validator.py`

**What:**

Add the following test cases to `test_strategy_validator.py`:

- `test_importlib_blocked`: source = `"import importlib"`, assert `result["valid"] is False` and `"importlib"` in `result["violation"]`
- `test_ctypes_blocked`: source = `"import ctypes"`, assert `result["valid"] is False`
- `test_builtins_blocked`: source = `"import builtins"`, assert `result["valid"] is False`
- `test_attribute_import_blocked`: source = `"x = obj.__import__('os')"`, assert `result["valid"] is False` and `"__import__"` in `result["violation"]`
- `test_importlib_submodule_blocked`: source = `"from importlib.util import find_spec"`, assert `result["valid"] is False`

**Done when:**
- `python3 -m pytest python/test_strategy_validator.py -v` passes all tests including the 5 new cases
- All 5 new test functions are present in `test_strategy_validator.py`

---

### T-10 · lean_runner.py Docker hardening + kill loop fix
**Status:** `done`
**Depends on:** none
**Files:** `python/lean_runner.py`

**What:**

_Backtest command_ — In `run_lean_backtest`, change the `cmd` list to include `--network none`, `--memory 2g`, `--cpus 2`, `--cap-drop ALL`, `--no-new-privileges`:
```python
cmd = [
    "docker", "run", "--rm",
    "--label", f"job_id={job_id}",
    "--network", "none",
    "--memory", "2g", "--cpus", "2",
    "--cap-drop", "ALL", "--no-new-privileges",
    "-v", f"{os.path.abspath(job_dir)}:/lean",
    LEAN_IMAGE
]
```

_Live command_ — In `run_lean_live`, replace `--add-host=host.docker.internal:host-gateway` with `--network lean-live-net` and `--add-host kafka-broker:{KAFKA_NODE_IP}`. Add resource flags. Read `KAFKA_NODE_IP` from `os.environ["KAFKA_NODE_IP"]` at module level or inside the function:
```python
KAFKA_NODE_IP = os.environ.get("KAFKA_NODE_IP", "host.docker.internal")

cmd = [
    "docker", "run", "-d", "--rm",
    "--label", f"job_id={job_id}",
    "--network", "lean-live-net",
    "--add-host", f"kafka-broker:{KAFKA_NODE_IP}",
    "--memory", "2g", "--cpus", "2",
    "--cap-drop", "ALL", "--no-new-privileges",
    "-v", f"{os.path.abspath(job_dir)}:/lean",
    LEAN_IMAGE
]
```

_docker kill loop fix_ — In the `TimeoutExpired` handler, move the `try/except` inside the `for cid` loop:
```python
for cid in kill_result.stdout.strip().splitlines():
    if cid:
        try:
            subprocess.run(["docker", "kill", cid], check=True)
        except subprocess.CalledProcessError as e:
            logger.error("Failed to kill container %s: %s", cid, e)
```

**Done when:**
- `python3 -m pytest python/test_lean_runner.py -v` passes all existing tests (no regressions)
- The string `"host.docker.internal"` does not appear as a `--add-host` argument in `run_lean_live`'s command construction
- The string `"--network"` appears in both `run_lean_backtest` and `run_lean_live` command lists

---

### T-11 · test_lean_runner.py Docker flag assertions
**Status:** `done`
**Depends on:** T-10
**Files:** `python/test_lean_runner.py`

**What:**

In the existing `test_run_lean_backtest_success` test (which mocks `subprocess.run`), add assertions on the captured `cmd` argument:
```python
assert "--network" in cmd and cmd[cmd.index("--network") + 1] == "none"
assert "--memory" in cmd and cmd[cmd.index("--memory") + 1] == "2g"
assert "--cap-drop" in cmd and cmd[cmd.index("--cap-drop") + 1] == "ALL"
assert "--no-new-privileges" in cmd
```

In the existing `test_run_lean_live_returns_container_id` test, add assertions:
```python
assert "--network" in cmd and cmd[cmd.index("--network") + 1] == "lean-live-net"
assert "--memory" in cmd and cmd[cmd.index("--memory") + 1] == "2g"
assert "--no-new-privileges" in cmd
# verify --add-host is NOT host.docker.internal
add_host_idx = cmd.index("--add-host") if "--add-host" in cmd else -1
if add_host_idx >= 0:
    assert "host.docker.internal" not in cmd[add_host_idx + 1]
```

Add a new test `test_backtest_timeout_kills_all_containers`: mock `subprocess.run` so the first call raises `TimeoutExpired`, the second call (docker ps) returns two container IDs, and the third/fourth calls (docker kill) are mocked. Assert `docker kill` is called for both container IDs even if one returns non-zero (simulate by having the kill mock raise `CalledProcessError` on the first ID only).

**Done when:**
- `python3 -m pytest python/test_lean_runner.py -v` passes all tests including the new `test_backtest_timeout_kills_all_containers`
- The flag assertions are present in both the backtest and live success tests

---

### T-12 · celery_worker.py fixes + conftest.py env fixtures
**Status:** `done`
**Depends on:** none
**Files:** `python/celery_worker.py`, `[new] python/conftest.py`

**What:**

_lean-live-net Celery signal_ — Add at the top of `celery_worker.py` (after imports):
```python
from celery.signals import worker_init

@worker_init.connect
def _create_lean_network(**kwargs):
    if not os.path.exists("/var/run/docker.sock"):
        return
    subprocess.run(
        ["docker", "network", "create", "lean-live-net", "--driver", "bridge"],
        capture_output=True  # ignore error — network may already exist
    )
```

_producer.flush fix_ — Move `producer = None` before the outer `try` block. Inside the function, assign `producer = Producer(...)` conditionally. After the `try/except/finally` block, flush with a timeout guard:
```python
producer = None
try:
    producer = Producer({"bootstrap.servers": KAFKA_BOOTSTRAP_SERVERS})
    # ... existing live task logic ...
except Exception as e:
    # ... existing error handling ...
finally:
    r.close()
if producer is not None:
    producer.flush(timeout=10)
```

_Remove credential fallbacks_ — Change the following module-level assignments from `os.environ.get(key, default)` to `os.environ[key]` (raises `KeyError` at import if missing):
- `DATABASE_URL`
- `S3_ACCESS_KEY`
- `S3_SECRET_KEY`
- `S3_BUCKET`
- `GO_DATA_URL`

Keep `KAFKA_BOOTSTRAP_SERVERS`, `REDIS_URL`, `CELERY_BROKER_URL`, and `LEAN_IMAGE` as `os.environ.get(key, default)` — they are not credentials.

_Update LEAN Kafka default_ — Change `LEAN_KAFKA_BOOTSTRAP_SERVERS` default from `"host.docker.internal:9092"` to `"kafka-broker:9092"`:
```python
LEAN_KAFKA_BOOTSTRAP_SERVERS = os.environ.get("LEAN_KAFKA_BOOTSTRAP_SERVERS", "kafka-broker:9092")
```

_New `python/conftest.py`_ — Create with a session-scoped autouse fixture that sets the required env vars before any test module imports `celery_worker`:
```python
import os
import pytest

@pytest.fixture(scope="session", autouse=True)
def _set_celery_env():
    os.environ.setdefault("DATABASE_URL", "postgresql://test:test@localhost:5432/test")
    os.environ.setdefault("S3_ACCESS_KEY", "test-access-key")
    os.environ.setdefault("S3_SECRET_KEY", "test-secret-key")
    os.environ.setdefault("S3_BUCKET", "test-bucket")
    os.environ.setdefault("GO_DATA_URL", "http://localhost:8081")
```
(The per-test fixtures in `test_celery_worker.py` that assign `celery_worker.DATABASE_URL = dsn` continue to work and override these defaults for integration tests.)

**Done when:**
- `python3 -c "import celery_worker"` raises `KeyError` when `DATABASE_URL` is absent from the environment
- `python3 -m pytest python/test_celery_worker.py -v` passes (existing tests unbroken; conftest.py provides the env defaults)
- `LEAN_KAFKA_BOOTSTRAP_SERVERS` default is `"kafka-broker:9092"` (grep-verifiable)
- The string `"host.docker.internal"` does not appear as a fallback in `celery_worker.py`

---

### T-13 · test_celery_worker.py runtime error test fix
**Status:** `done`
**Depends on:** T-12
**Files:** `python/test_celery_worker.py`

**What:**

In `test_lean_runtime_error` (around line 294):
1. Add `_seed_market_data(pg_dsn)` before `_run_task(...)`, identical to the call in `test_happy_path`. This ensures the market-data guard doesn't short-circuit the test before the mocked `run_lean_backtest` is reached.
2. Update the assertion after `_run_task` to verify: `job["status"] == "failed"` AND `"RuntimeError" in job["error_message"]` (or whatever string the mock raises). The current assertion only checks `status == "failed"`, which passes regardless of whether the runtime error path was exercised.

**Done when:**
- `python3 -m pytest python/test_celery_worker.py::test_lean_runtime_error -v` passes
- Removing the `_seed_market_data` call causes the test to fail (verifying it's now exercising the correct path)
- `python3 -m pytest python/ -v` passes all tests

---

### T-14 · celery-worker-deployment.yaml secretKeyRef entries
**Status:** `done`
**Depends on:** none
**Files:** `kubernetes/celery/celery-worker-deployment.yaml`

**What:**

Add the following to `containers[0].env` (retain the existing `CELERY_BROKER_URL` plain-value entry):

```yaml
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: atp-core-credentials
      key: DATABASE_URL
- name: S3_ACCESS_KEY
  valueFrom:
    secretKeyRef:
      name: atp-core-credentials
      key: S3_ACCESS_KEY
- name: S3_SECRET_KEY
  valueFrom:
    secretKeyRef:
      name: atp-core-credentials
      key: S3_SECRET_KEY
- name: S3_BUCKET
  valueFrom:
    secretKeyRef:
      name: atp-core-credentials
      key: S3_BUCKET
- name: REDIS_URL
  valueFrom:
    secretKeyRef:
      name: atp-core-credentials
      key: REDIS_URL
- name: KAFKA_NODE_IP
  valueFrom:
    secretKeyRef:
      name: atp-core-credentials
      key: KAFKA_NODE_IP
- name: LEAN_KAFKA_BOOTSTRAP_SERVERS
  value: "kafka-broker:9092"
- name: PYTHON_SERVICE_URL
  value: "http://python-service:8082"
- name: GO_DATA_URL
  value: "http://go-data-service:8081"
```

Also add the Docker socket volume mount (required for `docker run` from within the pod):
```yaml
containers:
- name: celery-worker
  ...
  volumeMounts:
  - name: docker-sock
    mountPath: /var/run/docker.sock
volumes:
- name: docker-sock
  hostPath:
    path: /var/run/docker.sock
    type: Socket
```

**Done when:**
- `kubernetes/celery/celery-worker-deployment.yaml` contains `secretKeyRef` entries for `DATABASE_URL`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `REDIS_URL`, `KAFKA_NODE_IP`
- The file contains the `docker-sock` hostPath volume and matching volumeMount
- `kubectl apply --dry-run=client -f kubernetes/celery/celery-worker-deployment.yaml` exits 0

---

### T-15 · api.ts BASE_URL export
**Status:** `done`
**Depends on:** none
**Files:** `web/src/lib/api.ts`

**What:**

Change line 4 of `web/src/lib/api.ts` from:
```typescript
const BASE_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
```
to:
```typescript
export const BASE_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
```

No other changes to `api.ts`.

**Done when:**
- `grep "export const BASE_URL" web/src/lib/api.ts` exits 0
- `cd web && npx tsc --noEmit` exits 0 (no TypeScript errors introduced)

---

### T-16 · store.ts proactive refresh URL fix
**Status:** `done`
**Depends on:** T-15
**Files:** `web/src/lib/store.ts`

**What:**

In `web/src/lib/store.ts`:

1. Add the import at the top of the file (alongside other imports from `'../lib/api'` or wherever `api.ts` is imported from):
   ```typescript
   import { BASE_URL } from './api'
   ```

2. On line 33 (the proactive refresh `fetch` call), change:
   ```typescript
   const { data } = await axios.post('/api/auth/refresh', {
   ```
   to:
   ```typescript
   const { data } = await axios.post(`${BASE_URL}/api/auth/refresh`, {
   ```
   (Or if the call uses `fetch` rather than `axios`, apply the same substitution to the `fetch` URL argument.)

3. Verify the string `'/api/auth/refresh'` (relative URL, without the BASE_URL prefix) no longer appears anywhere in `store.ts`.

**Done when:**
- `grep "'/api/auth/refresh'" web/src/lib/store.ts` returns no matches
- `grep "BASE_URL" web/src/lib/store.ts` returns at least one match
- `cd web && npm run build` exits 0 with no TypeScript errors

---

## Open Questions

None — all ambiguities resolved or inferred.
