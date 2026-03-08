# Development Roadmap

## 8-Week Timeline to MVP

### Week 1-2: Infrastructure & Authentication

#### PostgreSQL Setup
- [ ] Kubernetes StatefulSet with Patroni (HA) + TimescaleDB extension
- [ ] Database migrations (golang-migrate)
- [ ] Schema: users, strategies, jobs, job_logs, portfolio_metrics, performance_metrics
- [ ] EBS volumes for persistent storage (100GB gp3)

#### Authentication Service
- [ ] go-app: JWT endpoints (`/api/auth/register`, `/api/auth/login`)
- [ ] Password hashing (bcrypt)
- [ ] JWT middleware for protected routes
- [ ] React: Login/register pages

#### Sealed Secrets
- [ ] Create Alpaca credentials sealed secret
- [ ] Create JWT secret sealed secret
- [ ] Update go-data deployment to use sealed secrets

**Deliverable**: Users can register, login, and JWT auth works end-to-end

---

### Week 3-4: LEAN Engine Integration

#### LEAN Docker Image
- [ ] Dockerfile: `quantconnect/lean:latest` + confluent-kafka-python
- [ ] Python: `KafkaDataFeed(IDataQueueHandler)` implementation
- [ ] Subscribe to Kafka `stock_data` topic
- [ ] Convert Alpaca JSON → LEAN Bar objects
- [ ] Test with sample Buy & Hold strategy

#### Celery Task
- [ ] `run_lean_backtest(strategy_id, config)` task
- [ ] Download strategy from S3
- [ ] Generate LEAN project structure (`config.json`, `Main.py`)
- [ ] Execute LEAN in Docker container
- [ ] Parse results JSON

#### Results Parser
- [ ] Extract 90+ metrics from LEAN JSON
- [ ] Write to PostgreSQL `performance_metrics` table (50+ columns)
- [ ] Write equity curve to TimescaleDB `portfolio_metrics`
- [ ] Handle errors (LEAN runtime failures)

**Deliverable**: Celery worker can execute LEAN backtest and store results

---

### Week 5-6: Backend API & S3 Integration

#### Strategy Management
- [ ] S3 bucket creation (AWS CLI or manual)
- [ ] go-app: `POST /api/strategies/upload` endpoint
  - [ ] Validate .py file (syntax check, security scan)
  - [ ] Upload to S3 `s3://atp-strategies/{user_id}/{strategy_id}.py`
  - [ ] Insert record into `strategies` table
- [ ] go-app: `GET /api/strategies` (list user's strategies)
- [ ] go-app: `DELETE /api/strategies/:id`

#### Job Management
- [ ] go-app: `POST /api/jobs/backtest` endpoint
  - [ ] Validate config (dates, cash, symbols)
  - [ ] Create job record
  - [ ] Publish Celery message to Redis
- [ ] go-app: `GET /api/jobs/:id/status`
- [ ] go-app: `GET /api/jobs/:id/metrics` (fetch from PostgreSQL)
- [ ] go-app: `GET /api/jobs` (list user's jobs)

#### WebSocket (Live Updates)
- [ ] Update existing `/api/stream/portfolio/:userId` with JWT auth
- [ ] Subscribe to Kafka `portfolio_data` topic
- [ ] Stream real-time portfolio updates to React UI

**Deliverable**: Full REST API for strategy upload and job submission

---

### Week 7-8: Frontend Dashboard

#### Strategy Upload Page
- [ ] React: `src/pages/Strategies.tsx`
- [ ] File upload component (drag & drop)
- [ ] Strategy list table (name, created date, actions)
- [ ] Delete confirmation modal

#### Backtest Configuration
- [ ] React: `src/pages/NewBacktest.tsx`
- [ ] Form: Strategy dropdown, start/end dates, initial cash, commission
- [ ] Submit → POST `/api/jobs/backtest` → Redirect to results

#### Results Dashboard
- [ ] React: `src/pages/Results.tsx` (comprehensive metrics UI)
- [ ] Components (see UI_DESIGN.md):
  - [ ] JobHeader (status, runtime, order count)
  - [ ] HeroMetrics (5 cards: profit, Sharpe, drawdown, win rate, fees)
  - [ ] EquityCurve (Lightweight Charts candlestick)
  - [ ] TabContainer (Risk Metrics, Trade Stats, Portfolio Details)
  - [ ] RuntimeStats (sidebar with live updates)
  - [ ] ActionsToolbar (download, export, logs, clone)

#### Real-Time Updates
- [ ] React Query: Poll job status every 2s while running
- [ ] Auto-refresh metrics on completion
- [ ] Loading skeletons during fetch
- [ ] Error handling (failed jobs, LEAN errors)

**Deliverable**: Complete user flow - upload strategy → run backtest → view 90+ metrics

---

## Priority Tasks

### P0 (Blockers)
1. PostgreSQL + TimescaleDB setup
2. JWT authentication
3. LEAN engine integration
4. Results parser (90+ metrics)

### P1 (High)
1. Strategy upload (S3)
2. Backtest job submission
3. Results dashboard UI
4. Equity curve chart

### P2 (Medium)
1. Live trading (paper)
2. WebSocket streaming
3. Job logs viewer
4. Strategy cloning

### P3 (Nice-to-Have)
1. Prometheus + Grafana
2. Email notifications
3. Dark mode
4. Mobile polish

---

## Technical Decisions

### LEAN vs Backtrader
**Decision**: Migrate to LEAN
**Rationale**: 90+ metrics, industry standard, better documentation
**Timeline**: Week 3-4

### Chart Library
**Decision**: Lightweight Charts
**Rationale**: Better performance for financial data, interactive, touch-friendly
**Alternative**: Chart.js (easier but slower)

### Database
**Decision**: PostgreSQL + TimescaleDB extension
**Rationale**: Relational data (users, jobs) + time series (equity curve)
**Alternative**: Separate InfluxDB (more complex)

### State Management
**Decision**: React Query + Zustand
**Rationale**: React Query for server state, Zustand for client state
**Alternative**: Redux (overkill for MVP)

---

## Testing Strategy

### Unit Tests
- Go: Table-driven tests for handlers
- Python: Pytest for LEAN adapter, results parser
- React: Vitest for components

### Integration Tests
- End-to-end: Upload strategy → run backtest → verify metrics
- Kafka: Producer/consumer test
- PostgreSQL: Migration test

### Load Tests
- Locust: 100 concurrent backtests
- Target: p95 API latency <500ms

---

## Local Development Setup

### Quick Start
```bash
# 1. Start infrastructure (Kafka, Redis)
docker compose -f local-kafka-docker-compose.yml up -d

# 2. Start PostgreSQL (local)
docker run -d -p 5432:5432 \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=atp \
  timescale/timescaledb:latest-pg14

# 3. Run migrations
cd migrations && migrate -database "postgres://postgres:password@localhost:5432/atp?sslmode=disable" up

# 4. Start Go API
cd go-app && go run .

# 5. Start Celery worker
cd python && celery -A celery_worker worker --loglevel=info

# 6. Start frontend
cd web && npm run dev
```

### Environment Variables
```bash
# go-app/.env
JWT_SECRET_KEY=your-secret-key
DATABASE_URL=postgres://postgres:password@localhost:5432/atp
REDIS_URL=redis://localhost:6379
S3_BUCKET=atp-strategies-local

# python/.env
CELERY_BROKER_URL=redis://localhost:6379
DATABASE_URL=postgres://postgres:password@localhost:5432/atp
KAFKA_BOOTSTRAP_SERVERS=localhost:9092

# go-data/.env
APCA_API_KEY_ID=PKHQTX30R01VKNFYB03M
APCA_API_SECRET_KEY=hb9erv6LkY9K4jaWm60HjNYGSArkaWtjlGSZRc2K
KAFKA_BOOTSTRAP_SERVERS=localhost:9092
```

---

## Code Structure

```
.
├── go-app/                  # REST API + WebSocket
│   ├── handlers/
│   │   ├── auth.go          # Register, login
│   │   ├── strategies.go    # Upload, list, delete
│   │   ├── jobs.go          # Submit, status, metrics
│   │   └── stream.go        # WebSocket
│   ├── middleware/
│   │   └── jwt.go           # JWT validation
│   ├── models/
│   │   └── models.go        # User, Strategy, Job structs
│   └── main.go
│
├── go-data/                 # Alpaca → Kafka
│   └── main.go
│
├── python/                  # Celery workers
│   ├── celery_worker.py     # Task definitions
│   ├── lean_adapter.py      # KafkaDataFeed implementation
│   ├── results_parser.py    # LEAN JSON → PostgreSQL
│   └── strategy_validator.py  # Security scanning
│
├── web/                     # React frontend
│   ├── src/
│   │   ├── pages/
│   │   │   ├── Login.tsx
│   │   │   ├── Strategies.tsx
│   │   │   ├── NewBacktest.tsx
│   │   │   └── Results.tsx
│   │   ├── components/
│   │   │   └── results/
│   │   │       ├── JobHeader.tsx
│   │   │       ├── HeroMetrics.tsx
│   │   │       ├── EquityCurve.tsx
│   │   │       └── ...
│   │   └── hooks/
│   │       ├── useAuth.ts
│   │       ├── useJobStatus.ts
│   │       └── useJobMetrics.ts
│   └── package.json
│
├── kubernetes/              # K8s manifests
│   ├── infrastructure/      # Kafka, Redis, PostgreSQL
│   ├── core/                # go-app, go-data, celery-worker
│   ├── secrets/             # Sealed secrets
│   └── argocd/              # ArgoCD applications
│
├── migrations/              # Database migrations
│   ├── 001_create_users.sql
│   └── ...
│
└── documentation/           # This folder
    ├── ARCHITECTURE.md
    ├── DEPLOYMENT.md
    ├── DATABASE.md
    ├── UI_DESIGN.md
    └── DEVELOPMENT.md
```

---

## Definition of Done (MVP)

### Functional Requirements
- ✅ User can register and login
- ✅ User can upload .py strategy file
- ✅ User can submit backtest with config (dates, cash, symbols)
- ✅ Backtest executes in LEAN engine
- ✅ User sees 90+ metrics on results dashboard
- ✅ Equity curve displays as interactive chart
- ✅ All metrics grouped in tabs (Risk, Trades, Portfolio)
- ✅ User can download results (JSON, CSV)

### Non-Functional Requirements
- ✅ Backtest completes in <5 min (1-year daily data)
- ✅ API p95 latency <500ms
- ✅ System handles 10 concurrent backtests
- ✅ All logs written to `/logs` directory
- ✅ Zero-downtime deployments (Kubernetes rolling updates)
- ✅ ArgoCD auto-sync on Git push

### Infrastructure
- ✅ Kubernetes cluster deployed on AWS (kops)
- ✅ ArgoCD managing all deployments
- ✅ GitHub Actions CI/CD pipeline
- ✅ Prometheus + Grafana monitoring
- ✅ Sealed Secrets for credentials
- ✅ PostgreSQL HA with Patroni

---

## Post-MVP Roadmap

### v0.2 - Live Trading (4 weeks)
- Paper trading execution (real-time)
- Real-time WebSocket portfolio updates
- Stop/start controls
- Alerts (email, Slack)

### v0.3 - Advanced Features (6 weeks)
- Strategy optimization (parameter sweeps)
- Portfolio comparison (multiple strategies)
- Advanced charts (drawdown curve, rolling Sharpe)
- Strategy templates library

### v1.0 - Production Ready (8 weeks)
- Multi-source data (Polygon.io, Yahoo Finance)
- User teams (shared strategies)
- API rate limiting
- Comprehensive audit logs
- Security audit
