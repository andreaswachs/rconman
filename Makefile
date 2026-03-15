.PHONY: dev dev-server dev-css dev-watch test build build-css generate clean format lint e2e help install-tools

# Config file detection (prefer dev config with real credentials)
CONFIG_DEV := $(if $(wildcard config.dev.yaml),config.dev.yaml,config.example.yaml)

# Development targets
dev: generate dev-css
	@echo "🚀 Starting rconman in dev mode (with live reload)..."
	@echo "📝 Using config: $(CONFIG_DEV)"
	@echo "Make sure you have 'air' installed: go install github.com/cosmtrek/air@latest"
	air

dev-server: generate
	@echo "🚀 Starting rconman server (no watch)..."
	@echo "📝 Using config: $(CONFIG_DEV)"
	go run ./cmd/rconman -config $(CONFIG_DEV)

dev-css:
	@echo "📦 Building and watching CSS..."
	@cd web && npm run watch

dev-watch: generate
	@echo "👀 Watching Go files with air..."
	air

# Build targets
build: generate build-css
	CGO_ENABLED=0 go build -o rconman ./cmd/rconman

build-css:
	@cd web && npm run build

generate:
	go generate ./...

# Testing targets
test:
	go test -v ./...

test-watch:
	@echo "👀 Running tests in watch mode..."
	go test -v ./... -run . -count=1 2>&1 | grep -E "^(PASS|FAIL|RUN|ok|FAIL)" || go test -v ./...

e2e:
	./test/kind/setup.sh
	./test/kind/teardown.sh

# Code quality targets
format:
	gofmt -s -w .
	goimports -w .

lint:
	@echo "Running golangci-lint..."
	@command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint not found, install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

# Cleanup
clean:
	rm -f rconman rconman.db *.db-wal *.db-shm
	rm -f web/static/app.css
	go clean -cache -testcache

# Installation
install-tools:
	@echo "📥 Installing development tools..."
	go install github.com/cosmtrek/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest

# Docker targets
REGISTRY ?= ghcr.io
REPO ?= $(REGISTRY)/$(shell git remote get-url origin | sed 's/.*:\(.*\)\.git/\1/' | tr '[:upper:]' '[:lower:]')
IMAGE_TAG ?= latest

docker-build:
	docker build -t rconman:latest -f Containerfile .

docker-build-multiarch:
	docker buildx build --platform linux/amd64,linux/arm64 -t $(REPO):$(IMAGE_TAG) -f Containerfile .

docker-run: docker-build
	@echo "🐳 Starting rconman in Docker on http://localhost:8080"
	@echo "Note: Using dev credentials. Do NOT use in production."
	docker run -p 8080:8080 \
		-v $(PWD)/config.dev.yaml:/etc/rconman/config.yaml \
		--entrypoint /rconman \
		rconman:latest \
		-config /etc/rconman/config.yaml

# Hot-reloading Docker development with CompileDaemon
docker-build-dev:
	docker build -f Dockerfile.dev -t rconman:dev .

docker-dev: docker-build-dev
	@echo "🔥 Starting rconman in Docker with hot-reload (CompileDaemon)"
	@echo "Watching for changes in Go files..."
	@echo "Server: http://localhost:8080"
	@echo "Press Ctrl+C to stop"
	docker run --rm -p 8080:8080 \
		-v $(PWD):/app \
		-v $(PWD)/config.dev.yaml:/etc/rconman/config.yaml \
		rconman:dev

docker-tag: docker-build
	docker tag rconman:latest $(REPO):$(IMAGE_TAG)
	@echo "Tagged image: $(REPO):$(IMAGE_TAG)"

docker-push: docker-tag
	@echo "Pushing to $(REPO):$(IMAGE_TAG)..."
	docker push $(REPO):$(IMAGE_TAG)
	@echo "✅ Image pushed to $(REPO):$(IMAGE_TAG)"

docker-push-latest: docker-tag
	docker tag rconman:latest $(REPO):latest
	docker push $(REPO):latest
	docker push $(REPO):$(IMAGE_TAG)
	@echo "✅ Pushed $(REPO):latest and $(REPO):$(IMAGE_TAG)"

docker-buildx-push:
	@echo "Building and pushing multiarch image to $(REPO):$(IMAGE_TAG)..."
	docker buildx build --platform linux/amd64,linux/arm64 -t $(REPO):$(IMAGE_TAG) --push -f Containerfile .
	@echo "✅ Multiarch image pushed to $(REPO):$(IMAGE_TAG)"

# Helm targets
helm-lint:
	helm lint helm/rconman

helm-template:
	helm template rconman helm/rconman

# Help
help:
	@echo "🎮 rconman - Development Makefile"
	@echo ""
	@echo "Development targets:"
	@echo "  make dev              - Start app with live reload (requires 'air')"
	@echo "  make dev-server       - Start app without file watching"
	@echo "  make dev-css          - Watch and rebuild Tailwind CSS"
	@echo "  make dev-watch        - Watch Go files and rebuild"
	@echo ""
	@echo "Build targets:"
	@echo "  make build            - Build binary (generate + CSS + compile)"
	@echo "  make build-css        - Build CSS only"
	@echo "  make generate         - Run go generate"
	@echo ""
	@echo "Testing targets:"
	@echo "  make test             - Run full test suite"
	@echo "  make test-watch       - Run tests in watch mode"
	@echo "  make e2e              - Run e2e tests (Kind cluster)"
	@echo ""
	@echo "Code quality targets:"
	@echo "  make format           - Format code with gofmt and goimports"
	@echo "  make lint             - Run golangci-lint"
	@echo ""
	@echo "Infrastructure targets:"
	@echo "  make docker-build          - Build Docker image locally"
	@echo "  make docker-run            - Build and run Docker image"
	@echo "  make docker-build-dev      - Build Docker dev image with hot-reload"
	@echo "  make docker-dev            - Run Docker dev with CompileDaemon (hot-reload)"
	@echo "  make docker-tag            - Build and tag image for GHCR"
	@echo "  make docker-push           - Build and push image to GHCR"
	@echo "  make docker-push-latest    - Push both latest and versioned tags"
	@echo "  make docker-buildx-push    - Build multiarch (amd64/arm64) and push"
	@echo "  make helm-lint             - Lint Helm chart"
	@echo "  make helm-template         - Render Helm chart templates"
	@echo ""
	@echo "Utilities:"
	@echo "  make clean            - Remove build artifacts"
	@echo "  make install-tools    - Install development tools"
	@echo ""
	@echo "Quick start:"
	@echo "  1. make install-tools (one-time setup)"
	@echo "  2. make dev           (start developing)"
