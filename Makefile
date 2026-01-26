# GPU Telemetry Pipeline Makefile
# Provides build, test, coverage, and deployment commands

# Variables
APP_NAME := gpu-telemetry-pipeline
VERSION := 1.0.0
GO := go
GOFLAGS := -v
DOCKER := docker
HELM := helm

# Output directories
BUILD_DIR := ./bin
COVERAGE_DIR := ./coverage

# Docker image settings
DOCKER_REGISTRY ?= 
IMAGE_TAG ?= $(VERSION)

# Go source directories
CMD_DIRS := ./cmd/...
PKG_DIRS := ./pkg/...
INTERNAL_DIRS := ./internal/...
ALL_DIRS := $(CMD_DIRS) $(PKG_DIRS) $(INTERNAL_DIRS)

# Color output
GREEN := \033[0;32m
NC := \033[0m

.PHONY: all build test coverage clean docker helm swagger lint fmt vet help buildproject

# Default target
all: fmt vet lint test build

#
# One-command build target
#

## buildproject: Complete project build - installs deps, builds binaries, builds Docker images
buildproject: tidy swagger build docker-build
	@echo "$(GREEN)========================================$(NC)"
	@echo "$(GREEN)Project build complete!$(NC)"
	@echo "$(GREEN)========================================$(NC)"
	@echo ""
	@echo "Built binaries in ./bin:"
	@echo "  - bin/api"
	@echo "  - bin/mq-server"
	@echo "  - bin/streamer"
	@echo "  - bin/collector"
	@echo ""
	@echo "Built Docker images:"
	@echo "  - $(APP_NAME)/api:$(IMAGE_TAG)"
	@echo "  - $(APP_NAME)/mq-server:$(IMAGE_TAG)"
	@echo "  - $(APP_NAME)/streamer:$(IMAGE_TAG)"
	@echo "  - $(APP_NAME)/collector:$(IMAGE_TAG)"
	@echo ""
	@echo "To run with Docker Compose:"
	@echo "  docker-compose up -d"
	@echo ""

#
# Build targets
#

## build: Build all binaries
build: build-api build-mq-server build-streamer build-collector

## build-api: Build the API server binary
build-api:
	@echo "$(GREEN)Building API server...$(NC)"
	$(GO) build $(GOFLAGS) -ldflags="-X main.version=$(VERSION)" -o $(BUILD_DIR)/api ./cmd/api

## build-mq-server: Build the MQ server binary
build-mq-server:
	@echo "$(GREEN)Building MQ server...$(NC)"
	$(GO) build $(GOFLAGS) -ldflags="-X main.version=$(VERSION)" -o $(BUILD_DIR)/mq-server ./cmd/mq-server

## build-streamer: Build the streamer binary
build-streamer:
	@echo "$(GREEN)Building streamer...$(NC)"
	$(GO) build $(GOFLAGS) -ldflags="-X main.version=$(VERSION)" -o $(BUILD_DIR)/streamer ./cmd/streamer

## build-collector: Build the collector binary
build-collector:
	@echo "$(GREEN)Building collector...$(NC)"
	$(GO) build $(GOFLAGS) -ldflags="-X main.version=$(VERSION)" -o $(BUILD_DIR)/collector ./cmd/collector

#
# Test targets
#

## test: Run all unit tests
test:
	@echo "$(GREEN)Running tests...$(NC)"
	$(GO) test -v -race $(ALL_DIRS)

## test-short: Run tests without verbose output
test-short:
	$(GO) test -race $(ALL_DIRS)

## coverage: Run tests with coverage report
coverage:
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -v -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic $(ALL_DIRS)
	$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	$(GO) tool cover -func=$(COVERAGE_DIR)/coverage.out
	@echo "$(GREEN)Coverage report generated at $(COVERAGE_DIR)/coverage.html$(NC)"

## coverage-summary: Show coverage summary
coverage-summary:
	@$(GO) test -coverprofile=$(COVERAGE_DIR)/coverage.out $(ALL_DIRS) > /dev/null 2>&1
	@$(GO) tool cover -func=$(COVERAGE_DIR)/coverage.out | grep total

#
# Code quality targets
#

## fmt: Format Go code
fmt:
	@echo "$(GREEN)Formatting code...$(NC)"
	$(GO) fmt $(ALL_DIRS)

## vet: Run go vet
vet:
	@echo "$(GREEN)Running go vet...$(NC)"
	$(GO) vet $(ALL_DIRS)

## lint: Run golangci-lint (requires golangci-lint to be installed)
lint:
	@echo "$(GREEN)Running linter...$(NC)"
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run $(ALL_DIRS); \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

## tidy: Tidy Go modules
tidy:
	$(GO) mod tidy

## vendor: Vendor dependencies
vendor:
	$(GO) mod vendor

#
# Swagger/OpenAPI targets
#

## swagger: Generate Swagger/OpenAPI documentation
swagger:
	@echo "$(GREEN)Generating Swagger documentation...$(NC)"
	@if command -v swag > /dev/null; then \
		swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal; \
		echo "Swagger docs generated in ./docs"; \
	else \
		echo "swag not installed. Install with: go install github.com/swaggo/swag/cmd/swag@latest"; \
		exit 1; \
	fi

## swagger-fmt: Format Swagger comments
swagger-fmt:
	@if command -v swag > /dev/null; then \
		swag fmt; \
	fi

#
# Docker targets
#

## docker-build: Build all Docker images
docker-build: docker-build-api docker-build-mq-server docker-build-streamer docker-build-collector

## docker-build-api: Build API Docker image
docker-build-api:
	@echo "$(GREEN)Building API Docker image...$(NC)"
	$(DOCKER) build -t $(DOCKER_REGISTRY)$(APP_NAME)/api:$(IMAGE_TAG) -f deployments/docker/api.Dockerfile .

## docker-build-mq-server: Build MQ server Docker image
docker-build-mq-server:
	@echo "$(GREEN)Building MQ server Docker image...$(NC)"
	$(DOCKER) build -t $(DOCKER_REGISTRY)$(APP_NAME)/mq-server:$(IMAGE_TAG) -f deployments/docker/mq-server.Dockerfile .

## docker-build-streamer: Build streamer Docker image
docker-build-streamer:
	@echo "$(GREEN)Building streamer Docker image...$(NC)"
	$(DOCKER) build -t $(DOCKER_REGISTRY)$(APP_NAME)/streamer:$(IMAGE_TAG) -f deployments/docker/streamer.Dockerfile .

## docker-build-collector: Build collector Docker image
docker-build-collector:
	@echo "$(GREEN)Building collector Docker image...$(NC)"
	$(DOCKER) build -t $(DOCKER_REGISTRY)$(APP_NAME)/collector:$(IMAGE_TAG) -f deployments/docker/collector.Dockerfile .

## docker-push: Push all Docker images
docker-push:
	@echo "$(GREEN)Pushing Docker images...$(NC)"
	$(DOCKER) push $(DOCKER_REGISTRY)$(APP_NAME)/api:$(IMAGE_TAG)
	$(DOCKER) push $(DOCKER_REGISTRY)$(APP_NAME)/mq-server:$(IMAGE_TAG)
	$(DOCKER) push $(DOCKER_REGISTRY)$(APP_NAME)/streamer:$(IMAGE_TAG)
	$(DOCKER) push $(DOCKER_REGISTRY)$(APP_NAME)/collector:$(IMAGE_TAG)

#
# Helm targets
#

## helm-lint: Lint Helm chart
helm-lint:
	@echo "$(GREEN)Linting Helm chart...$(NC)"
	$(HELM) lint deployments/helm/gpu-telemetry-pipeline

## helm-template: Render Helm templates
helm-template:
	@echo "$(GREEN)Rendering Helm templates...$(NC)"
	$(HELM) template gpu-telemetry deployments/helm/gpu-telemetry-pipeline

## helm-install: Install Helm chart
helm-install:
	@echo "$(GREEN)Installing Helm chart...$(NC)"
	$(HELM) install gpu-telemetry deployments/helm/gpu-telemetry-pipeline

## helm-upgrade: Upgrade Helm release
helm-upgrade:
	$(HELM) upgrade gpu-telemetry deployments/helm/gpu-telemetry-pipeline

## helm-uninstall: Uninstall Helm release
helm-uninstall:
	$(HELM) uninstall gpu-telemetry

## helm-package: Package Helm chart
helm-package:
	@echo "$(GREEN)Packaging Helm chart...$(NC)"
	$(HELM) package deployments/helm/gpu-telemetry-pipeline -d $(BUILD_DIR)

#
# KIND deployment targets
#

KIND := kind
KIND_CLUSTER_NAME := gpu-telemetry
NAMESPACE := gpu-telemetry
INFLUXDB_TOKEN := my-super-secret-token
CSV_FILE ?= 

## kind-create: Create KIND cluster
kind-create:
	@echo "$(GREEN)Creating KIND cluster...$(NC)"
	$(KIND) create cluster --name $(KIND_CLUSTER_NAME) --config deployments/kind/kind-config.yaml

## kind-delete: Delete KIND cluster
kind-delete:
	@echo "$(GREEN)Deleting KIND cluster...$(NC)"
	$(KIND) delete cluster --name $(KIND_CLUSTER_NAME)

## kind-load: Load Docker images into KIND
kind-load: docker-build
	@echo "$(GREEN)Loading images into KIND...$(NC)"
	$(KIND) load docker-image $(DOCKER_REGISTRY)$(APP_NAME)/api:$(IMAGE_TAG) --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image $(DOCKER_REGISTRY)$(APP_NAME)/mq-server:$(IMAGE_TAG) --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image $(DOCKER_REGISTRY)$(APP_NAME)/streamer:$(IMAGE_TAG) --name $(KIND_CLUSTER_NAME)
	$(KIND) load docker-image $(DOCKER_REGISTRY)$(APP_NAME)/collector:$(IMAGE_TAG) --name $(KIND_CLUSTER_NAME)

## kind-deploy-infra: Deploy InfluxDB to KIND
kind-deploy-infra:
	@echo "$(GREEN)Deploying InfluxDB...$(NC)"
	kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f deployments/kind/influxdb.yaml -n $(NAMESPACE)
	kubectl wait --for=condition=ready pod -l app=influxdb -n $(NAMESPACE) --timeout=120s

## kind-create-configmap: Create ConfigMap with CSV data (use CSV_FILE=/path/to/file.csv)
kind-create-configmap:
	@if [ -z "$(CSV_FILE)" ]; then \
		echo "ERROR: CSV_FILE is required. Usage: make kind-create-configmap CSV_FILE=/path/to/telemetry.csv"; \
		exit 1; \
	fi
	@echo "$(GREEN)Creating ConfigMap with CSV data (first 1000 lines)...$(NC)"
	@head -1000 $(CSV_FILE) > /tmp/telemetry_sample.csv
	kubectl create configmap $(APP_NAME)-data --from-file=telemetry.csv=/tmp/telemetry_sample.csv -n $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@rm /tmp/telemetry_sample.csv

## kind-deploy-helm: Deploy application with Helm
kind-deploy-helm:
	@echo "$(GREEN)Deploying application with Helm...$(NC)"
	$(HELM) upgrade --install gpu-telemetry deployments/helm/gpu-telemetry-pipeline \
		--namespace $(NAMESPACE) \
		--set influxdb.url="http://influxdb:8086" \
		--set influxdb.token="$(INFLUXDB_TOKEN)" \
		--set influxdb.org="cisco" \
		--set influxdb.bucket="gpu_telemetry"

## kind-deploy: Full deployment to KIND (requires CSV_FILE)
kind-deploy: kind-create kind-load kind-deploy-infra kind-create-configmap kind-deploy-helm
	@echo "$(GREEN)Waiting for pods to be ready...$(NC)"
	kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=gpu-telemetry -n $(NAMESPACE) --timeout=180s
	@echo ""
	@echo "$(GREEN)========================================$(NC)"
	@echo "$(GREEN)Deployment complete!$(NC)"
	@echo "$(GREEN)========================================$(NC)"
	@echo "API Swagger UI: http://localhost:8080/swagger/index.html"
	@echo "API GPUs:       http://localhost:8080/api/v1/gpus"
	@echo "InfluxDB UI:    http://localhost:8086"
	@echo ""

## kind-status: Show KIND deployment status
kind-status:
	@echo "Pods:"
	@kubectl get pods -n $(NAMESPACE)
	@echo ""
	@echo "Services:"
	@kubectl get svc -n $(NAMESPACE)

## kind-logs: View logs of all components
kind-logs:
	@echo "=== MQ Server ===" && kubectl logs -l app.kubernetes.io/component=mq-server -n $(NAMESPACE) --tail=20 || true
	@echo "=== Collector ===" && kubectl logs -l app.kubernetes.io/component=collector -n $(NAMESPACE) --tail=20 || true
	@echo "=== Streamer ===" && kubectl logs -l app.kubernetes.io/component=streamer -n $(NAMESPACE) --tail=20 || true
	@echo "=== API ===" && kubectl logs -l app.kubernetes.io/component=api -n $(NAMESPACE) --tail=20 || true

## kind-logs-api: View API logs
kind-logs-api:
	kubectl logs -l app.kubernetes.io/component=api -n $(NAMESPACE) -f

## kind-logs-collector: View collector logs
kind-logs-collector:
	kubectl logs -l app.kubernetes.io/component=collector -n $(NAMESPACE) -f

## kind-logs-streamer: View streamer logs
kind-logs-streamer:
	kubectl logs -l app.kubernetes.io/component=streamer -n $(NAMESPACE) -f

## kind-logs-mq: View MQ server logs
kind-logs-mq:
	kubectl logs -l app.kubernetes.io/component=mq-server -n $(NAMESPACE) -f

## kind-stop: Stop all deployments (scale to 0)
kind-stop:
	@echo "$(GREEN)Stopping all deployments...$(NC)"
	kubectl scale deployment --all --replicas=0 -n $(NAMESPACE)

## kind-start: Start all deployments (scale to 1)
kind-start:
	@echo "$(GREEN)Starting all deployments...$(NC)"
	kubectl scale deployment --all --replicas=1 -n $(NAMESPACE)

## kind-restart: Restart all deployments
kind-restart:
	@echo "$(GREEN)Restarting all deployments...$(NC)"
	kubectl rollout restart deployment --all -n $(NAMESPACE)

## kind-redeploy: Rebuild images and redeploy (without recreating cluster)
kind-redeploy: docker-build kind-load
	@echo "$(GREEN)Redeploying application...$(NC)"
	kubectl rollout restart deployment --all -n $(NAMESPACE)
	kubectl rollout status deployment -l app.kubernetes.io/instance=gpu-telemetry -n $(NAMESPACE)

## kind-clean: Delete KIND cluster and clean up
kind-clean: kind-delete clean-docker

#
# Run targets (for local development)
#

## run-mq: Run MQ server locally
run-mq: build-mq-server
	@echo "$(GREEN)Starting MQ server...$(NC)"
	$(BUILD_DIR)/mq-server

## run-api: Run API server locally
run-api: build-api
	@echo "$(GREEN)Starting API server...$(NC)"
	$(BUILD_DIR)/api --mode debug

## run-streamer: Run streamer locally (requires CSV_PATH env var)
run-streamer: build-streamer
	@echo "$(GREEN)Starting streamer...$(NC)"
	$(BUILD_DIR)/streamer

## run-collector: Run collector locally
run-collector: build-collector
	@echo "$(GREEN)Starting collector...$(NC)"
	$(BUILD_DIR)/collector

#
# Clean targets
#

## clean: Clean build artifacts
clean:
	@echo "$(GREEN)Cleaning build artifacts...$(NC)"
	rm -rf $(BUILD_DIR)
	rm -rf $(COVERAGE_DIR)
	rm -rf vendor/

## clean-docker: Remove Docker images
clean-docker:
	$(DOCKER) rmi $(DOCKER_REGISTRY)$(APP_NAME)/api:$(IMAGE_TAG) || true
	$(DOCKER) rmi $(DOCKER_REGISTRY)$(APP_NAME)/mq-server:$(IMAGE_TAG) || true
	$(DOCKER) rmi $(DOCKER_REGISTRY)$(APP_NAME)/streamer:$(IMAGE_TAG) || true
	$(DOCKER) rmi $(DOCKER_REGISTRY)$(APP_NAME)/collector:$(IMAGE_TAG) || true

#
# Development tools installation
#

## install-tools: Install development tools
install-tools:
	@echo "$(GREEN)Installing development tools...$(NC)"
	$(GO) install github.com/swaggo/swag/cmd/swag@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

#
# Help
#

## help: Show this help message
help:
	@echo "GPU Telemetry Pipeline - Available targets:"
	@echo ""
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/  /'
