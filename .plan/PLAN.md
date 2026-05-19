# Plan: Algorithmic Trading Platform
> Generated: 2026-05-15
> Type: brownfield
> Documents: [PRD.md](PRD.md) | [SYSTEM-DESIGN.md](SYSTEM-DESIGN.md) | [UI-SPEC.md](UI-SPEC.md)

## Overview
**What:** A multi-user paper trading and backtesting platform powered by the QuantConnect LEAN engine. Users upload Python LEAN strategies, run backtests against historical Alpaca data (or uploaded CSV), run live paper trading against a real-time Alpaca feed, and view comprehensive results dashboards with 90+ performance metrics.

**Why:** Personal research platform for developing and evaluating algorithmic trading strategies in a production-grade, isolated environment without risking real capital.

**Who:** Quantitative traders and researchers (multi-user, fully isolated per-user).

## Current State vs Target
The codebase is in a very early brownfield state. Nearly everything needs to be rewritten:
- `go-app`: ~120 lines, no auth, no REST API → needs full rewrite
- `python/celery_worker.py`: Backtrader-based, single task → replace with LEAN orchestration
- `web/`: Single Chart.js page, no routing → replace with full React app
- No `migrations/` directory, no `lean-plugin/` directory

Existing assets to preserve:
- `go-data/main.go`: Alpaca WebSocket → Kafka producer (keep, extend with HTTP historical data endpoint and market_data DB writes)
- `local-kafka-docker-compose.yml`: Local dev Kafka (keep)
- `mockups/`: HTML mockups as design reference (keep, extend)
- `documentation/`: Architecture and schema docs (keep as reference, update after implementation)
- `kubernetes/`: K8s manifests (keep, update as services change)

## Definition of Done
- [ ] Users can register, log in, and have their data fully isolated from other users
- [ ] Users can upload a Python LEAN strategy (.py) — AST scan rejects dangerous imports
- [ ] Versioning: re-uploading a strategy creates a new version; all versions are retained
- [ ] Backtest flow: click "Run Backtest" on a strategy → job queued → go-data fills market_data cache → Celery materializes CSV → LEAN executes → results stored → user sees equity curve + 90+ metrics in the dashboard
- [ ] Live paper trading flow: click "Go Live" → warmup CSVs materialized from market_data → LEAN runs in live mode consuming real-time Kafka data → user sees real-time portfolio updates via WebSocket
- [ ] CSV upload: user can upload an OHLCV CSV as the data source for a backtest
- [ ] Job cancellation: running backtest or live jobs can be stopped from the UI
- [ ] Job timeout: backtests auto-terminate after configured max time
- [ ] Live jobs on server restart: marked as failed, user must manually restart
- [ ] Strategy deletion: cascade-deletes all versions, jobs, and results
- [ ] All UI pages render correct loading, error, and empty states
- [ ] All services write structured logs to `/logs`
- [ ] No secrets committed — Sealed Secrets or env vars used everywhere
- [ ] Database migrations apply cleanly via golang-migrate
- [ ] Kubernetes manifests updated, ArgoCD sync verified after deploy
- [ ] Unit tests for: AST scanner, results parser, JWT middleware, API handlers
- [ ] Integration tests for: full backtest pipeline, auth flow, job queue
- [ ] E2E tests for: register → upload → run backtest → view results; go live → see updates → stop

## Out of Scope
- Real broker order execution (Alpaca is data-only in this project)
- Prometheus / Grafana monitoring (basic `/logs` logging only)
- Email notifications (UI WebSocket polling only)
- Account management: change password, delete account
- Strategy optimization / parameter sweeps
- Portfolio comparison across multiple strategies
- Redux (use React Query + Zustand)
- Backtrader (fully replaced by LEAN)
- Zookeeper (Kafka runs KRaft only)
- Kubernetes operator for LEAN pods (Docker run via subprocess from Celery)

## Assumptions
- Alpaca API credentials are available and stored as Sealed Secrets
- AWS infrastructure (ECR, S3, kops cluster) can be created following existing documentation
- The LEAN Docker image (`quantconnect/lean:latest`) is accessible from the cluster
- The C# KafkaDataFeed plugin compiles successfully against LEAN's public interfaces
- Alpaca's free tier provides sufficient historical bar data for the intended backtest date ranges
- LEAN's Python engine supports `QCAlgorithm` subclasses without C# interop for algorithm logic
- The `market_data` TimescaleDB cache is sufficient as the single source of truth for historical bars; Kafka is not needed in the backtest data path
- LEAN's `SubscriptionDataReaderHistoryProvider` will read the materialized warmup CSV files before the KafkaDataQueueHandler takes over in live mode

## Open Questions

### OQ-1: Symbol and date range specification — RESOLVED (Option A)
User enters symbols (comma-separated tickers), start date, end date, and resolution
explicitly in the Run Backtest / Go Live modal. These values are passed in the
`POST /api/jobs` request body and used by Celery to fetch and materialize data.
The user is responsible for ensuring these match what their strategy code expects.

### OQ-2: Maximum historical data volume (Alpaca free tier)
Alpaca's free tier rate limits and maximum historical bar history depth are not
confirmed. This affects the practical backtest date range limit and whether
multi-year backtests are feasible without a paid Alpaca plan.
