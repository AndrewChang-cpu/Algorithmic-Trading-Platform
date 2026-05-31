# Tasks: ATP Wave 6 — Review Blocker Resolution
> Generated: 2026-05-31
> Source: .plan/
> Total: 8 tasks | Starting points: 7

## Dependency Graph

```
T-01 · celery_worker.py — _strip_currency fix + test
└── T-05 · celery_worker.py — stop_lean_live _attempted_stop + test

T-02 · stream.go — DB error path + semaphore ordering + tests

T-03 · jobs.go — CancelJob queued response + test

T-04 · strategy_validator.py — QCAlgorithm last-class scan + test

T-06 · conftest.py — Remove stale KAFKA_NODE_IP

T-07 · auth.go + service.yaml — Restore XFF; fix service type + tests

T-08 · lean_runner.py — UUID validation in backtest + live + test
```

---

## Tasks

### T-01 · `celery_worker.py` — Fix `_strip_currency` sign detection + test
**Status:** `done`
**Depends on:** none
**Files:** `python/celery_worker.py`, `python/test_celery_worker.py`
**What:** Replace the `_strip_currency` function body. The current implementation runs `negative = s.lstrip("-") != s` before stripping the `$` prefix; for `"$-500.00"` this evaluates `negative = False` and returns `+500.0`. Fix by stripping currency prefix first, then detecting sign:

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

Add `test_strip_currency` to `test_celery_worker.py` as a parametrized test covering these input/output pairs:
- `("$-500.00", -500.0)` — currency prefix before sign
- `("-500.00", -500.0)` — sign only
- `("$500.00", 500.0)` — positive with prefix
- `("500.00", 500.0)` — plain positive
- `("$0.00", 0.0)` — zero
- `("", 0.0)` — empty string
- `("N/A", 0.0)` — non-numeric

**Done when:** `python3 -m pytest python/test_celery_worker.py -k test_strip_currency -v` passes (7 parametrized cases); `_strip_currency("$-500.00")` returns `-500.0` in the REPL

---

### T-02 · `stream.go` — Fix DB error path + semaphore ordering in `PortfolioStream` + tests
**Status:** `done`
**Depends on:** none
**Files:** `go-app/handlers/stream.go`, `go-app/handlers/stream_test.go`
**What:** Two changes to `PortfolioStream` applied together (they overlap in the same code block).

**Fix 1 — Separate DB error from ownership mismatch (~line 116):**
Replace `if err != nil || ownerID != userID { /* 1008 forbidden */ }` with two separate checks:
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

**Fix 2 — Move semaphore to after ownership check (~line 213):**
Move the entire semaphore block (`wsSemaphore.LoadOrStore` + `atomic.AddInt32` + `defer` + `count > maxWSPerUser` check) to immediately after the two ownership check blocks above. Unauthorized requests must not consume a semaphore slot before the auth DB query completes.

Final order in `PortfolioStream`:
1. `wsFirstMessageAuth` (unchanged)
2. Ownership DB query → DB-error path (close 1011) → mismatch path (close 1008) [Fix 1]
3. Semaphore load/increment/defer/limit check [Fix 2, moved here]
4. `ctx`/`cancel`, disconnect pump, ticker, polling loop (unchanged)

Add to `stream_test.go`:
- `TestPortfolioStream_DBError`: inject a DB error on the ownership `QueryRow`; assert WebSocket close code is `1011`, message is `"server error"` (not `"forbidden"`)
- `TestPortfolioStream_SemaphoreNotConsumedOnAuthFail`: make 5 requests with a foreign job ID (valid JWT, ownership check returns mismatch); then make a 6th request for the user's own job; assert the 6th request is accepted (not rejected with "too many connections")

**Done when:** `go test ./go-app/handlers/ -run "TestPortfolioStream_DBError|TestPortfolioStream_SemaphoreNotConsumedOnAuthFail" -v` passes; `go test ./go-app/handlers/ -v` all pass

---

### T-03 · `jobs.go` — CancelJob: correct response for queued cancellations + test
**Status:** `done`
**Depends on:** none
**Files:** `go-app/handlers/jobs.go`, `go-app/handlers/jobs_test.go`
**What:** In `CancelJob`, the queued branch marks the DB row `status='failed'` then falls through to `writeJSON(... "status": "cancelling")`. Add a `return` after the queued branch writes its own response:

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
        "jobId":   jobID,
        "status":  "failed",
        "message": "cancelled by user",
    })
    return   // ← add this; prevents falling through to "cancelling" response
}
// running branch unchanged
```

Add `TestCancelJob_QueuedResponse` to `jobs_test.go`: cancel a job in `queued` state → 202 with body containing `"status":"failed"` and `"message":"cancelled by user"`; DB row has `status = 'failed'` and `error_message = 'cancelled by user'`.

**Done when:** `go test ./go-app/handlers/ -run TestCancelJob_QueuedResponse -v` passes; `go test ./go-app/handlers/ -v` all pass

---

### T-04 · `strategy_validator.py` — Return last QCAlgorithm subclass + test
**Status:** `done`
**Depends on:** none
**Files:** `python/strategy_validator.py`, `python/test_strategy_validator.py`
**What:** Replace the final `ast.walk` loop (which returns on the first `QCAlgorithm` subclass) with a collect-all approach that returns the **last** match. Files with multiple subclasses are valid (base class + concrete subclass); the last one in source order is the entry point.

Replace the existing loop and its `return` statements with:
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

Add `test_multiple_qc_classes` to `test_strategy_validator.py`:
- Source `"class Base(QCAlgorithm): pass\nclass Real(QCAlgorithm): pass"` → `valid == True`, `class_name == "Real"` (last class)
- Source `"class MyAlgo(QCAlgorithm): pass"` → `valid == True`, `class_name == "MyAlgo"` (single-class unchanged behavior)

**Done when:** `python3 -m pytest python/test_strategy_validator.py -k test_multiple_qc_classes -v` passes; `python3 -m pytest python/test_strategy_validator.py -v` all pass

---

### T-05 · `celery_worker.py` — Fix `stop_lean_live` double-call via `_attempted_stop` flag + test
**Status:** `done`
**Depends on:** T-01
**Files:** `python/celery_worker.py`, `python/test_celery_worker.py`
**What:** In `run_lean_live_task`, if `stop_lean_live(job_name, job_dir)` raises in the normal try-block path, the outer `except` handler's `if job_name:` guard is still True and calls it again. Add `_attempted_stop = False` before the try block; set `_attempted_stop = True` immediately **before** calling `stop_lean_live`; gate the except-block call on `not _attempted_stop`:

```python
_attempted_stop = False
try:
    # ... warmup, monitoring loop, r.close() ...

    _attempted_stop = True
    final_path = stop_lean_live(job_name, job_dir)
    if final_path:
        with open(final_path) as f:
            results_json = json.load(f)
        _store_results(conn, job_id, results_json)

    _update_job_status(conn, job_id, "completed")
    log.info("Live job completed")

except Exception as e:
    _update_job_status(conn, job_id, "failed", _sanitize_error(str(e)))
    log.error(f"Live job failed: {e}")
    if job_name and not _attempted_stop:
        try:
            stop_lean_live(job_name, job_dir)
        except Exception as stop_err:
            log.error("Failed to stop job %s: %s", job_name, stop_err)
finally:
    # ... producer.flush, conn.close, shutil.rmtree (unchanged) ...
```

The flag is set BEFORE the call so that even if `stop_lean_live` raises, the except block will not retry.

Add `test_stop_lean_live_called_once_on_exception` to `test_celery_worker.py`:
- Mock `stop_lean_live` to raise `RuntimeError("k8s 404")` on any call
- Set up a live task run that reaches the stop point (monitoring loop exits normally)
- Assert `stop_lean_live` mock was called exactly **once**
- Assert the job status was set to `"failed"`

**Done when:** `python3 -m pytest python/test_celery_worker.py -k test_stop_lean_live_called_once_on_exception -v` passes

---

### T-06 · `conftest.py` — Remove stale `KAFKA_NODE_IP` fixture line
**Status:** `done`
**Depends on:** none
**Files:** `python/conftest.py`
**What:** Delete the line `os.environ.setdefault("KAFKA_NODE_IP", "localhost")` from the `_set_celery_env` session fixture (currently line 27). Wave 5 T-13 removed `KAFKA_NODE_IP` from `lean_runner.py`; nothing in the codebase reads this variable anymore. The stale fixture makes it appear that tests validate the Kafka broker address when they do not.

No other changes to `conftest.py`.

**Done when:** `grep "KAFKA_NODE_IP" python/conftest.py` → 0 lines; `python3 -m pytest python/ -v` exits 0

---

### T-07 · `auth.go` + `service.yaml` — Restore XFF IP extraction; fix service type + tests
**Status:** `done`
**Depends on:** none
**Files:** `go-app/handlers/auth.go`, `go-app/handlers/auth_test.go`, `kubernetes/go-app/service.yaml`
**What:** Two changes.

**1. `auth.go` — Restore rightmost-XFF in `clientIP`:**

Wave 5 T-02 replaced `clientIP` with RemoteAddr-only. Production path is `Client → AWS ALB → K8s node (SNAT) → go-app pod`; SNAT means `r.RemoteAddr` is the node's internal IP. AWS ALB appends the real client IP as the rightmost `X-Forwarded-For` value. Restore XFF extraction with a validity guard:

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
- Add `TestClientIP_XFFPresent`: request with `X-Forwarded-For: 203.0.113.1` → `clientIP` returns `"203.0.113.1"`
- Add `TestClientIP_XFFMultiHop`: `X-Forwarded-For: 10.0.0.1, 203.0.113.1` → returns `"203.0.113.1"` (rightmost)
- Add `TestClientIP_XFFInvalid`: `X-Forwarded-For: not-an-ip` → falls back to RemoteAddr host
- Add `TestClientIP_XFFAbsent`: no `X-Forwarded-For` header → returns RemoteAddr host
- Remove `TestClientIP_UsesRemoteAddr` (added in Wave 5; the `XFFAbsent` case above is its replacement)

**2. `kubernetes/go-app/service.yaml` — Change ClusterIP → LoadBalancer:**

Update `spec.type` from `ClusterIP` to `LoadBalancer`. Leave `ports` and `selector` unchanged:
```yaml
spec:
  type: LoadBalancer
  ports:
  - port: 8080
    targetPort: 8080
  selector:
    app: go-app
```

**Done when:** `grep "X-Forwarded-For" go-app/handlers/auth.go` → 1 line; `grep "type: LoadBalancer" kubernetes/go-app/service.yaml` → 1 line; `go test ./go-app/handlers/ -run TestClientIP -v` passes (4 cases: Present, MultiHop, Invalid, Absent); `go test ./go-app/...` passes

---

### T-08 · `lean_runner.py` — UUID validation in `run_lean_backtest` + `run_lean_live` + test
**Status:** `done`
**Depends on:** none
**Files:** `python/lean_runner.py`, `python/test_lean_runner.py`
**What:** Both `run_lean_backtest` (line 187) and `run_lean_live` (line 245) slice `job_id[:8]` to build K8s job names with no UUID check in `lean_runner.py` itself. A malformed `job_id` produces a K8s-invalid name and a generic `RuntimeError`.

Add `import re` to `lean_runner.py` imports and a module-level `_UUID_RE` pattern. Add a validation guard at the top of both functions before any K8s or S3 interaction:

```python
import re   # add alongside existing stdlib imports

_UUID_RE = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
)

def run_lean_backtest(job_id, job_dir, timeout_seconds=300):
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    # ... existing body unchanged ...

def run_lean_live(job_id, job_dir):
    if not _UUID_RE.match(job_id):
        raise ValueError(f"invalid job_id format: {job_id!r}")
    # ... existing body unchanged ...
```

Add `test_invalid_job_id` to `test_lean_runner.py` as a parametrized test. For each case, mock the K8s `BatchV1Api` and `CoreV1Api` with `side_effect=AssertionError` to confirm they are never reached:
- `run_lean_backtest("bad", "/tmp")` → `ValueError` containing `"invalid job_id format"`
- `run_lean_live("", "/tmp")` → `ValueError` containing `"invalid job_id format"`
- `run_lean_backtest("not-a-uuid-at-all", "/tmp")` → `ValueError`

**Done when:** `python3 -m pytest python/test_lean_runner.py -k test_invalid_job_id -v` passes (3 parametrized cases); `grep "_UUID_RE" python/lean_runner.py` → 1 line

---

## Open Questions

None — all ambiguities resolved during planning.
