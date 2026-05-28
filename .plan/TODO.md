# TODO: Deferred Items

Items explicitly deferred from the 2026-05-27 code quality remediation plan.
Each item notes why it was deferred and the condition under which it should be revisited.

---

## WebSocket status polling optimization
**Location:** `go-app/handlers/stream.go` — `JobStatusStream`
**Issue:** Status and log messages are sent every 2 seconds unconditionally, even when nothing changed. At 100 concurrent WebSocket clients this is ~100 DB queries/sec just for polling.
**Fix when:** Load testing reveals DB connection pool saturation, or concurrent user count exceeds ~50.
**Approach:** Track `lastLogID int` and `lastStatus string` in the polling loop; only write a WebSocket message when either changes.

---

## WebSocket Kafka consumer group cleanup
**Location:** `go-app/handlers/stream.go` — `PortfolioStream`
**Issue:** Each WebSocket connection creates a unique Kafka consumer group (`ws-portfolio-<uuid>`). Groups are not cleaned up from Kafka after disconnect. At scale, this creates many stale consumer groups.
**Fix when:** Kafka consumer group count becomes operationally noisy or affects broker performance.
**Approach:** Consider stateless consumption (assign specific offsets, no group coordination) for WebSocket streaming use case.
