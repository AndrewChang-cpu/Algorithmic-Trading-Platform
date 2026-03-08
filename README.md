# Algorithmic Trading Platform

Paper trading and backtesting platform powered by QuantConnect LEAN engine.

## Quick Start

**Deploy to AWS** (automated):
```bash
./scripts/deploy-cluster.sh
```

**Local Development**:
```bash
# Start infrastructure
docker compose -f local-kafka-docker-compose.yml up -d

# Start services
cd go-app && go run .
cd python && celery -A celery_worker worker --loglevel=info
cd web && npm run dev
```

See **[documentation/QUICK_START.md](documentation/QUICK_START.md)** for complete setup guide.

---

## Documentation

All documentation is in the **`documentation/`** folder:

| Document | Description |
|----------|-------------|
| **[QUICK_START.md](documentation/QUICK_START.md)** | 30-minute deployment guide |
| **[ARCHITECTURE.md](documentation/ARCHITECTURE.md)** | System design, components, data flow |
| **[DEPLOYMENT.md](documentation/DEPLOYMENT.md)** | Complete CI/CD setup with ArgoCD |
| **[DATABASE.md](documentation/DATABASE.md)** | Schema for 90+ LEAN metrics |
| **[UI_DESIGN.md](documentation/UI_DESIGN.md)** | Results dashboard specification |
| **[DEVELOPMENT.md](documentation/DEVELOPMENT.md)** | 8-week MVP roadmap |

---

## Architecture

```
Alpaca API → go-data → Kafka → LEAN Engine → PostgreSQL → React UI
                          ↓
                      Redis Queue
                          ↓
                    Celery Workers
```

**Key Technologies**:
- **Backend**: Go (REST API), Python (LEAN execution)
- **Frontend**: React + TypeScript, Lightweight Charts
- **Data**: Kafka, PostgreSQL + TimescaleDB
- **Infra**: Kubernetes (kops), ArgoCD, AWS

---

## Features

### Current
- ✅ Real-time data ingestion (Alpaca API)
- ✅ Kafka message bus
- ✅ Celery job queue
- ✅ Basic Backtrader strategies

### MVP (8 Weeks)
- [ ] QuantConnect LEAN engine integration
- [ ] JWT authentication
- [ ] Strategy file upload (.py)
- [ ] Comprehensive results dashboard (90+ metrics)
- [ ] Interactive equity curve charts
- [ ] PostgreSQL + TimescaleDB storage

### Post-MVP
- [ ] Paper trading (live mode)
- [ ] Strategy optimization
- [ ] Portfolio comparison
- [ ] Advanced analytics

---

## Development

### Local Setup
```bash
# 1. Start infrastructure (Kafka, Zookeeper, Redis)
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

### Useful Commands

#### Kafka
```bash
# Test consumer
docker exec -it <kafka-container> /bin/sh
kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic stock_data --from-beginning

# List topics
kafka-topics.sh --bootstrap-server localhost:9092 --list

# Create topic
kafka-topics.sh --bootstrap-server localhost:9092 --create --topic stock_data --partitions 1 --replication-factor 1

# Delete topic
kafka-topics.sh --bootstrap-server localhost:9092 --delete --topic stock_data
```

#### Kubernetes
```bash
# Check cluster health
kops validate cluster

# View pods
kubectl get pods -A

# View logs
kubectl logs -f deployment/go-app -n atp-core

# Exec into pod
kubectl exec -it deployment/celery-worker -n atp-core -- /bin/bash
```

#### ArgoCD
```bash
# Access UI
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Get admin password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d
```

---

## CI/CD Pipeline

1. **Developer pushes to main**
2. **GitHub Actions builds Docker images** → Push to ECR
3. **Update kustomization.yaml** with new image tags
4. **ArgoCD detects change** → Auto-deploy to Kubernetes
5. **Rolling update** (zero downtime)

See [documentation/DEPLOYMENT.md](documentation/DEPLOYMENT.md) for complete setup.

---

## Project Structure

```
.
├── documentation/          # Complete technical docs
├── scripts/                # Deployment scripts
├── go-app/                 # REST API + WebSocket server
├── go-data/                # Alpaca → Kafka producer
├── python/                 # Celery workers, LEAN execution
├── web/                    # React frontend
├── kubernetes/             # K8s manifests
│   ├── infrastructure/     # Kafka, Redis, PostgreSQL
│   ├── core/               # go-app, celery-worker
│   ├── secrets/            # Sealed secrets
│   └── argocd/             # ArgoCD applications
├── migrations/             # Database migrations
├── research/               # Jupyter notebooks, experiments
└── kops.yaml               # Kubernetes cluster config
```

---

## Cost Estimate

**AWS Monthly**: ~$290 (optimized)

- EC2: $280 (7 nodes)
- EBS: $50 (500GB)
- S3: $2 (strategies)
- Data Transfer: $45
- ALB: $20

---

## Deployment

### Automated
```bash
./scripts/deploy-cluster.sh
```

### Manual
See [documentation/DEPLOYMENT.md](documentation/DEPLOYMENT.md)

### Teardown
```bash
./scripts/teardown-cluster.sh
```

---

## Learning Resources

- **QuantConnect LEAN**: https://www.quantconnect.com/docs/
- **Kubernetes**: https://kubernetes.io/docs/
- **ArgoCD**: https://argo-cd.readthedocs.io/
- **TimescaleDB**: https://docs.timescale.com/

---

## Contributing

1. Read [documentation/DEVELOPMENT.md](documentation/DEVELOPMENT.md)
2. Create feature branch
3. Make changes
4. Push to main → CI/CD handles deployment

---

## License

MIT

---

## Support

- Documentation: `documentation/` folder
- Issues: GitHub Issues
- Logs: All services write to `/logs`
