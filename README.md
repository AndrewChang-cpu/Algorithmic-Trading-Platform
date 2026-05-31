# Algorithmic Trading Platform

Paper trading and backtesting platform powered by QuantConnect LEAN engine.

## Quick Start

**Deploy to AWS** (automated):
```bash
./scripts/deploy-cluster.sh
```

**Local Development**:
```bash
# 1. Start infrastructure (Kafka KRaft, Redis, PostgreSQL, MinIO)
docker compose -f local-docker-compose.yml up -d

# 2. Run database migrations
cd migrations && migrate -database "postgres://postgres:password@localhost:5432/atp?sslmode=disable" up

# 3. Create MinIO bucket
docker run --rm --network host minio/mc:latest \
  sh -c 'mc alias set local http://localhost:9000 minioadmin minioadmin && mc mb local/atp-strategies --ignore-existing'

# 4. Create go-app/.env (if it doesn't exist)
cp go-app/.env.example go-app/.env   # then fill in any blanks

# 5. Start services (each in its own terminal)
cd go-app && go run .
cd python && celery -A celery_worker worker --loglevel=info
cd web && npm install && npm run dev
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
- ✅ Kafka message bus (KRaft mode)
- ✅ Celery job queue
- ✅ QuantConnect LEAN engine integration
- ✅ JWT authentication (RS256)
- ✅ Strategy file upload (.py) to S3/MinIO
- ✅ Backtest job submission and status tracking
- ✅ Results dashboard (90+ metrics)
- ✅ Equity curve charts
- ✅ PostgreSQL + TimescaleDB storage

### Planned
- [ ] Paper trading (live mode)
- [ ] Strategy optimization
- [ ] Portfolio comparison
- [ ] Advanced analytics

---

## Development

### Local Setup

**Prerequisites**: Docker, Go 1.22+, Python 3.11+, Node 20+, [golang-migrate](https://github.com/golang-migrate/migrate)

```bash
# 1. Start infrastructure (Kafka KRaft, Redis, PostgreSQL/TimescaleDB, MinIO)
docker compose -f local-docker-compose.yml up -d

# 2. Run database migrations
cd migrations && migrate -database "postgres://postgres:password@localhost:5432/atp?sslmode=disable" up

# 3. Create MinIO strategy bucket (first time only)
docker run --rm --network host minio/mc:latest \
  sh -c 'mc alias set local http://localhost:9000 minioadmin minioadmin && mc mb local/atp-strategies --ignore-existing'

# 4. Configure go-app environment
cat > go-app/.env <<'EOF'
CORS_ORIGINS=http://localhost:5173
DATABASE_URL=postgres://postgres:password@localhost:5432/atp?sslmode=disable
REDIS_URL=redis://localhost:6379/0
S3_ENDPOINT=http://localhost:9000
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_BUCKET=atp-strategies
S3_REGION=us-east-1
EOF

# 5. Start Go API
cd go-app && go run .

# 6. Start Celery worker
cd python && celery -A celery_worker worker --loglevel=info

# 7. Start frontend
cd web && npm install && npm run dev
```

### Useful Commands

#### Kafka
```bash
# Get the kafka container name
docker compose -f local-docker-compose.yml ps

# Test consumer (stock data)
docker exec -it <kafka-container> kafka-console-consumer \
  --bootstrap-server localhost:9092 --topic stock_data --from-beginning

# List topics
docker exec -it <kafka-container> kafka-topics \
  --bootstrap-server localhost:9092 --list
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
