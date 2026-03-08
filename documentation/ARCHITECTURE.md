# Architecture

## Overview

Algorithmic trading platform for paper trading and backtesting using QuantConnect LEAN engine. Users upload Python strategies via web UI, submit backtest/live trading jobs, and view comprehensive performance metrics.

## System Architecture Diagram

Resource-based view showing compute nodes and data flow:

```mermaid
graph TB
    subgraph External["EXTERNAL SERVICES"]
        Alpaca[Alpaca API<br/>Market Data]
        User[End Users<br/>Browser]
    end

    subgraph AWS["AWS CLOUD"]
        subgraph K8s["KUBERNETES CLUSTER"]
            subgraph Master["Control Plane<br/>t3.small"]
                K8sAPI[Kubernetes API Server]
            end

            subgraph Workers["Worker Nodes<br/>3x t2.micro"]
                GoApp[go-app<br/>REST API + WebSocket]
                GoData[go-data<br/>Alpaca Consumer]
                Redis[(Redis<br/>Job Queue)]
            end

            subgraph StrategyPods["Strategy Execution Pods<br/>Spawned dynamically"]
                WorkerPod[Celery Worker<br/>+ LEAN Engine<br/>+ KafkaDataFeed<br/>One pod per backtest job]
            end

            subgraph KafkaNodes["Kafka Nodes<br/>3x t3.small dedicated"]
                Kafka[Kafka Cluster<br/>KRaft]
            end

            subgraph DBNodes["Database Nodes<br/>2x t3.medium"]
                PostgreSQL[PostgreSQL + TimescaleDB<br/>Patroni HA]
            end

            subgraph Monitoring["Monitoring<br/>Runs on worker nodes"]
                Prometheus[Prometheus]
                Grafana[Grafana]
            end
        end

        S3[S3 Bucket<br/>Strategy Files]
        ECR[ECR Registry<br/>Docker Images]
        EBS[EBS Volumes<br/>PostgreSQL + Kafka]
    end

    subgraph GitOps["CI/CD"]
        GitHub[GitHub<br/>Source + Manifests]
        Actions[GitHub Actions<br/>Build + Push]
        ArgoCD[ArgoCD<br/>Runs in K8s]
    end

    %% External connections
    User -->|HTTPS| GoApp
    Alpaca -->|WebSocket| GoData

    %% Data ingestion
    GoData -->|Publish bars| Kafka

    %% User interactions
    GoApp -->|Store .py| S3
    GoApp -->|Auth + Queries| PostgreSQL
    GoApp -->|Enqueue job| Redis

    %% Job execution
    Redis -->|Pull task| WorkerPod
    WorkerPod -->|Download strategy| S3
    WorkerPod -->|Subscribe to data| Kafka
    WorkerPod -->|Store results| PostgreSQL

    %% Live trading updates
    WorkerPod -->|Publish portfolio| Kafka
    Kafka -->|Stream updates| GoApp
    GoApp -->|WebSocket| User

    %% Monitoring
    GoApp -.->|Metrics| Prometheus
    WorkerPod -.->|Metrics| Prometheus
    Kafka -.->|Metrics| Prometheus
    PostgreSQL -.->|Metrics| Prometheus
    Prometheus -->|Visualize| Grafana

    %% CI/CD
    GitHub -->|Trigger| Actions
    Actions -->|Build images| ECR
    Actions -->|Update manifests| GitHub
    GitHub -->|Auto-sync| ArgoCD
    ArgoCD -->|Deploy| Workers
    ArgoCD -->|Deploy| KafkaNodes
    ArgoCD -->|Deploy| DBNodes

    %% Persistence
    PostgreSQL -->|Data| EBS
    Kafka -->|Logs| EBS

    %% Styling
    classDef external fill:#e1f5ff,stroke:#0288d1,stroke-width:2px
    classDef compute fill:#fff3e0,stroke:#f57c00,stroke-width:2px
    classDef storage fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef monitoring fill:#e8f5e9,stroke:#388e3c,stroke-width:2px
    classDef cicd fill:#fce4ec,stroke:#c2185b,stroke-width:2px

    class Alpaca,User external
    class Master,Workers,KafkaNodes,DBNodes,StrategyPods,K8sAPI,GoApp,GoData,Redis,WorkerPod,Kafka,PostgreSQL compute
    class S3,ECR,EBS storage
    class Monitoring,Prometheus,Grafana monitoring
    class GitHub,Actions,ArgoCD cicd
```

## Key Architectural Decisions

### Zookeeper Removal
**Modern Kafka uses KRaft mode** (Kafka Raft consensus protocol), eliminating Zookeeper dependency. Benefits:
- Simpler deployment (one less service)
- Faster metadata operations
- Reduced operational complexity

### Celery + LEAN + KafkaDataFeed as Single Pod
**Strategy execution runs as unified pod**:
- Celery worker pulls job from Redis
- Spawns LEAN engine as subprocess/container
- KafkaDataFeed is a Python class within LEAN process
- All three run together in same pod lifecycle
- Pod terminates after job completes

### Resource Allocation
| Node Type | Count | Instance | Purpose |
|-----------|-------|----------|---------|
| Master | 1 | t3.small | Kubernetes control plane |
| Workers | 3 | t2.micro | API, data ingestion, monitoring |
| Kafka | 3 | t3.small | Dedicated Kafka brokers (tainted nodes) |
| Database | 2 | t3.medium | PostgreSQL HA with Patroni |
| Strategy Pods | Dynamic | Burstable | Spawned per backtest job, auto-scaled |

## Data Flow Scenarios

### 1. Backtest Execution Flow

```mermaid
sequenceDiagram
    actor User
    participant Browser
    participant GoApp as go-app
    participant S3
    participant Redis
    participant Worker as Strategy Pod<br/>(Celery+LEAN+KafkaDataFeed)
    participant Kafka
    participant DB as PostgreSQL

    User->>Browser: Upload strategy.py
    Browser->>GoApp: POST /api/strategies/upload
    GoApp->>S3: Store file
    GoApp->>DB: Insert strategy record
    GoApp-->>Browser: Strategy ID

    User->>Browser: Submit backtest
    Browser->>GoApp: POST /api/jobs/backtest
    GoApp->>DB: Create job (status=queued)
    GoApp->>Redis: Enqueue task
    GoApp-->>Browser: Job ID

    K8s->>Worker: Spawn pod
    Worker->>Redis: Pull task
    Worker->>S3: Download strategy.py
    Worker->>DB: Update status=running
    Worker->>Kafka: Subscribe stock_data
    Kafka-->>Worker: Stream bars
    Worker->>Worker: Execute LEAN strategy
    Worker->>DB: Write metrics + equity curve
    Worker->>DB: Update status=completed
    K8s->>Worker: Terminate pod

    Browser->>GoApp: Poll /api/jobs/:id/status
    GoApp->>DB: Query status
    GoApp-->>Browser: Completed + metrics
```

### 2. Live Trading Flow

```mermaid
sequenceDiagram
    participant Alpaca
    participant GoData as go-data
    participant Kafka
    participant Worker as Strategy Pod<br/>(live mode)
    participant GoApp as go-app
    participant Browser

    Alpaca->>GoData: WebSocket bars
    GoData->>Kafka: Publish stock_data

    Worker->>Kafka: Subscribe stock_data
    Kafka-->>Worker: Real-time bars
    Worker->>Worker: Execute LEAN logic
    Worker->>Kafka: Publish portfolio_data

    GoApp->>Kafka: Subscribe portfolio_data
    Kafka-->>GoApp: Portfolio updates
    GoApp-->>Browser: WebSocket stream
```

### 3. CI/CD Deployment

```mermaid
sequenceDiagram
    participant Dev as Developer
    participant GitHub
    participant Actions as GitHub Actions
    participant ECR
    participant ArgoCD
    participant K8s as Kubernetes

    Dev->>GitHub: git push main
    GitHub->>Actions: Trigger workflow
    Actions->>Actions: Build images
    Actions->>ECR: Push with SHA tag
    Actions->>GitHub: Update kustomization.yaml

    ArgoCD->>GitHub: Poll (auto-sync)
    ArgoCD->>ArgoCD: Detect change
    ArgoCD->>K8s: Apply manifests
    K8s->>K8s: Rolling update
```

## Components

### Data Ingestion
- **go-data**: Alpaca WebSocket client, Kafka producer
- **Kafka**: Message broker (KRaft mode, 3 brokers), topics: `stock_data`, `portfolio_data`

### Job Orchestration
- **Redis**: Celery job queue
- **Strategy Pods**: Dynamically spawned, contain Celery + LEAN + KafkaDataFeed

### Strategy Execution
- **LEAN Engine**: QuantConnect backtesting engine
- **KafkaDataFeed**: Python class implementing IDataQueueHandler, subscribes to Kafka
- **Sandboxing**: Block `os`, `subprocess`, `socket`, `eval` imports
- **Limits**: 2 CPU cores, 4GB RAM, 30min timeout (backtests)

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
- **PostgreSQL + TimescaleDB**: Users, strategies, jobs, performance metrics (90+ fields)
- **S3**: Strategy file storage (versioned, encrypted)
- **EBS**: Persistent volumes for PostgreSQL and Kafka

### Frontend
- **React + TypeScript**: Web UI
- **Lightweight Charts**: Interactive equity curves
- **Features**: Auth, strategy upload, backtest config, comprehensive results dashboard

## Infrastructure (Kubernetes on AWS)

### Kops Cluster
- **Region**: us-east-1
- **Network**: Cilium CNI, 172.20.0.0/16 CIDR
- **Nodes**: 1 master + 3 workers + 3 Kafka + 2 database = 9 EC2 instances

### Namespaces
- `atp-core`: go-app, go-data, redis, strategy pods
- `atp-data`: kafka
- `atp-db`: postgresql
- `atp-monitoring`: prometheus, grafana
- `argocd`: ArgoCD

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
- Allow-list: API to PostgreSQL, Workers to Kafka/S3

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

### Logs
- All services log to `/logs` (JSON format)
- Loki aggregates logs from all pods
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
