# GPU Telemetry Pipeline Makefile

# Variables
APP_NAME := gpu-telemetry-pipeline
VERSION := 1.0.0
GO := go
DOCKER := docker

BUILD_DIR := ./bin
COVERAGE_DIR := ./coverage
DOCKER_REGISTRY ?= 
IMAGE_TAG ?= $(VERSION)
KIND_CLUSTER := gpu-telemetry
CSV_FILE := dcgm_metrics_20250718_134233.csv

.PHONY: all build test clean docker-build load-kind k8s-deploy k8s-delete kind-setup kind-delete

# ============================================
# Build Targets
# ============================================

## build: Build all Go binaries
build:
	@echo "Building binaries..."
	$(GO) build -o $(BUILD_DIR)/api ./cmd/api
	$(GO) build -o $(BUILD_DIR)/mq-server ./cmd/mq-server
	$(GO) build -o $(BUILD_DIR)/streamer ./cmd/streamer
	$(GO) build -o $(BUILD_DIR)/collector ./cmd/collector

## tidy: Install Go dependencies
tidy:
	$(GO) mod tidy

# ============================================
# Docker Targets
# ============================================

## docker-build: Build all Docker images
docker-build:
	@echo "Building Docker images..."
	$(DOCKER) build -t $(APP_NAME)/api:$(IMAGE_TAG) -f deployments/docker/api.Dockerfile .
	$(DOCKER) build -t $(APP_NAME)/mq-server:$(IMAGE_TAG) -f deployments/docker/mq-server.Dockerfile .
	$(DOCKER) build -t $(APP_NAME)/streamer:$(IMAGE_TAG) -f deployments/docker/streamer.Dockerfile .
	$(DOCKER) build -t $(APP_NAME)/collector:$(IMAGE_TAG) -f deployments/docker/collector.Dockerfile .

# ============================================
# KIND Targets
# ============================================

## kind-setup: Create KIND cluster, load images, copy CSV, and deploy
kind-setup: docker-build
	@echo "Creating KIND cluster with port mappings..."
	kind create cluster --name $(KIND_CLUSTER) --config deployments/kind/kind-config.yaml || true
	@echo "Loading images into KIND cluster..."
	kind load docker-image $(APP_NAME)/api:$(IMAGE_TAG) --name $(KIND_CLUSTER)
	kind load docker-image $(APP_NAME)/mq-server:$(IMAGE_TAG) --name $(KIND_CLUSTER)
	kind load docker-image $(APP_NAME)/streamer:$(IMAGE_TAG) --name $(KIND_CLUSTER)
	kind load docker-image $(APP_NAME)/collector:$(IMAGE_TAG) --name $(KIND_CLUSTER)
	@echo "Copying CSV data to cluster node..."
	docker exec $(KIND_CLUSTER)-control-plane mkdir -p /data
	docker cp $(CSV_FILE) $(KIND_CLUSTER)-control-plane:/data/dcgm_metrics.csv
	@echo "Creating namespace..."
	kubectl apply -f deployments/kubernetes/namespace.yaml
	@echo "Waiting for namespace to be ready..."
	sleep 3
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deployments/kubernetes/
	@echo "KIND cluster setup complete!"
	@echo ""
	@echo "Access URLs:"
	@echo "  API:      http://localhost:30080"
	@echo "  InfluxDB: http://localhost:30086"

## kind-delete: Delete KIND cluster
kind-delete:
	@echo "Deleting KIND cluster..."
	kind delete cluster --name $(KIND_CLUSTER)

# ============================================
# Kubernetes Targets
# ============================================

## k8s-deploy: Deploy to Kubernetes
k8s-deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f deployments/kubernetes/

## k8s-delete: Delete from Kubernetes
k8s-delete:
	@echo "Deleting from Kubernetes..."
	kubectl delete namespace gpu-telemetry --ignore-not-found

## k8s-status: Show Kubernetes status
k8s-status:
	kubectl get pods -n gpu-telemetry
	kubectl get svc -n gpu-telemetry

## k8s-reset-influxdb: Reset InfluxDB (delete pod to clear all data)
k8s-reset-influxdb:
	@echo "Resetting InfluxDB (deleting deployment and PVC)..."
	kubectl delete deployment influxdb -n gpu-telemetry --ignore-not-found
	kubectl delete pvc influxdb-pvc -n gpu-telemetry --ignore-not-found
	@echo "Recreating InfluxDB..."
	kubectl apply -f deployments/kubernetes/influxdb.yaml
	@echo "Waiting for InfluxDB to be ready..."
	kubectl wait --for=condition=ready pod -l app=influxdb -n gpu-telemetry --timeout=120s
	@echo "InfluxDB reset complete!"

# ============================================
# Test Targets
# ============================================

## test: Run tests
test:
	$(GO) test -v ./...

## coverage: Run tests with coverage
coverage:
	@mkdir -p $(COVERAGE_DIR)
	$(GO) test -coverprofile=$(COVERAGE_DIR)/coverage.out ./...
	$(GO) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html

## integration-test: Run integration tests against deployed system
integration-test:
	@echo "Running integration tests..."
	@if [ -f tests/integration_test.sh ]; then \
		chmod +x tests/integration_test.sh && \
		./tests/integration_test.sh; \
	elif [ -f tests/integration_test.ps1 ]; then \
		powershell -ExecutionPolicy Bypass -File tests/integration_test.ps1; \
	else \
		echo "Integration test script not found"; \
		exit 1; \
	fi

## integration-test-kind: Deploy to KIND and run integration tests
integration-test-kind: kind-setup
	@echo "Waiting for system to be ready..."
	@sleep 30
	@echo "Running integration tests..."
	@$(MAKE) integration-test

# ============================================
# Clean Targets
# ============================================

## clean: Remove build artifacts
clean:
	rm -rf $(BUILD_DIR) $(COVERAGE_DIR)

## clean-docker: Remove Docker images
clean-docker:
	$(DOCKER) rmi $(APP_NAME)/api:$(IMAGE_TAG) || true
	$(DOCKER) rmi $(APP_NAME)/mq-server:$(IMAGE_TAG) || true
	$(DOCKER) rmi $(APP_NAME)/streamer:$(IMAGE_TAG) || true
	$(DOCKER) rmi $(APP_NAME)/collector:$(IMAGE_TAG) || true

# ============================================
# Help
# ============================================

## helm-install: Install using Helm charts
helm-install:
	@echo "Installing with Helm..."
	helm install telemetry-pipeline ./helm/telemetry-pipeline

## openapi-gen: Generate OpenAPI (Swagger) spec
openapi-gen:
	@echo "Generating OpenAPI spec..."
	swagger generate spec -o ./docs/swagger.json --scan-models

## port-forward-api: Port-forward API service to localhost:30080
port-forward-api:
	kubectl port-forward svc/api 30080:8080 -n gpu-telemetry

## port-forward-influxdb: Port-forward InfluxDB to localhost:30086
port-forward-influxdb:
	kubectl port-forward svc/influxdb 30086:8086 -n gpu-telemetry

## helm-upgrade: Upgrade Helm deployment
helm-upgrade:
	@echo "Upgrading Helm deployment..."
	helm upgrade telemetry-pipeline ./helm/telemetry-pipeline

## helm-uninstall: Uninstall Helm deployment
helm-uninstall:
	@echo "Uninstalling Helm deployment..."
	helm uninstall telemetry-pipeline

## helm-template: Render Helm templates (for debugging)
helm-template:
	@echo "Rendering Helm templates..."
	helm template telemetry-pipeline ./helm/telemetry-pipeline

## help: Show this help
help:
	@echo "Available targets:"
	@echo ""
	@echo "  kind-setup         - Create KIND cluster, build, load images, copy CSV, deploy (full setup)"
	@echo "  kind-delete        - Delete KIND cluster"
	@echo "  build              - Build Go binaries only"
	@echo "  docker-build       - Build Docker images only"
	@echo "  load-kind          - Load Docker images into KIND cluster"
	@echo "  k8s-deploy         - Deploy to Kubernetes"
	@echo "  k8s-delete         - Delete from Kubernetes"
	@echo "  k8s-status        - Show Kubernetes pod/service status"
	@echo "  test               - Run unit tests"
	@echo "  coverage           - Run tests with coverage"
	@echo "  integration-test   - Run integration tests (requires deployed system)"
	@echo "  integration-test-kind - Deploy to KIND and run integration tests"
	@echo "  helm-install       - Install using Helm charts"
	@echo "  helm-upgrade       - Upgrade Helm deployment"
	@echo "  helm-uninstall     - Uninstall Helm deployment"
	@echo "  helm-template      - Render Helm templates (for debugging)"
	@echo "  clean              - Remove build artifacts"
	@echo ""
