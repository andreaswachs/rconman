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

echo "Deploying mock-rcon..."
kubectl run mock-rcon --image=mock-rcon:e2e --port=25575
kubectl wait --for=condition=ready pod/mock-rcon --timeout=60s

MOCK_RCON_IP=$(kubectl get pod mock-rcon -o jsonpath='{.status.podIP}')

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
  --set "config.minecraft.servers[0].id=my-server" \
  --set "config.minecraft.servers[0].name=Test Server" \
  --set "config.minecraft.servers[0].rcon.host=${MOCK_RCON_IP}" \
  --set "config.minecraft.servers[0].rcon.port=25575" \
  --set "secrets.minecraft.servers[0].id=my-server" \
  --set "secrets.minecraft.servers[0].rconPassword.value=e2e-rcon-password"

echo "Waiting for rconman pod to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=rconman --timeout=120s

echo "Port-forwarding rconman to localhost:8080..."
kubectl port-forward svc/rconman-rconman 8080:8080 &
PF_PID=$!
sleep 3

echo "Running e2e tests..."
RCONMAN_URL=http://localhost:8080 go test -v ./test/e2e/...
TEST_EXIT=$?

kill $PF_PID 2>/dev/null
echo "Setup and tests complete (exit code: $TEST_EXIT)"
exit $TEST_EXIT
