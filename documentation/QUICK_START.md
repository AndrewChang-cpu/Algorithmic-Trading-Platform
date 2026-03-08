# Quick Start Guide

Get from zero to deployed cluster in 30 minutes.

## Prerequisites

Install these tools:
```bash
# AWS CLI
curl "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip" -o "awscliv2.zip"
unzip awscliv2.zip && sudo ./aws/install

# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
chmod +x kubectl && sudo mv kubectl /usr/local/bin/

# kops
curl -LO https://github.com/kubernetes/kops/releases/download/$(curl -s https://api.github.com/repos/kubernetes/kops/releases/latest | grep tag_name | cut -d '"' -f 4)/kops-linux-amd64
chmod +x kops-linux-amd64 && sudo mv kops-linux-amd64 /usr/local/bin/kops

# Configure AWS
aws configure
# Enter: Access Key ID, Secret Access Key, Region (us-east-1), Output (json)
```

## Deploy Cluster (Automated)

```bash
# Clone repo
git clone https://github.com/AndrewChang-cpu/Algorithmic-Trading-Platform
cd Algorithmic-Trading-Platform

# Run deployment script
./scripts/deploy-cluster.sh
```

**What it does**:
1. Creates S3 bucket for kops state
2. Deploys Kubernetes cluster (7 nodes)
3. Installs Sealed Secrets controller
4. Creates secrets (Alpaca API keys, JWT)
5. Installs ArgoCD
6. Deploys infrastructure (Kafka, Redis, PostgreSQL)
7. Deploys core services (go-app, go-data, celery-worker)
8. Deploys monitoring (Prometheus, Grafana)

**Time**: ~10 minutes

## Verify Deployment

```bash
# Check cluster health
kops validate cluster

# Check pods
kubectl get pods -A

# Expected namespaces:
# - atp-core: go-app, go-data, celery-worker
# - atp-data: kafka, zookeeper, redis
# - atp-db: postgres
# - atp-monitoring: prometheus, grafana
# - argocd: argocd-server
```

## Access Services

### ArgoCD (GitOps UI)
```bash
# Port-forward
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Get password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d

# Open browser: https://localhost:8080
# Username: admin
# Password: (from above)
```

### API Endpoint
```bash
# Get LoadBalancer URL
kubectl get svc go-app -n atp-core -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'

# Test
curl http://<loadbalancer-url>:8080/api/health
```

### Grafana (Monitoring)
```bash
# Port-forward
kubectl port-forward svc/kube-prometheus-stack-grafana -n atp-monitoring 3000:80

# Default credentials
# Username: admin
# Password: prom-operator

# Open browser: http://localhost:3000
```

## Configure CI/CD (GitHub Actions)

### 1. Create ECR Repositories
```bash
aws ecr create-repository --repository-name atp/go-app --region us-east-1
aws ecr create-repository --repository-name atp/go-data --region us-east-1
aws ecr create-repository --repository-name atp/celery-worker --region us-east-1
aws ecr create-repository --repository-name atp/web --region us-east-1
```

### 2. Add GitHub Secrets

In your GitHub repo: Settings → Secrets and variables → Actions

Add these secrets:
```
AWS_ACCOUNT_ID: 337909769295
AWS_REGION: us-east-1
AWS_ACCESS_KEY_ID: <your-key>
AWS_SECRET_ACCESS_KEY: <your-secret>
KOPS_STATE_STORE: s3://atp-kops-state-<username>
```

### 3. Create GitHub Actions Workflow

File: `.github/workflows/deploy.yml` (see DEPLOYMENT.md for full workflow)

### 4. Test CI/CD

```bash
# Make a change
echo "# Test" >> README.md
git add README.md
git commit -m "Test CI/CD"
git push origin main

# GitHub Actions will:
# 1. Build Docker images
# 2. Push to ECR
# 3. Update kustomization.yaml with new image tags
# 4. ArgoCD detects change and deploys automatically
```

## Development Workflow

### Local Development
```bash
# Start infrastructure
docker compose -f local-kafka-docker-compose.yml up -d

# Start PostgreSQL
docker run -d -p 5432:5432 \
  -e POSTGRES_PASSWORD=password \
  -e POSTGRES_DB=atp \
  timescale/timescaledb:latest-pg14

# Run services
cd go-app && go run .  # Terminal 1
cd python && celery -A celery_worker worker --loglevel=info  # Terminal 2
cd web && npm run dev  # Terminal 3
```

### Making Changes

1. **Backend (Go)**:
   - Edit `go-app/handlers/*.go`
   - Push to `main` → CI builds → ArgoCD deploys

2. **Workers (Python)**:
   - Edit `python/*.py`
   - Push to `main` → CI builds → ArgoCD deploys

3. **Frontend (React)**:
   - Edit `web/src/**/*.tsx`
   - Push to `main` → CI builds → ArgoCD deploys

### Debugging in Kubernetes

```bash
# View logs
kubectl logs -f deployment/go-app -n atp-core
kubectl logs -f deployment/celery-worker -n atp-core

# Exec into pod
kubectl exec -it deployment/go-app -n atp-core -- /bin/sh

# Check Kafka messages
kubectl exec -it kafka-0 -n atp-data -- kafka-console-consumer \
  --bootstrap-server localhost:9092 \
  --topic stock_data \
  --from-beginning

# Check Redis queue
kubectl exec -it redis-0 -n atp-data -- redis-cli LLEN celery
```

## Teardown

```bash
# Run teardown script
./scripts/teardown-cluster.sh

# Confirm by typing: yes
```

**What it deletes**:
- Kubernetes cluster (all nodes)
- S3 state bucket
- ECR repositories
- All data (PostgreSQL, Kafka, etc.)

## Next Steps

1. **Read Documentation**:
   - [ARCHITECTURE.md](ARCHITECTURE.md) - Understand system design
   - [DATABASE.md](DATABASE.md) - Schema and queries
   - [UI_DESIGN.md](UI_DESIGN.md) - Results dashboard spec
   - [DEVELOPMENT.md](DEVELOPMENT.md) - 8-week roadmap

2. **Start Development**:
   - Week 1-2: PostgreSQL + Auth
   - Week 3-4: LEAN integration
   - Week 5-6: Backend API
   - Week 7-8: Frontend dashboard

3. **Monitor Progress**:
   - ArgoCD: Application sync status
   - Grafana: System metrics
   - Logs: `/logs` directory in pods

## Common Issues

### Cluster won't create
```bash
# Check AWS limits
aws service-quotas list-service-quotas --service-code ec2 --query 'Quotas[?QuotaName==`Running On-Demand Standard (A, C, D, H, I, M, R, T, Z) instances`]'

# Increase limits in AWS Console if needed
```

### ArgoCD app stuck in "Progressing"
```bash
# Check sync status
kubectl describe application atp-core -n argocd

# Manual sync
kubectl patch application atp-core -n argocd --type merge -p '{"operation":{"initiatedBy":{"username":"admin"},"sync":{"revision":"HEAD"}}}'
```

### Pods in CrashLoopBackOff
```bash
# Check logs
kubectl logs <pod-name> -n <namespace> --previous

# Common fixes:
# - Missing secrets: kubectl get secrets -n <namespace>
# - Connection errors: Check service names and ports
# - Image pull errors: Verify ECR permissions
```

## Support

- Documentation: `documentation/` folder
- Issues: GitHub Issues
- Logs: All services log to `/logs` directory
