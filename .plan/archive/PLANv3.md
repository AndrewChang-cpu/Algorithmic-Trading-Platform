# Plan: ATP Wave 3 — Security Hardening & Bug Fixes
> Generated: 2026-05-29
> Type: brownfield
> Documents: single file
> Archived: [PLANv2.md](archive/PLANv2.md) (Wave 2 code quality remediation)

## Overview
**What:** A targeted third wave of fixes addressing bugs and security gaps surfaced by a deep three-agent review of the full ATP codebase. Covers Go API (`go-app`), Go data service (`go-data`), Python Celery workers, and Kubernetes manifests. No new features; no API contract changes; no UI changes.

**Why:** The Wave 2 remediation left 8 code-level blockers (silent job dispatch failures, panicking handlers, orphaned S3 objects, broken rate-limit bypass, stale test), 4 critical security issues (LEAN containers running with full host network, hardcoded credential fallbacks silently active in production, go-app K8s deployment missing all env vars), and several high-severity operational gaps.

**Who:** Internal — no user-visible behavior changes.

---

## Definition of Done

### go-app

- [ ] `POST /api/jobs` when Redis is unavailable returns HTTP 500; the job row in `jobs` has `status='failed'` and a non-empty `error_message`; `go test ./go-app/handlers/ -run TestCreateJob` passes a case that injects a Redis failure and asserts both the 500 response and the `failed` row state
- [ ] `GET /api/jobs/:id/metrics` with a valid job ID returns 200; with a nonexistent ID returns 404; no pgx connection leak — `defer rows.Close()` appears immediately after `db.Pool.Query`, before any `rows.Next()` check; `rows.Values()` error causes a 500 response, not a panic
- [ ] `POST /api/strategies/:id/versions` when the DB INSERT fails: the handler returns 500, the S3 object does NOT exist; when DB INSERT succeeds but S3 PutObject fails: the handler returns 500 and the DB row is deleted; `nextVersion` QueryRow error is checked and returns 500 on failure
- [ ] All four `QueryRow().Scan()` sites in `handlers/strategies.go` (lines 253, 288, 299, 321) return 404 on `pgx.ErrNoRows` and 500 on other errors; `rows.Err()` is checked after every `for rows.Next()` loop in all handlers
- [ ] `TestRefreshDeleteFailure` in `handlers/auth_test.go` passes with a `BEFORE DELETE` trigger implementation; the test does NOT use a view; `go test ./go-app/handlers/ -run TestRefreshDeleteFailure` passes on PostgreSQL 16
- [ ] WebSocket upgrader `CheckOrigin` in `handlers/stream.go` uses the same parsed origin slice as the HTTP CORS middleware; if `CORS_ORIGINS` is unset at startup, `log.Fatal` fires before the server accepts any connection (WebSocket and HTTP both covered)
- [ ] Rate limit client IP is extracted as the last comma-separated value of `X-Forwarded-For`; if the header is absent, `r.RemoteAddr` is used with the port stripped; `go test ./go-app/handlers/ -run TestRateLimit` passes a case where the header contains two IPs and verifies the last one is used
- [ ] `kubernetes/go-app/deployment.yaml` contains `secretKeyRef` entries for: `DATABASE_URL`, `JWT_PRIVATE_KEY_PATH`, `JWT_PUBLIC_KEY_PATH`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `REDIS_URL`, `CORS_ORIGINS`, `PYTHON_SERVICE_URL`; JWT PEM files are mounted as a volume from a `jwt-keypair` Sealed Secret at `/etc/jwt/private.pem` and `/etc/jwt/public.pem`; liveness and readiness probes use path `/api/health`
- [ ] `go test ./go-app/...` passes

### go-data

- [ ] `kubernetes/go-data/service.yaml` exists: `kind: Service`, `name: go-data-service`, selector `app: go-data`, port 8081, type `ClusterIP`
- [ ] `kubernetes/go-data/network-policy.yaml` exists: `kind: NetworkPolicy` allowing ingress to port 8081 only from pods with label `app: celery-worker`; no other ingress allowed
- [ ] `go test ./go-data/...` passes

### Python workers

- [ ] `POST http://localhost:8082/validate` with source `import importlib` returns `{"valid": false, "violation": "...importlib..."}` (status 200); same for `import ctypes`, `import builtins`, and source containing a call `__import__('os')` expressed as an attribute lookup
- [ ] `python3 -m pytest python/test_strategy_validator.py -v` passes including new test cases for `importlib`, `ctypes`, `builtins`, and attribute-based `__import__` invocation
- [ ] `docker inspect` output for a running backtest LEAN container shows `NetworkMode: none`; `docker inspect` output for a running live LEAN container shows `NetworkMode: lean-live-net`; neither container type has `--add-host=host.docker.internal:*`
- [ ] Both backtest and live LEAN Docker commands include `--memory 2g`, `--cpus 2`, `--cap-drop ALL`, `--no-new-privileges`
- [ ] On backtest timeout, `docker kill` is called with `check=True` inside the `for cid` loop; a `CalledProcessError` from `docker kill` is logged at `error` level and does NOT abort the kill loop for remaining container IDs
- [ ] `celery_worker.py` module-level: `DATABASE_URL`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `GO_DATA_URL` use `os.environ["KEY"]` (no default fallback); `python3 -c "import celery_worker"` raises `KeyError` when `DATABASE_URL` is absent from the environment
- [ ] `LEAN_KAFKA_BOOTSTRAP_SERVERS` default is `"kafka-broker:9092"`; the `run_lean_live` Docker command includes `--add-host=kafka-broker:${KAFKA_NODE_IP}` where `KAFKA_NODE_IP` comes from `os.environ["KAFKA_NODE_IP"]`
- [ ] `producer` is initialized as `None` before the `try` block in `run_lean_live_task`; `producer.flush(timeout=10)` is called only when `producer is not None`, inside a `finally` block or after the `try/except`
- [ ] `test_lean_runtime_error` in `test_celery_worker.py` calls `_seed_market_data(pg_dsn)` before `_run_task`; the test verifies the job ends in `failed` state with an `error_message` matching the RuntimeError raised by the mock
- [ ] `kubernetes/celery/celery-worker-deployment.yaml` contains `secretKeyRef` entries for: `DATABASE_URL`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `REDIS_URL`, `KAFKA_NODE_IP`, `LEAN_KAFKA_BOOTSTRAP_SERVERS`, `PYTHON_SERVICE_URL`, `GO_DATA_URL`
- [ ] `python3 -m pytest python/ -v` passes all tests

### React

- [ ] `web/src/lib/api.ts` exports `BASE_URL` as a named export
- [ ] `web/src/lib/store.ts` imports `BASE_URL` from `../lib/api` and uses `` `${BASE_URL}/api/auth/refresh` `` for the proactive refresh fetch; the relative `/api/auth/refresh` string does not appear in store.ts
- [ ] `npm run build` exits 0 with no TypeScript errors

---

## Unchanged Behavior

- WHEN a user submits a valid backtest job THEN the response SHALL continue to be `202 Accepted` with `{"jobId": "<uuid>"}`
- WHEN a backtest completes THEN `performance_metrics` and `portfolio_metrics` rows SHALL continue to be inserted with the same schema
- WHEN a user uploads a valid strategy THEN the response SHALL continue to be `201 Created` with `{"versionId": "<uuid>", "versionNumber": N}`
- WHEN a WebSocket client sends a valid first-message auth THEN streaming SHALL continue to work as before
- WHEN `POST /data/historical` is called with a fully-cached range THEN Alpaca SHALL NOT be called (dedup preserved)

---

## Fixes by Area

### go-app: Job dispatch atomicity (`handlers/jobs.go`)

**Current:** `queue.EnqueueBacktest(jobID)` / `queue.EnqueueLive(jobID)` return values are ignored. On Redis failure the job row stays in `queued` indefinitely.

**Fix:**
```go
var enqueueErr error
if req.Type == "live" {
    enqueueErr = queue.EnqueueLive(jobID)
} else {
    enqueueErr = queue.EnqueueBacktest(jobID)
}
if enqueueErr != nil {
    db.Pool.Exec(r.Context(),
        "UPDATE jobs SET status='failed', error_message=$1, completed_at=NOW() WHERE id=$2",
        "failed to queue job: "+enqueueErr.Error(), jobID)
    writeError(w, http.StatusInternalServerError, "failed to queue job")
    return
}
```

---

### go-app: GetJobMetrics resource leak and panic (`handlers/jobs.go:257-264`)

**Current:** `defer rows.Close()` only reached if `rows.Next()` is true (zero-row early return leaks connection). `rows.Values()` error discarded — nil slice panics the handler goroutine.

**Fix:** Move `defer rows.Close()` immediately after `db.Pool.Query`. Check `rows.Values()` error; on failure return 500.

---

### go-app: UploadNewVersion operation order and error handling (`handlers/strategies.go`)

**Current:** S3 PutObject runs before the DB INSERT; INSERT error is not checked; orphaned S3 objects possible; `nextVersion` QueryRow error not checked.

**Fix — reverse the order:**
1. Check `nextVersion` QueryRow error → 500 on failure
2. Compute `s3Key`
3. `INSERT INTO strategy_versions ... RETURNING id` → check error → 500 on failure; capture `versionID`
4. `s3client.PutObject(...)` → if fails: `DELETE FROM strategy_versions WHERE id=$versionID`, return 500

**Fix all four QueryRow().Scan() silent error sites:**
- Line 253: `pgx.ErrNoRows` → 404; other errors → 500
- Line 288: `nextVersion` query error → 500
- Line 299: `versionID` scan error → 500 (covered by the order reversal above)
- Line 321: `GetVersionCode` s3Key scan error → 500

**Add `rows.Err()` checks** after every `for rows.Next()` loop in `ListStrategies`, `GetStrategy`, `ListJobs`, `GetPortfolio`.

---

### go-app: TestRefreshDeleteFailure reliability (`handlers/auth_test.go`)

**Current:** View-based DELETE simulation is auto-updatable in PostgreSQL 16; DELETE succeeds, handler returns 200, test fails.

**Fix:**
```sql
CREATE FUNCTION _block_refresh_delete() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'delete blocked by test fixture'; END; $$;
CREATE TRIGGER _block_delete BEFORE DELETE ON refresh_tokens
FOR EACH ROW EXECUTE FUNCTION _block_refresh_delete();
```
`defer` drops both the trigger and function. The SELECT still succeeds (token found), DELETE raises exception, Refresh handler returns 500.

---

### go-app: WebSocket CORS fallback (`handlers/stream.go`)

**Current:** `CheckOrigin` returns `true` when `CORS_ORIGINS` is empty — accepts WebSocket from any origin.

**Fix:** Extract the origin parsing into a shared `parseAllowedOrigins(env string) []string` helper called at startup. Both the HTTP CORS middleware and `CheckOrigin` use the result. `log.Fatal("CORS_ORIGINS must be set")` before `ListenAndServe` covers both.

---

### go-app: Rate limit IP extraction (`handlers/auth.go`)

**Current:** Full `X-Forwarded-For` header value used as rate-limit key; client can spoof any prefix.

**Fix:**
```go
func clientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        parts := strings.Split(xff, ",")
        return strings.TrimSpace(parts[len(parts)-1])
    }
    ip, _, _ := net.SplitHostPort(r.RemoteAddr)
    return ip
}
```
Use `clientIP(r)` in both login and register rate-limit key construction.

---

### go-app: Stale comment (`main.go:133`)

Change `// WebSocket streams (auth via ?token= query param, handled inside handler)` to `// WebSocket streams: auth via first message {"type":"auth","token":"<JWT>"}`.

---

### go-app: Kubernetes deployment (`kubernetes/go-app/deployment.yaml`)

Add the following to the container spec:

**Env vars (secretKeyRef):**
```yaml
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: atp-core-credentials
      key: DATABASE_URL
- name: JWT_PRIVATE_KEY_PATH
  value: /etc/jwt/private.pem
- name: JWT_PUBLIC_KEY_PATH
  value: /etc/jwt/public.pem
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
- name: CORS_ORIGINS
  valueFrom:
    secretKeyRef:
      name: atp-core-credentials
      key: CORS_ORIGINS
- name: PYTHON_SERVICE_URL
  value: http://python-service:8082
```

**JWT volume mount (PEM files):**
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

**Fix probe paths:**
```yaml
livenessProbe:
  httpGet:
    path: /api/health
readinessProbe:
  httpGet:
    path: /api/health
```

A Sealed Secret named `jwt-keypair` must exist with keys `private.pem` and `public.pem` (created via `kubernetes/initialize.sh` pattern). A Sealed Secret named `atp-core-credentials` must exist with the keys listed above.

---

### go-data: Kubernetes Service and NetworkPolicy

**New file `kubernetes/go-data/service.yaml`:**
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

**New file `kubernetes/go-data/network-policy.yaml`:**
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
  policyTypes:
  - Ingress
```

---

### Python: Strategy sandbox extension (`python/strategy_validator.py`)

**Current:** `BLOCKED_MODULES` does not include `importlib`, `ctypes`, `builtins`; `ast.Attribute` access to `__import__` / `__builtins__` not checked.

**Fix — extend BLOCKED_MODULES:**
```python
BLOCKED_MODULES = {
    "os", "sys", "subprocess", "socket", "eval", "exec",
    "importlib", "importlib.util", "importlib.machinery",
    "ctypes", "builtins",
}
```

**Fix — add ast.Attribute check for `__import__` and `__builtins__`:**
```python
BLOCKED_ATTRS = {"__import__", "__builtins__", "__loader__"}

for node in ast.walk(tree):
    if isinstance(node, ast.Attribute) and node.attr in BLOCKED_ATTRS:
        return {"valid": False, "violation": f"use of blocked attribute '{node.attr}' on line {node.lineno}"}
```

Add module-level docstring: `# Security note: The AST scan is a first-line UX check only. Docker container isolation (--network none / lean-live-net, --cap-drop ALL) is the enforced security boundary.`

**New tests in `test_strategy_validator.py`:** test cases for `import importlib`, `import ctypes`, `import builtins`, and a source string containing `x.__import__('os')`.

---

### Python: LEAN Docker hardening (`python/lean_runner.py`)

**Backtests — add `--network none` and resource limits:**
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

**Live — replace `--add-host=host.docker.internal` with bridge network:**
```python
KAFKA_NODE_IP = os.environ["KAFKA_NODE_IP"]

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

**`lean-live-net` creation — Celery `worker_init` signal in `celery_worker.py`:**
```python
from celery.signals import worker_init

@worker_init.connect
def create_lean_network(**kwargs):
    if not os.path.exists("/var/run/docker.sock"):
        return
    subprocess.run(
        ["docker", "network", "create", "lean-live-net", "--driver", "bridge"],
        capture_output=True
    )  # ignore error — network may already exist
```

**docker kill — `check=True` inside loop:**
```python
for cid in kill_result.stdout.strip().splitlines():
    if cid:
        try:
            subprocess.run(["docker", "kill", cid], check=True)
        except subprocess.CalledProcessError as e:
            logger.error("Failed to kill container %s: %s", cid, e)
```

**New assertions in `test_lean_runner.py`:** verify `--network none` in backtest command; verify `--network lean-live-net` and `--memory 2g` in live command; verify `docker kill check=True` semantics on timeout.

---

### Python: Credential handling (`python/celery_worker.py`)

**Remove hardcoded fallbacks — fail fast at import:**
```python
DATABASE_URL = os.environ["DATABASE_URL"]
S3_ACCESS_KEY = os.environ["S3_ACCESS_KEY"]
S3_SECRET_KEY = os.environ["S3_SECRET_KEY"]
S3_BUCKET = os.environ["S3_BUCKET"]
GO_DATA_URL = os.environ["GO_DATA_URL"]
```
`KAFKA_BOOTSTRAP_SERVERS` (used by the Celery-side Kafka producer) retains `os.environ.get(...)` with the existing default — only credential-bearing vars are changed.

**Update Kafka bootstrap for LEAN containers:**
```python
LEAN_KAFKA_BOOTSTRAP_SERVERS = os.environ.get("LEAN_KAFKA_BOOTSTRAP_SERVERS", "kafka-broker:9092")
```

**producer.flush fix:**
```python
producer = None
try:
    producer = Producer({"bootstrap.servers": KAFKA_BOOTSTRAP_SERVERS})
    # ... live trading logic ...
except Exception as e:
    # ... mark job failed ...
finally:
    r.close()
if producer is not None:
    producer.flush(timeout=10)
```

**Test suite — conftest.py env vars:** Add `python/conftest.py` with a `session`-scoped `autouse` fixture that sets `DATABASE_URL`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `GO_DATA_URL` to test values before any module import.

---

### Python: Fix test_lean_runtime_error (`python/test_celery_worker.py:294`)

Add `_seed_market_data(pg_dsn)` call before `_run_task(...)` in `test_lean_runtime_error`, identical to `test_happy_path`. The test must then assert `job["status"] == "failed"` and `"RuntimeError" in job["error_message"]`.

---

### Python: Kubernetes deployment (`kubernetes/celery/celery-worker-deployment.yaml`)

Add `secretKeyRef` entries for: `DATABASE_URL`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_BUCKET`, `REDIS_URL`, `KAFKA_NODE_IP`, `LEAN_KAFKA_BOOTSTRAP_SERVERS`, `PYTHON_SERVICE_URL`, `GO_DATA_URL` — all from a Sealed Secret named `atp-core-credentials`. Retain the existing `CELERY_BROKER_URL` plain-value entry.

Also ensure the Docker socket is mounted (required for `docker run` from within the pod):
```yaml
volumeMounts:
- name: docker-sock
  mountPath: /var/run/docker.sock
volumes:
- name: docker-sock
  hostPath:
    path: /var/run/docker.sock
```

---

### React: Proactive refresh URL (`web/src/lib/store.ts`)

**Current:** `fetch('/api/auth/refresh', ...)` — relative URL hits Vite dev server, triggers silent logout at 12 min.

**Fix in `api.ts`:** Change `const BASE_URL = ...` to `export const BASE_URL = ...`.

**Fix in `store.ts`:** Import `BASE_URL` from `'../lib/api'` and use `` `${BASE_URL}/api/auth/refresh` ``.

---

## Out of Scope

- Alpaca API key rotation (owner's decision; keys are gitignored and not committed)
- Token storage in localStorage vs httpOnly cookie (acceptable for personal research platform)
- Type annotations in `celery_worker.py`
- Filter params allowlist in `ListJobs` (values are parameterized; no injection vector)
- 5 nullable columns always `NULL` in `performance_metrics` (data quality gap, not a crash)
- Security headers (CSP, X-Frame-Options, etc.)
- `go-data` historical endpoint date range / symbol limits
- `clearAuth` without server-side logout
- `python-service.yaml` namespace (currently `default` — correct; all ATP resources are in `default`)
- Refresh token storage in localStorage
- Concurrent backtest limits per user

---

## Assumptions

- A Sealed Secret named `atp-core-credentials` will be created via `kubernetes/initialize.sh` with all keys listed in the go-app and celery deployment sections
- A Sealed Secret named `jwt-keypair` will be created with `private.pem` and `public.pem` keys
- The K8s cluster can pull the existing `lean-atp:latest` Docker image for LEAN backtest/live containers
- `KAFKA_NODE_IP` is the static IP or DNS name of the K8s node running Kafka, reachable from the Docker host bridge network
- `lean-live-net` Docker bridge does not need iptables egress rules for the initial deployment; isolating by hostname (`--add-host kafka-broker`) is the primary restriction
- The `go-data-service` ClusterIP will be used by celery via `GO_DATA_URL=http://go-data-service:8081`

## Open Questions

None — all decisions resolved.
