#!/bin/bash
set -e

# Deploy Algorithmic Trading Platform to AWS
# See documentation/DEPLOYMENT.md for full guide

echo "🚀 Deploying ATP to AWS..."

# Configuration
export KOPS_STATE_STORE=s3://atp-kops-state-$(whoami)
export CLUSTER_NAME=atp.k8s.local
export AWS_REGION=us-east-1

# Step 1: Create S3 bucket for kops state
echo "📦 Creating S3 bucket for kops state..."
aws s3 mb $KOPS_STATE_STORE --region $AWS_REGION 2>/dev/null || echo "Bucket already exists"
aws s3api put-bucket-versioning \
  --bucket $(echo $KOPS_STATE_STORE | cut -d/ -f3) \
  --versioning-configuration Status=Enabled

# Step 2: Create kops cluster
echo "🏗️  Creating kops cluster..."
kops create -f kops.yaml

# Create SSH key if it doesn't exist
if [ ! -f ~/.ssh/id_rsa.pub ]; then
  echo "🔑 Generating SSH key..."
  ssh-keygen -t rsa -b 4096 -f ~/.ssh/id_rsa -N ""
fi

kops create secret --name $CLUSTER_NAME sshpublickey admin -i ~/.ssh/id_rsa.pub

# Deploy cluster
echo "☁️  Deploying to AWS (this will take 5-10 minutes)..."
kops update cluster --name $CLUSTER_NAME --yes

# Wait for cluster to be ready
echo "⏳ Waiting for cluster to be ready..."
kops validate cluster --wait 10m

# Step 3: Install Sealed Secrets
echo "🔒 Installing Sealed Secrets controller..."
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.18.0/controller.yaml
kubectl wait --for=condition=ready pod -l name=sealed-secrets-controller -n kube-system --timeout=300s

# Install kubeseal CLI if not present
if ! command -v kubeseal &> /dev/null; then
  echo "📥 Installing kubeseal CLI..."
  wget https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.18.0/kubeseal-linux-amd64 -O /tmp/kubeseal
  chmod +x /tmp/kubeseal
  sudo mv /tmp/kubeseal /usr/local/bin/
fi

# Step 4: Create sealed secrets
echo "🔐 Creating sealed secrets..."
mkdir -p kubernetes/secrets

# Alpaca credentials
kubectl create secret generic alpaca-credentials \
  --from-literal=APCA_API_KEY_ID=PKHQTX30R01VKNFYB03M \
  --from-literal=APCA_API_SECRET_KEY=hb9erv6LkY9K4jaWm60HjNYGSArkaWtjlGSZRc2K \
  --dry-run=client -o yaml | kubeseal -o yaml > kubernetes/secrets/alpaca-credentials-sealed.yaml

# JWT secret
kubectl create secret generic jwt-secret \
  --from-literal=JWT_SECRET_KEY=$(openssl rand -base64 32) \
  --dry-run=client -o yaml | kubeseal -o yaml > kubernetes/secrets/jwt-secret-sealed.yaml

# Apply secrets
kubectl apply -f kubernetes/secrets/

# Step 5: Install ArgoCD
echo "🎯 Installing ArgoCD..."
kubectl create namespace argocd
kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=argocd-server -n argocd --timeout=300s

# Get ArgoCD admin password
ARGOCD_PASSWORD=$(kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath="{.data.password}" | base64 -d)
echo ""
echo "✅ ArgoCD installed!"
echo "   URL: kubectl port-forward svc/argocd-server -n argocd 8080:443"
echo "   Username: admin"
echo "   Password: $ARGOCD_PASSWORD"
echo ""

# Step 6: Deploy infrastructure via ArgoCD
echo "🏗️  Deploying infrastructure (Kafka, Redis, PostgreSQL)..."
kubectl apply -f kubernetes/argocd/infrastructure-app.yaml

# Step 7: Deploy core services via ArgoCD
echo "🚢 Deploying core services (go-app, go-data, celery-worker)..."
kubectl apply -f kubernetes/argocd/core-app.yaml

# Step 8: Deploy monitoring stack
echo "📊 Deploying monitoring (Prometheus, Grafana)..."
kubectl apply -f kubernetes/argocd/monitoring-app.yaml

# Wait for applications to sync
echo "⏳ Waiting for ArgoCD to sync applications..."
sleep 30

echo ""
echo "✅ Deployment complete!"
echo ""
echo "Next steps:"
echo "1. Access ArgoCD UI: kubectl port-forward svc/argocd-server -n argocd 8080:443"
echo "2. Check application status: kubectl get pods -A"
echo "3. Get API endpoint: kubectl get svc go-app -n atp-core"
echo "4. View Grafana: kubectl port-forward svc/kube-prometheus-stack-grafana -n atp-monitoring 3000:80"
echo ""
echo "See documentation/DEPLOYMENT.md for more details"
