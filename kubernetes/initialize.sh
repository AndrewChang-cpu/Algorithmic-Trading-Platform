#!/bin/bash


# SEALED SECRETS
# Configuration
kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.27.3/controller.yaml

# Configuration
SSM_PARAMETER_NAME="/atp/alpaca-credentials" # Name of the Parameter Store entry
NAMESPACE="default"                          # Kubernetes namespace
SEALED_SECRET_NAME="alpaca-credentials"      # Name of the SealedSecret
PUBLIC_CERT_FILE="temp.pem"                # Temporary file to store the public certificate
SEALED_SECRET_FILE="${SEALED_SECRET_NAME}-sealed.yaml" # SealedSecret output file

# Step 1: Fetch parameter from AWS SSM Parameter Store
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

# Step 2: Create Kubernetes secret manifest
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

# Step 3: Fetch the public certificate
echo "Fetching Sealed Secrets public certificate..."
kubeseal --fetch-cert > "$PUBLIC_CERT_FILE"
if [ $? -ne 0 ]; then
  echo "Error: Failed to fetch the public certificate."
  rm "$TEMP_SECRET_FILE"
  exit 1
fi

# Step 4: Seal the secret using the fetched certificate
echo "Sealing the secret..."
kubeseal --cert "$PUBLIC_CERT_FILE" --namespace "$NAMESPACE" -o yaml < "$TEMP_SECRET_FILE" > "$SEALED_SECRET_FILE"
if [ $? -ne 0 ]; then
  echo "Error: Failed to seal the secret."
  rm "$TEMP_SECRET_FILE" "$PUBLIC_CERT_FILE"
  exit 1
fi

# Step 5: Apply the sealed secret to the cluster
echo "Applying sealed secret to the cluster..."
kubectl apply -f "$SEALED_SECRET_FILE"
if [ $? -ne 0 ]; then
  echo "Error: Failed to apply the sealed secret."
  rm "$TEMP_SECRET_FILE" "$PUBLIC_CERT_FILE" "$SEALED_SECRET_FILE"
  exit 1
fi

# Clean up temporary files
rm "$TEMP_SECRET_FILE" "$PUBLIC_CERT_FILE"

echo "Sealed secret $SEALED_SECRET_NAME successfully created and applied in namespace $NAMESPACE."


# OTHER RESOURCES
kubectl apply -f . --recursive

if [ $? -eq 0 ]; then
  echo "All manifests applied successfully!"
else
  echo "An error occurred while applying the manifests."
fi