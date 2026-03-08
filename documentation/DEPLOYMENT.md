# Deployment Guide

## Zero to Production: Complete Flow

This guide takes you from nothing to a fully deployed Kubernetes cluster on AWS with CI/CD.

---

## Prerequisites

- AWS CLI configured with credentials (`aws configure`)
- kubectl installed
- kops installed
- Docker installed (for local testing)
- GitHub repository access (for ArgoCD)

---

## Step 1: Bootstrap Infrastructure (One-Time Setup)

### 1.1 Create S3 Bucket for Kops State

```bash
export KOPS_STATE_STORE=s3://atp-kops-state-$(whoami)
export CLUSTER_NAME=atp.k8s.local

aws s3 mb $KOPS_STATE_STORE --region us-east-1
aws s3api put-bucket-versioning \
  --bucket $(echo $KOPS_STATE_STORE | cut -d/ -f3) \
  --versioning-configuration Status=Enabled
```

### 1.2 Create Kops Cluster

```bash
# Create cluster configuration
kops create -f kops.yaml

# Create SSH key for cluster access
kops create secret --name $CLUSTER_NAME sshpublickey admin -i ~/.ssh/id_rsa.pub

# Deploy cluster to AWS
kops update cluster --name $CLUSTER_NAME --yes

# Wait for cluster to be ready (5-10 minutes)
kops validate cluster --wait 10m
```

**What this creates**:
- 1x t3.small master node (control plane)
- 3x t2.micro worker nodes
- 3x t3.small Kafka nodes (tainted)
- 2x t3.medium database nodes
- VPC, subnets, security groups, IAM roles
- Auto-scaling groups

### 1.3 Install Sealed Secrets Controller

```bash
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.18.0/controller.yaml

# Wait for controller to be ready
kubectl wait --for=condition=ready pod -l name=sealed-secrets-controller -n kube-system --timeout=300s
```

### 1.4 Create Sealed Secrets

```bash
# Install kubeseal CLI
wget https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.18.0/kubeseal-linux-amd64 -O kubeseal
chmod +x kubeseal
sudo mv kubeseal /usr/local/bin/

# Create Alpaca credentials secret
kubectl create secret generic alpaca-credentials \
  --from-literal=APCA_API_KEY_ID=PKHQTX30R01VKNFYB03M \
  --from-literal=APCA_API_SECRET_KEY=hb9erv6LkY9K4jaWm60HjNYGSArkaWtjlGSZRc2K \
  --dry-run=client -o yaml | kubeseal -o yaml > kubernetes/secrets/alpaca-credentials-sealed.yaml

# Create JWT secret (generate random key)
kubectl create secret generic jwt-secret \
  --from-literal=JWT_SECRET_KEY=$(openssl rand -base64 32) \
  --dry-run=client -o yaml | kubeseal -o yaml > kubernetes/secrets/jwt-secret-sealed.yaml

# Apply sealed secrets
kubectl apply -f kubernetes/secrets/
```

### 1.5 Create ECR Repository

```bash
aws ecr create-repository --repository-name atp/go-app --region us-east-1
aws ecr create-repository --repository-name atp/go-data --region us-east-1
aws ecr create-repository --repository-name atp/celery-worker --region us-east-1
aws ecr create-repository --repository-name atp/web --region us-east-1

# Get ECR login token (valid for 12 hours)
aws ecr get-login-password --region us-east-1 | docker login \
  --username AWS \
  --password-stdin 337909769295.dkr.ecr.us-east-1.amazonaws.com
```

---

## Step 2: Install ArgoCD

### 2.1 Install ArgoCD

```bash
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml

# Wait for ArgoCD to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s
```

### 2.2 Expose ArgoCD UI

```bash
# Option 1: Port-forward (local development)
kubectl port-forward svc/argocd-server -n argocd 8080:443

# Option 2: LoadBalancer (production)
kubectl patch svc argocd-server -n argocd -p '{"spec": {"type": "LoadBalancer"}}'
```

### 2.3 Get Admin Password

```bash
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d; echo
```

Login at: `https://localhost:8080` (username: `admin`, password from above)

### 2.4 Configure Repository

In ArgoCD UI:
1. Settings → Repositories → Connect Repo
2. Repository URL: `https://github.com/AndrewChang-cpu/Algorithmic-Trading-Platform`
3. Authentication: None (public repo) or SSH key

---

## Step 3: Deploy Infrastructure Services

### 3.1 Create ArgoCD Application for Infrastructure

```bash
cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: atp-infrastructure
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/AndrewChang-cpu/Algorithmic-Trading-Platform
    targetRevision: main
    path: kubernetes/infrastructure
  destination:
    server: https://kubernetes.default.svc
    namespace: atp-data
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
    - CreateNamespace=true
EOF
```

**What this deploys** (from `kubernetes/infrastructure/`):
- Kafka + Zookeeper
- Redis
- PostgreSQL (StatefulSet with Patroni)

### 3.2 Verify Infrastructure

```bash
kubectl get pods -n atp-data
# Expected: kafka-0, kafka-1, kafka-2, zookeeper-0, redis-0

kubectl get pods -n atp-db
# Expected: postgres-0, postgres-1
```

---

## Step 4: Deploy Application Services

### 4.1 Create ArgoCD Application for Core Services

```bash
cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: atp-core
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/AndrewChang-cpu/Algorithmic-Trading-Platform
    targetRevision: main
    path: kubernetes/core
  destination:
    server: https://kubernetes.default.svc
    namespace: atp-core
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
    - CreateNamespace=true
EOF
```

**What this deploys** (from `kubernetes/core/`):
- go-app (API server)
- go-data (Alpaca → Kafka)
- celery-worker (LEAN execution)

### 4.2 Deploy Monitoring Stack

```bash
cat <<EOF | kubectl apply -f -
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: atp-monitoring
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/prometheus-community/helm-charts
    targetRevision: 45.0.0
    chart: kube-prometheus-stack
  destination:
    server: https://kubernetes.default.svc
    namespace: atp-monitoring
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
    - CreateNamespace=true
EOF
```

---

## Step 5: CI/CD Pipeline (GitHub Actions)

### 5.1 Create GitHub Secrets

In GitHub repository settings → Secrets and variables → Actions:

```
AWS_ACCOUNT_ID: 337909769295
AWS_REGION: us-east-1
AWS_ACCESS_KEY_ID: <your-key>
AWS_SECRET_ACCESS_KEY: <your-secret>
KOPS_STATE_STORE: s3://atp-kops-state-<username>
```

### 5.2 GitHub Actions Workflow

Create `.github/workflows/deploy.yml`:

```yaml
name: Build and Deploy

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

env:
  ECR_REGISTRY: 337909769295.dkr.ecr.us-east-1.amazonaws.com

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        service: [go-app, go-data, celery-worker, web]

    steps:
    - uses: actions/checkout@v3

    - name: Configure AWS credentials
      uses: aws-actions/configure-aws-credentials@v2
      with:
        aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
        aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
        aws-region: us-east-1

    - name: Login to ECR
      run: |
        aws ecr get-login-password --region us-east-1 | \
        docker login --username AWS --password-stdin $ECR_REGISTRY

    - name: Build and push image
      run: |
        IMAGE_TAG=${{ github.sha }}
        docker build -t $ECR_REGISTRY/atp/${{ matrix.service }}:$IMAGE_TAG \
          -f ${{ matrix.service }}/Dockerfile .
        docker push $ECR_REGISTRY/atp/${{ matrix.service }}:$IMAGE_TAG
        docker tag $ECR_REGISTRY/atp/${{ matrix.service }}:$IMAGE_TAG \
          $ECR_REGISTRY/atp/${{ matrix.service }}:latest
        docker push $ECR_REGISTRY/atp/${{ matrix.service }}:latest

    - name: Update Kustomization
      if: github.ref == 'refs/heads/main'
      run: |
        sed -i "s|newTag:.*|newTag: ${{ github.sha }}|" \
          kubernetes/core/${{ matrix.service }}/kustomization.yaml
        git config user.name "GitHub Actions"
        git config user.email "actions@github.com"
        git add kubernetes/core/${{ matrix.service }}/kustomization.yaml
        git commit -m "Update ${{ matrix.service }} to ${{ github.sha }}" || true
        git push || true
```

**How it works**:
1. **On PR**: Build images, push to ECR with commit SHA tag
2. **On merge to main**: Build + update Kustomization files with new image tag
3. **ArgoCD auto-sync**: Detects Kustomization change, deploys new images

---

## Step 6: Access Applications

### 6.1 Get LoadBalancer URLs

```bash
# API endpoint
kubectl get svc go-app -n atp-core -o jsonpath='{.status.loadBalancer.ingress[0].hostname}'

# Grafana (if using LoadBalancer)
kubectl get svc -n atp-monitoring | grep grafana
```

### 6.2 Port-Forward for Local Access

```bash
# API
kubectl port-forward svc/go-app -n atp-core 8080:8080

# Grafana
kubectl port-forward svc/kube-prometheus-stack-grafana -n atp-monitoring 3000:80

# Kafka (for debugging)
kubectl port-forward svc/kafka -n atp-data 9092:9092
```

---

## Directory Structure (Kubernetes Manifests)

```
kubernetes/
├── infrastructure/           # ArgoCD app: atp-infrastructure
│   ├── kafka/
│   │   ├── kafka.yaml
│   │   └── zookeeper.yaml
│   ├── redis/
│   │   └── redis.yaml
│   └── postgresql/
│       ├── statefulset.yaml
│       └── service.yaml
│
├── core/                     # ArgoCD app: atp-core
│   ├── go-app/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── kustomization.yaml  # Image tag updated by CI
│   ├── go-data/
│   │   ├── deployment.yaml
│   │   └── kustomization.yaml
│   └── celery-worker/
│       ├── deployment.yaml
│       └── kustomization.yaml
│
├── secrets/                  # Sealed secrets (applied manually)
│   ├── alpaca-credentials-sealed.yaml
│   └── jwt-secret-sealed.yaml
│
└── argocd/                   # ArgoCD application definitions
    ├── infrastructure-app.yaml
    ├── core-app.yaml
    └── monitoring-app.yaml
```

---

## Deployment Flow Summary

```
1. Developer pushes to main branch
     ↓
2. GitHub Actions triggered
     ↓
3. Build Docker images
     ↓
4. Push to ECR with commit SHA tag
     ↓
5. Update kustomization.yaml with new image tag
     ↓
6. Commit + push kustomization changes
     ↓
7. ArgoCD detects Git repo change (auto-sync enabled)
     ↓
8. ArgoCD applies new manifests to cluster
     ↓
9. Kubernetes rolling update (zero downtime)
     ↓
10. Prometheus alerts on failures (if any)
```

---

## Teardown (Delete Everything)

```bash
# Delete ArgoCD applications (removes all deployed resources)
kubectl delete application atp-core atp-infrastructure atp-monitoring -n argocd

# Delete cluster
kops delete cluster --name $CLUSTER_NAME --yes

# Delete S3 bucket
aws s3 rb $KOPS_STATE_STORE --force

# Delete ECR repositories
aws ecr delete-repository --repository-name atp/go-app --force --region us-east-1
aws ecr delete-repository --repository-name atp/go-data --force --region us-east-1
aws ecr delete-repository --repository-name atp/celery-worker --force --region us-east-1
```

---

## Troubleshooting

### Cluster won't validate
```bash
kops validate cluster --name $CLUSTER_NAME
# Check AWS console for EC2 instance failures
# Common issue: VPC limits, subnet exhaustion
```

### ArgoCD app stuck in "Progressing"
```bash
kubectl describe application atp-core -n argocd
# Check sync status and error messages
```

### Pods in CrashLoopBackOff
```bash
kubectl logs -n atp-core <pod-name> --previous
# Check for missing secrets, connection errors
```

### Images not pulling from ECR
```bash
# Verify ECR repository exists
aws ecr describe-repositories --region us-east-1

# Check image exists with tag
aws ecr list-images --repository-name atp/go-app --region us-east-1

# Verify IRSA permissions (if using)
kubectl describe pod <pod-name> -n atp-core
```
