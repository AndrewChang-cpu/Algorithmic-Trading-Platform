#!/bin/bash

kubectl apply -f https://github.com/bitnami-labs/sealed-secrets/releases/download/v0.27.3/controller.yaml
kubectl apply -f . --recursive

if [ $? -eq 0 ]; then
  echo "All manifests applied successfully!"
else
  echo "An error occurred while applying the manifests."
fi