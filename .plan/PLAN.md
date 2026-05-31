# Plan: ATP Wave 6 — Review Blocker Resolution
> Generated: 2026-05-31
> Type: brownfield
> Documents: single file
> Archived: [PLANv5.md](archive/PLANv5.md) (Wave 5 — S3 secretKeyRef, Dockerfile non-root, XFF removal, strategy validator blocklists, CancelJob queued branch, KAFKA_NODE_IP removal)

## Overview
**What:** Fix 9 bugs (4 BLOCKERs, 3 HIGH, 2 MEDIUM) surfaced by the Wave 5 post-implementation code review. No new features; no schema changes. Every change is a targeted correction to code already on `switch-to-lean`.

**Why:** The Wave 5 implementation introduced one new bug (RemoteAddr-only rate limiting breaks behind K8s LB with SNAT) and left nine issues unaddressed: a financial metric sign-corruption bug, two WebSocket authorization defects, an immediate-cancel status mismatch, a strategy validator bypass vector, a Kubernetes job double-cleanup on exception, a stale test fixture, and missing input validation in lean_runner itself.

**Who:** Internal — no user-visible behavior changes except CancelJob now returns `"status": "failed"` (not `"cancelling"`) for queued jobs that are cancelled synchronously.

---

## Definition of Done

### A. BLOCKERs

- [ ] `_strip_currency("$-500.00")` returns `-500.0`; `_strip_currency("-500.00")` returns `-500.0`; `_strip_currency("$500.00")` returns `500.0`; `python3 -m pytest python/test_celery_worker.py -k test_strip_currency -v` passes
- [ ] WebSocket connect to `/api/stream/portfolio/:jobId` when the DB query times out → WebSocket close with code `1011` (not `1008`) and message `"server error"`; `go test ./go-app/handlers/ -run TestPortfolioStream_DBError -v` passes
- [ ] `POST /api/jobs/<queued-job-id>/cancel` → 202 `{"jobId":"...","status":"failed","message":"cancelled by user"}`; subsequent `GET /api/jobs/<id>` → `status: "failed"`; `go test ./go-app/handlers/ -run TestCancelJob_QueuedResponse -v` passes
- [ ] With 5 concurrent WebSocket requests using valid JWTs but a foreign job ID: each returns close 1008 `"forbidden"`; a 6th request for the user's own job opens successfully (semaphore not exhausted); `go test ./go-app/handlers/ -run TestPortfolioStream_SemaphoreNotConsumedOnAuthFail -v` passes

### B. HIGH

- [ ] `validate_strategy("class Base(QCAlgorithm): pass\nclass Real(QCAlgorithm): pass")` → `{"valid": true, "class_name": "Real"}` (last class wins); `python3 -m pytest python/test_strategy_validator.py -k test_multiple_qc_classes -v` passes
- [ ] `run_lean_live_task` with `stop_lean_live` mocked to raise `RuntimeError`: `stop_lean_live` is called exactly **once** (not twice); `python3 -m pytest python/test_celery_worker.py -k test_stop_lean_live_called_once_on_exception -v` passes
- [ ] `grep "KAFKA_NODE_IP" python/conftest.py` → 0 lines; `python3 -m pytest python/ -v` passes (no fixture KeyError)

### C. MEDIUM

- [ ] `clientIP` returns the rightmost `X-Forwarded-For` value when the header is present and contains a valid IP; falls back to `RemoteAddr` if header is absent or contains an invalid IP; `go test ./go-app/handlers/ -run TestClientIP -v` passes
- [ ] `run_lean_backtest("not-a-uuid", "/tmp")` raises `ValueError("invalid job_id format")`; `run_lean_live("", "/tmp")` raises `ValueError("invalid job_id format")`; `python3 -m pytest python/test_lean_runner.py -k test_invalid_job_id -v` passes

---

## Unchanged Behavior

- WHEN a user submits a valid backtest job THEN the response SHALL continue to be `202 Accepted` with `{"jobId": "<uuid>"}`
- WHEN a running job is cancelled THEN `POST /api/jobs/<id>/cancel` SHALL continue to return `{"status": "cancelling"}`
- WHEN a backtest or live job completes THEN `performance_metrics` and `portfolio_metrics` rows SHALL continue to be inserted
- WHEN a WebSocket client provides a valid JWT for their own job THEN streaming SHALL continue to work
- WHEN `validate_strategy` receives a file with exactly one `QCAlgorithm` subclass THEN it SHALL continue to return `{"valid": true, "class_name": "<name>"}`

---

## Fixes by Area

### A. BLOCKERs

#### A1. `celery_worker.py` — Fix `_strip_currency` sign detection

The current implementation runs `negative = s.lstrip("-") != s` before stripping the `$` prefix. For `"$-500.00"`, `lstrip("-")` returns the string unchanged (it starts with `$`, not `-`), so `negative` is `False` and the result is `+500.0`.

Fix: strip the currency prefix and whitespace first, then detect the sign:

```python
def _strip_currency(v) -> float:
    s = str(v)
    stripped = s.replace("$", "").replace(",", "").strip()
    negative = stripped.startswith("-")
    cleaned = stripped.lstrip("+-")
    try:
        result = float(cleaned) if cleaned else 0.0
    except ValueError:
        result = 0.0
    return -result if negative else result
```

Add test `test_strip_currency` to `test_celery_worker.py` covering:
- `"$-500.00"` → `-500.0`
- `"-500.00"` → `-500.0`
- `"$500.00"` → `500.0`
- `"500.00"` → `500.0`
- `"$0.00"` → `0.0`
- `""` → `0.0`
- `"N/A"` → `0.0`

#### A2. `stream.go` — Separate DB error from ownership mismatch (PortfolioStream)

The condition `if err != nil || ownerID != userID` sends close code 1008 (PolicyViolation / "forbidden") for both a DB timeout and an actual ownership mismatch. A legitimate user receives an auth-error close code on transient DB failures.

Fix: split into two separate checks with appropriate close codes:

```go
if err != nil {
    log.Printf("PortfolioStream: ownership query failed for job %s: %v", jobID, err)
    conn.WriteMessage(websocket.CloseMessage, //nolint:errcheck
        websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "server error"))
    return
}
if ownerID != userID {
    conn.WriteMessage(websocket.CloseMessage, //nolint:errcheck
        websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "forbidden"))
    return
}
```

Add test `TestPortfolioStream_DBError` to `stream_test.go`: inject a DB error on the ownership query; assert close code is `1011` (not `1008`).

#### A3. `jobs.go` — Return correct status for synchronously-cancelled queued jobs

For queued jobs, the handler immediately sets `status = 'failed'` in the DB but still returns `{"status": "cancelling"}`. The WebSocket then immediately broadcasts `"failed"`, creating a state-machine inconsistency for any client expecting `queued → cancelling → cancelled`.

Fix: return the actual final status for the queued branch:

```go
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
    writeJSON(w, http.StatusAccepted, map[string]string{
        "jobId":    jobID,
        "status":   "failed",
        "message":  "cancelled by user",
    })
    return
}
// running branch (unchanged):
if err := queue.SetStopSignal(jobID); err != nil {
    ...
}
writeJSON(w, http.StatusAccepted, map[string]string{
    "jobId":  jobID,
    "status": "cancelling",
})
```

Add test `TestCancelJob_QueuedResponse` to `jobs_test.go`: queued job cancel → 202 with `"status": "failed"` and `"message": "cancelled by user"`; subsequent DB state is `status = failed`.

#### A4. `stream.go` — Move semaphore check after ownership verification (PortfolioStream)

The per-user connection semaphore is incremented before the ownership DB query. With `maxWSPerUser = 5`, an attacker with a valid JWT can make 5 concurrent requests to a foreign job ID; each increments the counter, the DB query runs, and only then returns 1008. During the DB query window, a legitimate request from the same user would see count `> 5` and be rejected — even though `defer` will eventually decrement all 5.

Fix: move the semaphore load/store/increment block to immediately after the ownership check passes:

```go
// 1. First message auth (unchanged)
userID, err := wsFirstMessageAuth(conn)
if err != nil { return }

// 2. Verify job ownership BEFORE touching the semaphore
var ownerID string
err = db.Pool.QueryRow(r.Context(), `
    SELECT s.user_id FROM jobs j
    JOIN strategy_versions sv ON j.strategy_version_id = sv.id
    JOIN strategies s ON sv.strategy_id = s.id
    WHERE j.id = $1
`, jobID).Scan(&ownerID)
if err != nil {
    // (same error handling as A2)
}
if ownerID != userID {
    conn.WriteMessage(...)
    return
}

// 3. Now check per-user connection limit
counterVal, _ := wsSemaphore.LoadOrStore(userID, new(int32))
count := atomic.AddInt32(counterVal.(*int32), 1)
defer atomic.AddInt32(counterVal.(*int32), -1)
if count > maxWSPerUser {
    conn.WriteMessage(...)
    return
}
```

Add test `TestPortfolioStream_SemaphoreNotConsumedOnAuthFail` to `stream_test.go`: simulate 5 concurrent unauthorized requests (valid JWT, foreign job ID); then assert a 6th request for the user's own job is accepted without hitting the "too many connections" path.

---

### B. HIGH

#### B1. `strategy_validator.py` — Collect all QCAlgorithm subclasses; reject multiple

The current `ast.walk` loop returns on the first `QCAlgorithm` subclass found. An adversary can prepend a no-op `class Decoy(QCAlgorithm): pass` before the real algorithm; the validator returns `class_name = "Decoy"` and LEAN runs the empty stub.

Fix: collect all matches, then return the **last** one. A file with multiple QCAlgorithm subclasses is valid (e.g., a base class + concrete subclass); the last subclass in source order is the entry point.

```python
qc_classes = []
for node in ast.walk(tree):
    if isinstance(node, ast.ClassDef):
        for base in node.bases:
            if (isinstance(base, ast.Name) and base.id == "QCAlgorithm") or \
               (isinstance(base, ast.Attribute) and base.attr == "QCAlgorithm"):
                qc_classes.append(node.name)

if not qc_classes:
    return {"valid": False, "violation": "no QCAlgorithm subclass found"}
return {"valid": True, "class_name": qc_classes[-1]}
```

Add test `test_multiple_qc_classes` to `test_strategy_validator.py`:
- Source with two QCAlgorithm subclasses → `valid == True`, `class_name` is the **last** one
- Source with one QCAlgorithm subclass → `valid == True`, `class_name` is correct (unchanged behavior)

#### B2. `celery_worker.py` — Prevent double `stop_lean_live` call on exception

In `run_lean_live_task`, `stop_lean_live(job_name, job_dir)` is called in the normal try-block path. If it raises, the outer `except` handler also calls `stop_lean_live` (the `if job_name:` guard is always True at that point). The second call may hit a 404 if Kubernetes already deleted the job, corrupting the error state.

Fix: add an `_attempted_stop` flag set immediately before the try-block call:

```python
_attempted_stop = False
try:
    ...  # monitoring loop
    r.close()

    _attempted_stop = True
    final_path = stop_lean_live(job_name, job_dir)
    if final_path:
        with open(final_path) as f:
            results_json = json.load(f)
        _store_results(conn, job_id, results_json)

    _update_job_status(conn, job_id, "completed")

except Exception as e:
    _update_job_status(conn, job_id, "failed", _sanitize_error(str(e)))
    log.error(f"Live job failed: {e}")
    if job_name and not _attempted_stop:
        try:
            stop_lean_live(job_name, job_dir)
        except Exception as stop_err:
            log.error("Failed to stop job %s: %s", job_name, stop_err)
```

Note: `_attempted_stop = True` is placed BEFORE the call. This means if `stop_lean_live` raises, the flag is already set and the except block will not retry.

Add test `test_stop_lean_live_called_once_on_exception` to `test_celery_worker.py`: mock `stop_lean_live` to raise `RuntimeError` on first call; run a live task that reaches the stop point; assert `stop_lean_live` was called exactly once.

#### B3. `conftest.py` — Remove stale `KAFKA_NODE_IP` fixture

Wave 5 T-13 removed `KAFKA_NODE_IP` from `lean_runner.py`. The `conftest.py` session fixture still sets `os.environ.setdefault("KAFKA_NODE_IP", "localhost")`, which is now dead code. Any test that was supposed to verify the Kafka broker address in LEAN config will silently pass against the hardcoded `"kafka:9092"` string regardless.

Fix: remove line 27 from `conftest.py`:
```python
# Delete this line:
os.environ.setdefault("KAFKA_NODE_IP", "localhost")
```

No other changes — LEAN's K8s Job spec hardcodes `"kafka:9092"` for `KAFKA_BOOTSTRAP_SERVERS` by design (the cluster Kafka service name). The stale fixture was making it look like the address was test-configurable when it is not.

---

### C. MEDIUM

#### C1. `auth.go` + `kubernetes/go-app/service.yaml` — Restore XFF IP extraction; fix service type

Two changes:

**1. `auth.go` — Restore rightmost-XFF extraction in `clientIP`:**

Wave 5 T-02 replaced `clientIP` with RemoteAddr-only. The confirmed production path is `Client → AWS ALB → K8s node (SNAT) → go-app pod`. SNAT rewrites the source IP to the node's internal IP, so `r.RemoteAddr` is never the client's real IP in production. The AWS ALB appends the real client IP as the rightmost `X-Forwarded-For` value — this is authoritative for a single-hop topology.

Fix: restore XFF extraction with an added validity guard:

```go
func clientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        parts := strings.Split(xff, ",")
        ip := strings.TrimSpace(parts[len(parts)-1])
        if net.ParseIP(ip) != nil {
            return ip
        }
    }
    ip, _, err := net.SplitHostPort(r.RemoteAddr)
    if err != nil {
        return r.RemoteAddr
    }
    return ip
}
```

Restore the `strings` import (removed in Wave 5 T-02). Update `auth_test.go`:
- Add `TestClientIP_XFFPresent`: XFF `"203.0.113.1"` → returns `"203.0.113.1"`
- Add `TestClientIP_XFFMultiHop`: XFF `"10.0.0.1, 203.0.113.1"` → returns `"203.0.113.1"` (rightmost)
- Add `TestClientIP_XFFInvalid`: XFF `"not-an-ip"` → falls back to RemoteAddr host
- Add `TestClientIP_XFFAbsent`: no XFF header → returns RemoteAddr host
- Remove `TestClientIP_UsesRemoteAddr` (the XFF-absent case above replaces it)

**2. `kubernetes/go-app/service.yaml` — Change type from ClusterIP to LoadBalancer:**

The current manifest defines the service as `ClusterIP` (internal-only), but the deployment documentation describes an AWS ALB fronting go-app. Update:

```yaml
spec:
  type: LoadBalancer
  ports:
  - port: 8080
    targetPort: 8080
  selector:
    app: go-app
```

No other changes. ArgoCD reconciles and provisions the AWS ELB on next sync.

#### C2. `lean_runner.py` — Add UUID validation to `run_lean_backtest` and `run_lean_live`

`celery_worker.py` validates `job_id` before calling `lean_runner` functions, but `lean_runner.py` can be called directly (e.g., from tests or future callers). Both `run_lean_backtest` (line 187) and `run_lean_live` (line 245) build K8s job names with `job_id[:8]` without their own validation, producing names like `lean-backtest--<timestamp>` or names with K8s-invalid characters on bad input.

Fix: add module-level `_UUID_RE` and validate at the top of both functions.

```python
import re  # add to imports

_UUID_RE = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
)

def run_lean_backtest(job_id, job_dir, timeout_seconds=300):
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    ...

def run_lean_live(job_id, job_dir):
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    ...
```

Add test `test_invalid_job_id` to `test_lean_runner.py` covering both functions:
- `run_lean_backtest("bad", "/tmp")` → `ValueError` with "invalid job_id format"
- `run_lean_live("", "/tmp")` → `ValueError` with "invalid job_id format"
- `run_lean_backtest("not-a-uuid", "/tmp")` → `ValueError`

---

## Out of Scope

- `KAFKA_BOOTSTRAP_SERVERS` hardcoded to `"kafka:9092"` in `lean_runner.py:123` — this is intentional (cluster service name); making it configurable is a future infrastructure task
- Trusted-proxy count configuration (e.g., `TRUSTED_PROXY_COUNT` env var) — the current XFF rightmost approach is correct for single-hop AWS ALB; multi-hop proxy support is Wave 7
- `SameSite=Strict` on refresh cookie — explicitly out of scope since Wave 4
- `K8S_NAMESPACE=atp-jobs` isolation for LEAN pods — Wave 7
- Python type annotations, f-strings in logging, gVisor — Wave 7
- `go-data` service changes

---

## Assumptions

- AWS ALB appends the real client IP as the rightmost `X-Forwarded-For` value. If the deployment later moves to a multi-hop proxy topology, `clientIP` must be revisited.
- `_attempted_stop` flag approach in `run_lean_live_task` does not change the behavior for the case where `stop_lean_live` is never reached (e.g., exception before the monitoring loop exits) — `_attempted_stop` remains `False` and the except block still calls stop. This is correct.
- `from pathlib import Path` in `lean_runner.py` is used at line 61 (`job_dir_path = Path(job_dir)`); it is NOT removed.

## Open Questions

None — all decisions resolved during planning.
