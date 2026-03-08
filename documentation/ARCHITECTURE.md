# Architecture

## Overview

Algorithmic trading platform for paper trading and backtesting using QuantConnect LEAN engine. Users upload Python strategies via web UI, submit backtest/live trading jobs, and view comprehensive performance metrics.

## System Architecture Diagram

```mermaid
graph TB
    subgraph "External Data Sources"
        Alpaca[Alpaca WebSocket API<br/>Market Data]
    end

    subgraph "Data Ingestion Layer"
        GoData[go-data Service<br/>Alpaca → Kafka Producer]
    end

    subgraph "Message Bus - atp-data namespace"
        Kafka[(Kafka Cluster<br/>3 brokers<br/>stock_data topic)]
        Zookeeper[(Zookeeper<br/>Coordination)]
        Redis[(Redis<br/>Celery Queue)]
        Kafka -.-> Zookeeper
    end

    subgraph "User Interface"
        Browser[React Web UI<br/>Lightweight Charts]
    end

    subgraph "API Layer - atp-core namespace"
        GoApp[go-app REST API<br/>+ WebSocket Server<br/>Port 8080]
    end

    subgraph "Storage - atp-db namespace"
        PostgreSQL[(PostgreSQL + TimescaleDB<br/>Users, Jobs, Metrics<br/>Equity Curves)]
        S3[(S3 Bucket<br/>Strategy Files<br/>.py uploads)]
    end

    subgraph "Job Execution - atp-core namespace"
        CeleryWorker[Celery Workers<br/>Python]
        LEANEngine[LEAN Engine<br/>Docker Container<br/>Strategy Execution]
        KafkaFeed[KafkaDataFeed<br/>Kafka → LEAN Adapter]
    end

    subgraph "Monitoring - atp-monitoring namespace"
        Prometheus[(Prometheus<br/>Metrics Storage)]
        Grafana[Grafana<br/>Dashboards]
        Loki[(Loki<br/>Log Aggregation)]
        Promtail[Promtail<br/>Log Scraper]
    end

    subgraph "GitOps - argocd namespace"
        ArgoCD[ArgoCD<br/>Continuous Deployment]
        GitHub[GitHub Repository<br/>Manifests + Code]
    end

    %% Data Flow: Market Data Ingestion
    Alpaca -->|WebSocket Stream| GoData
    GoData -->|Publish Bars| Kafka

    %% Data Flow: User Interaction
    Browser -->|REST API| GoApp
    Browser <-->|WebSocket<br/>Real-time Updates| GoApp
    GoApp -->|JWT Auth| PostgreSQL
    GoApp -->|Store Strategy| S3
    GoApp -->|Publish Job| Redis
    GoApp -->|Fetch Metrics| PostgreSQL

    %% Data Flow: Job Execution
    CeleryWorker -->|Pull Tasks| Redis
    CeleryWorker -->|Download Strategy| S3
    CeleryWorker -->|Spawn Container| LEANEngine
    LEANEngine -->|Use Adapter| KafkaFeed
    KafkaFeed -->|Subscribe| Kafka
    LEANEngine -->|Results JSON<br/>90+ Metrics| CeleryWorker
    CeleryWorker -->|Store Results| PostgreSQL

    %% Data Flow: Live Trading
    LEANEngine -->|Portfolio Updates| Kafka
    Kafka -->|portfolio_data topic| GoApp

    %% Monitoring Flow
    GoApp -.->|Metrics| Prometheus
    CeleryWorker -.->|Metrics| Prometheus
    Kafka -.->|Metrics| Prometheus
    PostgreSQL -.->|Metrics| Prometheus
    Prometheus -->|Query| Grafana

    GoApp -.->|Logs to /logs| Promtail
    CeleryWorker -.->|Logs to /logs| Promtail
    Promtail -->|Ship Logs| Loki
    Loki -->|Query| Grafana

    %% GitOps Flow
    GitHub -->|Auto-Sync| ArgoCD
    ArgoCD -.->|Deploy Manifests| GoApp
    ArgoCD -.->|Deploy Manifests| CeleryWorker
    ArgoCD -.->|Deploy Manifests| Kafka

    %% Styling
    classDef external fill:#e1f5ff,stroke:#0288d1,stroke-width:2px
    classDef service fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    classDef storage fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef monitoring fill:#e8f5e9,stroke:#388e3c,stroke-width:2px
    classDef gitops fill:#fce4ec,stroke:#c2185b,stroke-width:2px

    class Alpaca external
    class GoData,GoApp,CeleryWorker,LEANEngine,KafkaFeed service
    class Kafka,Redis,Zookeeper,PostgreSQL,S3 storage
    class Prometheus,Grafana,Loki,Promtail monitoring
    class ArgoCD,GitHub gitops
    class Browser external
```

## Data Flow Scenarios

### 1. Backtest Execution Flow

```mermaid
sequenceDiagram
    actor User
    participant Browser
    participant GoApp as go-app API
    participant S3
    participant Redis
    participant Celery as Celery Worker
    participant LEAN as LEAN Engine
    participant Kafka
    participant DB as PostgreSQL

    User->>Browser: Upload strategy.py
    Browser->>GoApp: POST /api/strategies/upload
    GoApp->>S3: Store strategy file
    GoApp->>DB: Insert strategy record
    GoApp-->>Browser: Strategy ID

    User->>Browser: Submit backtest config
    Browser->>GoApp: POST /api/jobs/backtest
    GoApp->>DB: Create job record (status=queued)
    GoApp->>Redis: Publish Celery task
    GoApp-->>Browser: Job ID

    Celery->>Redis: Pull task
    Celery->>S3: Download strategy.py
    Celery->>DB: Update job (status=running)
    Celery->>LEAN: Spawn container + mount strategy

    LEAN->>Kafka: Subscribe to stock_data topic
    Kafka-->>LEAN: Stream historical bars
    LEAN->>LEAN: Execute strategy logic
    LEAN-->>Celery: Results JSON (90+ metrics)

    Celery->>DB: Insert performance_metrics
    Celery->>DB: Insert portfolio_metrics (TimescaleDB)
    Celery->>DB: Update job (status=completed)

    Browser->>GoApp: GET /api/jobs/:id/status (poll every 2s)
    GoApp->>DB: Query job status
    GoApp-->>Browser: status=completed

    Browser->>GoApp: GET /api/jobs/:id/metrics
    GoApp->>DB: Query all metrics
    GoApp-->>Browser: Full LEAN results
    Browser->>Browser: Render dashboard (90+ metrics)
```

### 2. Live Trading Flow

```mermaid
sequenceDiagram
    participant Alpaca
    participant GoData as go-data
    participant Kafka
    participant LEAN as LEAN Engine
    participant GoApp as go-app
    participant Browser

    Alpaca->>GoData: WebSocket stream (real-time bars)
    GoData->>Kafka: Publish to stock_data topic

    LEAN->>Kafka: Subscribe to stock_data (live mode)
    Kafka-->>LEAN: Stream real-time bars
    LEAN->>LEAN: Execute strategy logic
    LEAN->>Kafka: Publish portfolio updates (portfolio_data topic)

    GoApp->>Kafka: Subscribe to portfolio_data
    Kafka-->>GoApp: Portfolio state (equity, positions, P&L)
    GoApp-->>Browser: WebSocket stream
    Browser->>Browser: Update live dashboard (2s refresh)
```

### 3. CI/CD Deployment Flow

```mermaid
sequenceDiagram
    actor Developer
    participant GitHub
    participant Actions as GitHub Actions
    participant ECR
    participant Kustomize
    participant ArgoCD
    participant K8s as Kubernetes Cluster

    Developer->>GitHub: git push origin main
    GitHub->>Actions: Trigger workflow

    Actions->>Actions: Build Docker images (go-app, celery-worker, etc.)
    Actions->>ECR: Push images with SHA tag
    Actions->>Kustomize: Update kustomization.yaml (newTag: SHA)
    Actions->>GitHub: Commit kustomization changes

    ArgoCD->>GitHub: Poll for changes (auto-sync)
    ArgoCD->>ArgoCD: Detect kustomization update
    ArgoCD->>K8s: Apply new manifests
    K8s->>K8s: Rolling update (zero downtime)
    ArgoCD-->>Developer: Deployment complete notification
```

## Components

### Data Ingestion
- **go-data**: Alpaca WebSocket client → Kafka producer
- **Kafka**: Message broker for real-time bars (topic: `stock_data`)
- **Credentials**: `APCA_API_KEY_ID`, `APCA_API_SECRET_KEY` (Sealed Secrets)

### Job Orchestration
- **Redis**: Celery job queue
- **Celery Workers**: Python workers execute LEAN containers
- **Job Types**: Backtest (historical) or Live (paper trading)

### Strategy Execution
- **LEAN Engine**: QuantConnect backtesting engine in Docker
- **KafkaDataFeed**: Custom adapter (Kafka → LEAN Bar objects)
- **Output**: JSON with 90+ metrics (Sharpe, Sortino, Alpha, Beta, VaR, trade stats, etc.)
- **Sandboxing**: Block `os`, `subprocess`, `socket`, `eval` imports

### Backend API (Go)
- **go-app**: REST API + WebSocket server
- **Endpoints**:
  - `POST /api/auth/register` - User registration
  - `POST /api/auth/login` - JWT authentication
  - `POST /api/strategies/upload` - Upload .py file to S3
  - `POST /api/jobs/backtest` - Submit backtest job
  - `GET /api/jobs/:id/metrics` - Fetch LEAN results
  - `WS /api/stream/portfolio/:userId` - Real-time portfolio updates

### Database
- **PostgreSQL**: Users, strategies, jobs, performance metrics (90+ fields)
- **TimescaleDB**: Equity curve time series (OHLC data)
- **S3**: Strategy file storage (versioned, encrypted)

### Frontend (React)
- **Auth**: Login/register with JWT
- **Strategy Upload**: File upload (.py)
- **Backtest Config**: Form (dates, cash, symbols)
- **Results Dashboard**: Comprehensive metrics display (hero cards, tabs, equity chart)
- **Live Trading**: Real-time WebSocket updates

## Infrastructure (Kubernetes on AWS)

### Kops Cluster
- **Master**: 1x t3.small (control plane)
- **Workers**: 3x t2.micro (API, Celery)
- **Kafka**: 3x t3.small (dedicated nodes, tainted)
- **Database**: 2x t3.medium (PostgreSQL HA with Patroni)
- **Region**: us-east-1
- **Network**: Cilium CNI, 172.20.0.0/16 CIDR

### Namespaces
- `atp-core`: go-app, celery-workers
- `atp-data`: kafka, zookeeper, redis
- `atp-db`: postgresql
- `atp-monitoring`: prometheus, grafana, loki
- `argocd`: ArgoCD (GitOps)

### Secrets Management
- **Sealed Secrets**: Alpaca API keys, JWT secret, PostgreSQL credentials

### Storage
- **PostgreSQL**: EBS gp3, 100GB
- **Kafka**: EBS gp3, 200GB, 7-day retention
- **S3**: Strategy files, backups

## Security

### Authentication
- JWT tokens (HS256, 24hr expiry)
- bcrypt password hashing (cost 12)

### Strategy Sandboxing
- Blocked imports: `os`, `subprocess`, `socket`, `eval`
- Container limits: 2 CPU cores, 4GB RAM
- Execution timeout: 30min (backtests), 24hr (live)

### Network Policies
- Cilium NetworkPolicy: Deny egress by default
- Allow-list: API → PostgreSQL, Workers → Kafka/S3

## Monitoring

### Prometheus Metrics
- Celery: Queue depth, task duration
- API: Request latency (p95), error rate
- Kafka: Partition lag, broker health
- PostgreSQL: Connection count, query duration

### Grafana Dashboards
- System: Node CPU/memory, pod status
- Application: Job throughput, API latency
- Business: Active users, backtests/day

### Loki Logs
- All services log to `/logs` (JSON format)
- Promtail DaemonSet scrapes pod logs
- Retention: 30 days

### Alerts
- Critical: Database down, Kafka offline, no workers
- Warning: High failure rate, slow API, queue backup
- Destination: Slack

## Cost Estimate (Monthly)

| Resource | Configuration | Cost |
|----------|---------------|------|
| EC2 | 1 master + 3 workers + 3 Kafka + 2 DB | ~$280 |
| EBS | 500GB total | ~$50 |
| S3 | 50GB | ~$2 |
| Data Transfer | 500GB | ~$45 |
| ALB | 1 load balancer | ~$20 |
| **Total** | | **~$397** |
| **Optimized** (spot + reserved) | | **~$290** |
