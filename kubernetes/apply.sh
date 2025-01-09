#!/bin/bash

kubectl apply -f . --recursive

if [ $? -eq 0 ]; then
  echo "All manifests applied successfully!"
else
  echo "An error occurred while applying the manifests."
fi