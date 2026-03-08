# Documentation

Complete documentation for the Algorithmic Trading Platform MVP.

## Quick Links

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - System design, components, data flow
- **[DEPLOYMENT.md](DEPLOYMENT.md)** - Zero-to-production deployment guide with ArgoCD
- **[DATABASE.md](DATABASE.md)** - Schema, migrations, queries
- **[UI_DESIGN.md](UI_DESIGN.md)** - Results dashboard specification (90+ metrics)
- **[DEVELOPMENT.md](DEVELOPMENT.md)** - 8-week roadmap, local setup, code structure

## Overview

**Goal**: Paper trading and backtesting platform using QuantConnect LEAN engine

**Key Features**:
- Upload Python strategies (.py files)
- Run backtests with comprehensive metrics (90+ from LEAN)
- View interactive results dashboard (equity curve, risk metrics, trade stats)
- JWT authentication, S3 storage, PostgreSQL + TimescaleDB

**Tech Stack**:
- **Backend**: Go (REST API), Python (Celery workers, LEAN execution)
- **Frontend**: React + TypeScript, Lightweight Charts
- **Data**: Alpaca API → Kafka → LEAN
- **Infrastructure**: Kubernetes (kops), ArgoCD (GitOps), AWS (us-east-1)

## Getting Started

### For Deployment
1. Read [DEPLOYMENT.md](DEPLOYMENT.md) for complete zero-to-production guide
2. Prerequisites: AWS CLI, kubectl, kops, Docker
3. Estimated time: 1-2 hours for full cluster setup

### For Development
1. Read [DEVELOPMENT.md](DEVELOPMENT.md) for local setup
2. Prerequisites: Docker, Go 1.23+, Python 3.10+, Node.js 18+
3. Quick start: `docker compose up` → run services locally

### For Understanding the System
1. Start with [ARCHITECTURE.md](ARCHITECTURE.md) - high-level overview
2. Read [DATABASE.md](DATABASE.md) - data models
3. Read [UI_DESIGN.md](UI_DESIGN.md) - user experience

## Documentation Structure

```
documentation/
├── README.md              # This file
├── ARCHITECTURE.md        # System design (components, data flow, costs)
├── DEPLOYMENT.md          # CI/CD with ArgoCD, Kubernetes setup
├── DATABASE.md            # Schema, 90+ metrics storage
├── UI_DESIGN.md           # Results dashboard specification
└── DEVELOPMENT.md         # 8-week roadmap, local setup
```

## Key Decisions

### Why LEAN?
- 90+ built-in metrics (vs. 10 in Backtrader)
- Industry standard (QuantConnect)
- Active community, better documentation

### Why Lightweight Charts?
- Better performance for financial data
- Interactive (zoom, pan, touch)
- Smaller bundle size than Chart.js

### Why ArgoCD?
- GitOps: Git as single source of truth
- Auto-sync on push to main
- Rollback capability
- Better than manual kubectl apply

### Why TimescaleDB?
- PostgreSQL extension (no separate database)
- Optimized for time series (equity curve)
- SQL queries for charts

## Cost Estimate

**AWS Monthly**: ~$290 (optimized with spot + reserved instances)

Breakdown:
- EC2: $280 (7 nodes: 1 master, 3 workers, 3 Kafka)
- EBS: $50 (500GB total)
- S3: $2 (50GB strategies)
- Data transfer: $45 (500GB/month)
- ALB: $20

## MVP Timeline

**8 weeks to production**:
- Week 1-2: Infrastructure + Auth
- Week 3-4: LEAN integration
- Week 5-6: Backend API + S3
- Week 7-8: Frontend dashboard

See [DEVELOPMENT.md](DEVELOPMENT.md) for detailed roadmap.

## Support

- Issues: GitHub Issues
- Architecture questions: See ARCHITECTURE.md
- Deployment issues: See DEPLOYMENT.md troubleshooting section
