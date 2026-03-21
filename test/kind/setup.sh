#!/bin/bash
set -e

echo "Creating Kind cluster..."
kind create cluster --config test/kind/kind-config.yaml

echo "Building images..."
docker build -t rconman:e2e -f Containerfile .
docker build -t mock-rcon:e2e -f test/mock-rcon/Containerfile test/mock-rcon

echo "Loading images into cluster..."
kind load docker-image rconman:e2e
kind load docker-image mock-rcon:e2e

echo "Installing Helm chart..."
helm install rconman helm/rconman \
  --set image.repository=rconman \
  --set image.tag=e2e \
  --set image.pullPolicy=Never \
  --set config.server.baseURL="http://localhost:8080" \
  --set config.server.insecureMode=true \
  --set secrets.sessionSecret.value="e2e-test-session-secret-32-bytes!!" \
  --set secrets.oidcClientID.value="e2e-client-id" \
  --set secrets.oidcClientSecret.value="e2e-client-secret" \
  --set "secrets.minecraft.servers[0].id=my-server" \
  --set "secrets.minecraft.servers[0].rconPassword.value=e2e-rcon-password"

echo "Waiting for pod to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=rconman --timeout=120s

echo "Setup complete"
