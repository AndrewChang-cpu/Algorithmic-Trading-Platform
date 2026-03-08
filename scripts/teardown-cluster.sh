#!/bin/bash
set -e

# Teardown Algorithmic Trading Platform cluster
# WARNING: This will delete all resources!

echo "[WARNING] This will delete the entire cluster and all data!"
read -p "Are you sure? (type 'yes' to confirm): " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
  echo "Aborted."
  exit 0
fi

# Configuration
export KOPS_STATE_STORE=s3://atp-kops-state-$(whoami)
export CLUSTER_NAME=atp.k8s.local
export AWS_REGION=us-east-1

echo "[DELETE] Deleting ArgoCD applications..."
kubectl delete application atp-core atp-infrastructure atp-monitoring -n argocd 2>/dev/null || echo "Applications not found"

echo "[WAIT] Waiting for resources to be cleaned up..."
sleep 30

echo "[DELETE] Deleting kops cluster..."
kops delete cluster --name $CLUSTER_NAME --yes

echo "[DELETE] Deleting S3 state bucket..."
aws s3 rb $KOPS_STATE_STORE --force

echo "[DELETE] Deleting ECR repositories..."
aws ecr delete-repository --repository-name atp/go-app --force --region $AWS_REGION 2>/dev/null || echo "ECR repo not found"
aws ecr delete-repository --repository-name atp/go-data --force --region $AWS_REGION 2>/dev/null || echo "ECR repo not found"
aws ecr delete-repository --repository-name atp/celery-worker --force --region $AWS_REGION 2>/dev/null || echo "ECR repo not found"
aws ecr delete-repository --repository-name atp/web --force --region $AWS_REGION 2>/dev/null || echo "ECR repo not found"

echo ""
echo "[OK] Teardown complete!"
echo "All resources have been deleted."
