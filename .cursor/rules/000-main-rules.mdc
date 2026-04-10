# Project Context
- Name: Algorithmic Trading Platform (ATP)
- Stack: Go (REST API + WebSocket), Python (Celery + LEAN engine), React + TypeScript (frontend), Kafka (KRaft mode), Redis, PostgreSQL + TimescaleDB, Kubernetes (kops on AWS), ArgoCD, GitHub Actions
- Architecture: Event-driven pipeline where Alpaca market data flows through Kafka into dynamically spawned Celery pods that execute QuantConnect LEAN backtests, with results stored in PostgreSQL and streamed to a React UI via 

NOTE: This project is under development. Some of features in this plan may not be fully implemented. Make sure to check what exists in the codebase before making changes.

## Project Architecture & Directory Map
```
.
├── documentation/          # Technical docs (ARCHITECTURE, DATABASE, DEPLOYMENT, DEVELOPMENT, UI_DESIGN)
├── scripts/                # Deployment and teardown scripts
├── go-app/                 # REST API + WebSocket server (Go)
│   ├── handlers/           # auth.go, strategies.go, jobs.go, stream.go
│   ├── middleware/         # jwt.go
│   ├── models/             # models.go (User, Strategy, Job structs)
│   └── main.go
├── go-data/                # Alpaca WebSocket client -> Kafka producer (Go)
│   └── main.go
├── python/                 # Celery workers + LEAN execution
│   ├── celery_worker.py    # Task definitions
│   ├── lean_adapter.py     # KafkaDataFeed (IDataQueueHandler implementation)
│   ├── results_parser.py   # LEAN JSON -> PostgreSQL
│   ├── strategy_validator.py  # Security scanning
│   ├── strategy.py         # Backtrader strategy with KafkaDataFeed
│   └── consumer_test.py    # Kafka consumer test utility
├── web/                    # React + TypeScript frontend (Vite)
│   └── src/
│       ├── pages/          # Login, Strategies, NewBacktest, Results
│       ├── components/     # results/ (JobHeader, HeroMetrics, EquityCurve, tabs)
│       └── hooks/          # useAuth, useJobStatus, useJobMetrics
├── kubernetes/             # K8s manifests
│   ├── infrastructure/     # Kafka, Redis, PostgreSQL
│   ├── core/               # go-app, go-data, celery-worker
│   ├── secrets/            # Sealed secrets
│   └── argocd/             # ArgoCD applications
├── migrations/             # Database migrations (golang-migrate)
├── research/               # Jupyter notebooks, experiments, data downloads
├── logs/                   # All service logs (ALL logs go here - mandatory)
├── local-kafka-docker-compose.yml  # Local dev: Kafka (KRaft), Redis
└── kops.yaml               # Kubernetes cluster config (AWS, us-east-1)
```

## Data Flow
```
Alpaca API -> go-data -> Kafka (stock_data topic)
                              |
                         Celery Worker (Redis queue)
                              |
                    LEAN Engine + KafkaDataFeed
                              |
                    PostgreSQL + TimescaleDB
                              |
                  go-app REST API / WebSocket
                              |
                         React UI
```

## Key Topics
- `stock_data`: Alpaca bars published by go-data, consumed by KafkaDataFeed
- `portfolio_data`: LEAN results published by workers, consumed by go-app for WebSocket streaming

## API Endpoints (go-app)
- `POST /api/auth/register` - User registration
- `POST /api/auth/login` - JWT authentication
- `POST /api/strategies/upload` - Upload .py file to S3
- `POST /api/jobs/backtest` - Submit backtest job
- `GET /api/jobs/:id/metrics` - Fetch LEAN results
- `WS /api/stream/portfolio/:userId` - Real-time portfolio updates

## Database Tables
- `users` - Auth (email, bcrypt hash)
- `strategies` - Metadata + S3 key for uploaded .py files
- `jobs` - Backtest/live runs (status: queued/running/completed/failed), JSONB config
- `job_logs` - Per-job log lines
- `performance_metrics` - 90+ LEAN output metrics (50+ columns)
- `portfolio_metrics` - TimescaleDB time-series equity curve

## Infrastructure (AWS / Kubernetes)
- Cluster: kops, us-east-1, Cilium CNI
- Namespaces: atp-core, atp-data, atp-db, atp-monitoring, argocd
- Nodes: 1 master (t3.small) + 3 workers (t2.micro) + 3 Kafka (t3.small) + 2 DB (t3.medium)
- Strategy pods: dynamically spawned per job, auto-terminated on completion
- Secrets: Sealed Secrets (Alpaca keys, JWT secret, PostgreSQL creds)
- CI/CD: GitHub Actions -> ECR -> kustomization.yaml update -> ArgoCD auto-sync

## Anti-Patterns & "Never Do This"
- Never write logs outside of `/logs` - all services must log there
- Never use emojis in code, comments, or output
- Never block `os`, `subprocess`, `socket`, `eval` inside strategy execution sandbox (they are already blocked - don't accidentally allow them)
- Never use Zookeeper - Kafka runs KRaft mode only
- Never commit real API keys - use Sealed Secrets or `.env` files (gitignored)
- Never skip the strategy security scan before LEAN execution
- Never deploy directly - all deployments go through ArgoCD (push to Git, ArgoCD syncs)
- Never use Backtrader for new work - LEAN is the chosen engine going forward
- Never use Redux - state management is React Query (server state) + Zustand (client state)

## Git & Workflow Standards
- Branch from main, push feature branches
- Merge to main triggers CI/CD: GitHub Actions -> ECR push -> ArgoCD deploy
- Commit messages: imperative mood, concise, describe "what" and "why"
- PRs should be reviewed before merge (use github MCP to draft PRs)

## Definition of Done (DoD)
- [ ] Code linted and tests passing
- [ ] No secrets in code (use env vars or Sealed Secrets)
- [ ] All logs writing to `/logs`
- [ ] Kubernetes manifests updated if infrastructure changes
- [ ] ArgoCD sync verified after deploy
- [ ] API endpoints return correct status codes and error messages
- [ ] React components handle loading, error, and empty states
- [ ] Database migrations applied (golang-migrate)

## Useful Project Commands

### Local Dev Infrastructure
```bash
docker compose -f local-kafka-docker-compose.yml up -d
# Provides: Zookeeper :2181, Kafka :9092, Redis :6379
```

### Run Development Services
```bash
# Go API
cd go-app && go run .

# Go data publisher (requires Alpaca keys in go-data/.env)
cd go-data && go run main.go

# Python Celery worker
cd python && celery -A celery_worker worker --loglevel=info

# Frontend
cd web && npm install && npm run dev
```

### Local PostgreSQL
```bash
docker run -d -p 5432:5432 \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=atp \
  timescale/timescaledb:latest-pg14

cd migrations && migrate -database "postgres://postgres:password@localhost:5432/atp?sslmode=disable" up
```

### Kafka Utilities
```bash
# Test consumer
docker exec -it <kafka-container> /bin/sh
kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic stock_data --from-beginning

# List topics
kafka-topics.sh --bootstrap-server localhost:9092 --list
```

### Kubernetes
```bash
kops validate cluster
kubectl get pods -A
kubectl logs -f deployment/go-app -n atp-core
```

### Deploy / Teardown
```bash
./scripts/deploy-cluster.sh
./scripts/teardown-cluster.sh
```

## Current Status
- Working: Alpaca data ingestion, Kafka pipeline, Celery job queue, basic Backtrader strategies
- In progress: Migrating to LEAN engine (8-week MVP roadmap)
- MVP priorities (P0): PostgreSQL + TimescaleDB setup, JWT auth, LEAN integration, results parser (90+ metrics)
- MVP priorities (P1): Strategy upload (S3), backtest job submission, results dashboard, equity curve chart
