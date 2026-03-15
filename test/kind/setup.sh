#!/bin/bash
set -e

echo "Creating Kind cluster..."
kind create cluster --config test/kind/kind-config.yaml

echo "Building images..."
docker build -t rconman:e2e -f Containerfile .
docker build -t mock-rcon:e2e test/mock-rcon

echo "Loading images into cluster..."
kind load docker-image rconman:e2e
kind load docker-image mock-rcon:e2e

echo "Installing Helm chart..."
helm install rconman helm/rconman \
  --set image.repository=rconman \
  --set image.tag=e2e

echo "Waiting for pod to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=rconman --timeout=120s

echo "Setup complete"
