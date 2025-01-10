#!/bin/bash


# SEALED SECRETS
# Configuration
SSM_PARAMETER_NAME="/atp/alpaca-credentials" # Name of the Parameter Store entry
NAMESPACE="default"                          # Kubernetes namespace
SEALED_SECRET_NAME="alpaca-credentials"      # Name of the SealedSecret
SEALED_SECRET_PUBLIC_KEY="sealed-secrets.pub" # Path to the retrieved public key

# Step 1: Retrieve the Sealed Secrets public key
echo "Retrieving Sealed Secrets public key from the cluster..."
kubectl get secret -n kube-system sealed-secrets-key -o jsonpath="{.data.tls.crt}" | base64 --decode > "$SEALED_SECRET_PUBLIC_KEY"
if [ $? -ne 0 ]; then
  echo "Error: Failed to retrieve the Sealed Secrets public key."
  exit 1
fi
echo "Sealed Secrets public key saved to $SEALED_SECRET_PUBLIC_KEY."

# Step 2: Fetch parameter from AWS SSM Parameter Store
echo "Fetching parameter from AWS Parameter Store..."
PARAMETER_VALUE=$(aws ssm get-parameter --name "$SSM_PARAMETER_NAME" --query "Parameter.Value" --output text)
if [ $? -ne 0 ]; then
  echo "Error: Failed to fetch parameter from AWS Parameter Store."
  exit 1
fi

# Parameter value is JSON-encoded
echo "Parsing parameter value..."
ALPACA_API_KEY=$(echo "$PARAMETER_VALUE" | jq -r '.ALPACA_API_KEY')
ALPACA_API_SECRET=$(echo "$PARAMETER_VALUE" | jq -r '.ALPACA_API_SECRET')

if [ -z "$ALPACA_API_KEY" ] || [ -z "$ALPACA_API_SECRET" ]; then
  echo "Error: Missing ALPACA_API_KEY or ALPACA_API_SECRET in the parameter value."
  exit 1
fi

# Step 3: Create Kubernetes secret manifest
echo "Creating Kubernetes secret manifest..."
SECRET_MANIFEST=$(cat <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: $SEALED_SECRET_NAME
  namespace: $NAMESPACE
type: Opaque
data:
  ALPACA_API_KEY: $(echo -n "$ALPACA_API_KEY" | base64)
  ALPACA_API_SECRET: $(echo -n "$ALPACA_API_SECRET" | base64)
EOF
)

# Save the Kubernetes secret manifest to a temporary file
TEMP_SECRET_FILE=$(mktemp)
echo "$SECRET_MANIFEST" > "$TEMP_SECRET_FILE"

# Step 4: Seal the secret
echo "Sealing the secret..."
SEALED_SECRET_MANIFEST=$(kubeseal --cert "$SEALED_SECRET_PUBLIC_KEY" --namespace "$NAMESPACE" -o yaml < "$TEMP_SECRET_FILE")
if [ $? -ne 0 ]; then
  echo "Error: Failed to seal the secret."
  rm "$TEMP_SECRET_FILE"
  rm "$SEALED_SECRET_PUBLIC_KEY"
  exit 1
fi

# Step 5: Apply the sealed secret to the cluster
SEALED_SECRET_FILE="${SEALED_SECRET_NAME}-sealed.yaml"
echo "$SEALED_SECRET_MANIFEST" > "$SEALED_SECRET_FILE"
echo "Applying sealed secret to the cluster..."
kubectl apply -f "$SEALED_SECRET_FILE"
if [ $? -ne 0 ]; then
  echo "Error: Failed to apply the sealed secret."
  rm "$TEMP_SECRET_FILE"
  rm "$SEALED_SECRET_PUBLIC_KEY"
  exit 1
fi

# Clean up temporary files
rm "$TEMP_SECRET_FILE"
rm "$SEALED_SECRET_PUBLIC_KEY"

echo "Sealed secret $SEALED_SECRET_NAME successfully created and applied in namespace $NAMESPACE."


# OTHER RESOURCES
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.27.3/controller.yaml
kubectl apply -f . --recursive

if [ $? -eq 0 ]; then
  echo "All manifests applied successfully!"
else
  echo "An error occurred while applying the manifests."
fi