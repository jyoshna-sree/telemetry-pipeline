# Elastic GPU Telemetry Pipeline with Message Queue

A high-performance, scalable GPU telemetry collection and streaming pipeline built in Go for AI cluster monitoring. This system processes DCGM (Data Center GPU Manager) metrics from NVIDIA GPUs, providing real-time telemetry streaming, time-series storage with InfluxDB, and a REST API for data access.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Components](#components)
- [Prerequisites](#prerequisites)
- [Build Instructions](#build-instructions)
- [Installation](#installation)
  - [Local Development](#local-development)
  - [Docker Deployment](#docker-deployment)
  - [Kubernetes Deployment](#kubernetes-deployment)
- [Configuration](#configuration)
- [API Reference](#api-reference)
- [Sample User Workflows](#sample-user-workflows)
- [Testing](#testing)
- [AI Usage Documentation](#ai-usage-documentation)

---

## Architecture Overview

```
                                GPU Telemetry Pipeline Architecture
                                ===================================

  ┌───────────────────────────────────────────────────────────────┐
  │                    Telemetry Streamers                        │
  │   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
  │   │ Streamer-1  │  │ Streamer-2  │  │ Streamer-N  │          │
  │   │ (GPU Rack 1)│  │ (GPU Rack 2)│  │ (GPU Rack N)│          │
  │   │   CSV-1     │  │   CSV-2     │  │   CSV-N     │          │
  │   └──────┬──────┘  └──────┬──────┘  └──────┬──────┘          │
  │          │                │                │                  │
  │   Multiple streamers can publish to the same MQ               │
  └──────────┼────────────────┼────────────────┼──────────────────┘
             │                │                │
             └────────────────┼────────────────┘
                              ▼
     ┌───────────────────────────────────────────────────────────┐     ┌─────────────────┐
     │              Custom Log-Based Message Queue                │     │                 │
     │                                                           │     │   REST API      │
     │  ┌─────────────────────────────────────────────────────┐ │     │   Gateway       │
     │  │              Message Log (Dynamic Slice)             │ │     │                 │
     │  │  [msg0][msg1][msg2][msg3][msg4][msg5][msg6]...      │ │     │  Swagger UI     │
     │  │    ↑                              ↑                  │ │     │  /swagger/*     │
     │  │    │                              │                  │ │     │                 │
     │  │  offset=0                    offset=6 (latest)      │ │     │  /api/v1/*      │
     │  └─────────────────────────────────────────────────────┘ │     └────────┬────────┘
     │                                                           │              │
     │  ┌────────────────────┐      ┌────────────────────────┐  │              │
     │  │   TCP Server       │      │    HTTP Server         │  │              │
     │  │   Port: 9000       │      │    Port: 9001          │  │              │
     │  │   - Publish        │      │    - /health           │  │              │
     │  │   - Subscribe      │      │    - /stats            │  │              │
     │  │   (with offset)    │      │                        │  │              │
     │  └────────────────────┘      └────────────────────────┘  │              │
     │                                                           │              │
     └───────────────────────────────────────────────────────────┘              │
                              │                                                 │
                              │ Subscribe (offset-based)                        │
                              ▼                                                 ▼
  ┌───────────────────────────────────────────────────────────────┐ ┌─────────────────────────────┐
  │                    Telemetry Collectors                       │ │                             │
  │   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │ │      InfluxDB               │
  │   │ Collector-1 │  │ Collector-2 │  │ Collector-N │          │ │  (Time-Series DB)           │
  │   │  offset=0   │  │  offset=0   │  │  offset=3   │          │▶│                             │
  │   │ (earliest)  │  │ (earliest)  │  │ (specific)  │          │ │  - GPU Metrics Store        │
  │   └─────────────┘  └─────────────┘  └─────────────┘          │ │  - Time-based Queries       │
  │                                                               │ │  - Data Retention           │
  │   Each collector reads ALL messages independently             │ │                             │
  └───────────────────────────────────────────────────────────────┘ └─────────────────────────────┘
```

### Data Flow

1. **Telemetry Streamers** (1 or more) read GPU metrics from CSV files containing DCGM exporter data
2. Each streamer collects metrics locally in a buffer for a configurable interval (default: 5s)
3. Batched metrics are published to the **Custom Message Queue** over TCP (all streamers publish to the same MQ)
4. The MQ appends messages to a log-based structure (dynamic slice) - messages from all streamers are interleaved
5. **Telemetry Collectors** (1 or more) subscribe with offset support - each collector reads ALL messages independently
6. Collectors persist metrics to **InfluxDB** (time-series database) for efficient time-based queries
7. **API Gateway** provides REST endpoints for querying stored telemetry data from InfluxDB

### Key Design Decisions

- **Custom Log-Based MQ**: Built from scratch using a dynamic slice (append-only log). No external MQ dependencies. Uses TCP protocol with length-prefixed JSON messages.
- **Multiple Streamers**: Any number of streamers can publish to the same MQ simultaneously. Each streamer can read from different data sources (e.g., different GPU racks or nodes).
- **Offset-Based Consumption**: Subscribers can specify where to start reading: `OffsetEarliest` (beginning), `OffsetLatest` (new messages only), or a specific offset. Each collector tracks its own position.
- **Fan-Out Pattern**: Every collector receives ALL messages (not load-balanced). This allows multiple independent consumers to process the same data stream.
- **InfluxDB Time-Series Storage**: Telemetry is persisted to InfluxDB for efficient time-range queries, retention policies, and high write throughput.
- **Collect-Then-Batch Pattern**: Each streamer collects metrics locally for an interval before publishing, reducing MQ traffic and improving efficiency.

---

## Components

### 1. Message Queue Server (`cmd/mq-server`)

A custom, log-based message queue supporting:
- **Append-only log**: Messages stored in a dynamic slice that grows as needed
- **Offset-based subscription**: Consumers specify starting offset (`OffsetEarliest`, `OffsetLatest`, or specific offset)
- **Fan-out delivery**: All subscribers receive all messages (no load balancing)
- **TCP protocol**: Length-prefixed JSON messages for reliable communication
- **HTTP endpoints**: Health checks and statistics at port 9001

### 2. Telemetry Streamer (`cmd/streamer`)

Reads DCGM metrics from CSV files and streams them to the message queue:
- **Multiple instances**: Deploy multiple streamers to read from different data sources (GPU racks, nodes)
- **Collect-then-batch**: Collects metrics locally for configurable interval (default 5s), then publishes as batch
- **Two goroutines**: Separate collection and publishing loops for decoupled processing
- **Automatic reconnection**: Reconnects to MQ on connection loss
- **Graceful shutdown**: Properly drains buffer before shutdown
- **Unique instance ID**: Each streamer has a unique ID for identification in logs and metrics

### 3. Telemetry Collector (`cmd/collector`)

Subscribes to MQ and persists telemetry data to InfluxDB:
- **Offset-based consumption**: Starts from earliest, latest, or specific offset
- **InfluxDB persistence**: Writes to InfluxDB time-series database
- **Independent consumers**: Multiple collectors can run simultaneously, each reading all messages
- **Configurable retention**: Data cleanup based on retention policies

### 4. API Gateway (`cmd/api`)

REST API for querying telemetry data with auto-generated Swagger documentation:
- List all GPUs with pagination
- Get GPU details by ID
- Query telemetry with time filters
- Supports both in-memory storage (for development) and InfluxDB (for production)
- Health and statistics endpoints

---

## Prerequisites

- **Go 1.22+** - [Download Go](https://go.dev/dl/)
- **InfluxDB 2.x** - [Install InfluxDB](https://docs.influxdata.com/influxdb/v2/install/) (for production)
- **Docker** (optional) - For containerized deployment
- **Helm 3+** (optional) - For Kubernetes deployment
- **Make** - For running build commands

### Install Development Tools

```bash
# Install Swagger documentation generator
go install github.com/swaggo/swag/cmd/swag@latest

# Install linter (optional)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

---

## Build Instructions

### Using Make

```bash
# Clone and navigate to project
cd gpu-telemetry-pipeline

# Install dependencies
go mod tidy

# Build all binaries
make build

# Binaries are created in ./bin directory:
# - bin/mq-server
# - bin/api
# - bin/streamer
# - bin/collector
```

### Build Individual Components

```bash
# Build only the API server
make build-api

# Build only the MQ server
make build-mq-server

# Build only the streamer
make build-streamer

# Build only the collector
make build-collector
```

### Generate Swagger Documentation

```bash
# Generate OpenAPI/Swagger spec
make swagger

# Documentation is generated in ./docs directory
```

---

## Installation

### Local Development

#### Step 1: Start the Message Queue Server

```bash
# Using Make
make run-mq

# Or directly
./bin/mq-server --tcp-addr :9000 --http-addr :9001
```

#### Step 2: Start InfluxDB (for production)

```bash
# Using Docker
docker run -d --name influxdb -p 8086:8086 influxdb:2.7

# Create bucket and get token from InfluxDB UI at http://localhost:8086
# Or use CLI:
influx setup --username admin --password adminpassword --org cisco --bucket gpu_telemetry --force
```

#### Step 3: Start the Collector

```bash
# Set environment variables for InfluxDB
export MQ_HOST=localhost
export MQ_PORT=9000
export INFLUXDB_URL=http://localhost:8086
export INFLUXDB_TOKEN=your-token-here
export INFLUXDB_ORG=cisco
export INFLUXDB_BUCKET=gpu_telemetry

# Run collector
./bin/collector
```

#### Step 4: Start the Streamer

```bash
# Set environment variables
export MQ_HOST=localhost
export MQ_PORT=9000
export CSV_PATH=/path/to/your/dcgm_metrics.csv

# Run streamer
./bin/streamer
```

#### Step 5: Start the API Gateway

```bash
# For development (in-memory storage)
./bin/api --storage-type memory

# For production (InfluxDB storage)
export INFLUXDB_URL=http://localhost:8086
export INFLUXDB_TOKEN=your-token-here
export INFLUXDB_ORG=cisco
export INFLUXDB_BUCKET=gpu_telemetry
./bin/api --storage-type influxdb

# Access Swagger UI at http://localhost:8080/swagger/index.html
```

### Docker Deployment

#### Build Docker Images

```bash
# Build all images
make docker-build

# Or build individually
make docker-build-api
make docker-build-mq-server
make docker-build-streamer
make docker-build-collector
```

#### Run with Docker Compose

Create a `docker-compose.yml`:

```yaml
version: '3.8'

services:
  influxdb:
    image: influxdb:2.7
    ports:
      - "8086:8086"
    environment:
      - DOCKER_INFLUXDB_INIT_MODE=setup
      - DOCKER_INFLUXDB_INIT_USERNAME=admin
      - DOCKER_INFLUXDB_INIT_PASSWORD=adminpassword
      - DOCKER_INFLUXDB_INIT_ORG=cisco
      - DOCKER_INFLUXDB_INIT_BUCKET=gpu_telemetry
      - DOCKER_INFLUXDB_INIT_ADMIN_TOKEN=my-super-secret-token
    volumes:
      - influxdb-data:/var/lib/influxdb2

  mq-server:
    image: gpu-telemetry-pipeline/mq-server:1.0.0
    ports:
      - "9000:9000"
      - "9001:9001"
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9001/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  collector:
    image: gpu-telemetry-pipeline/collector:1.0.0
    environment:
      - MQ_HOST=mq-server
      - MQ_PORT=9000
      - INFLUXDB_URL=http://influxdb:8086
      - INFLUXDB_TOKEN=my-super-secret-token
      - INFLUXDB_ORG=cisco
      - INFLUXDB_BUCKET=gpu_telemetry
    depends_on:
      mq-server:
        condition: service_healthy
      influxdb:
        condition: service_started

  streamer:
    image: gpu-telemetry-pipeline/streamer:1.0.0
    environment:
      - MQ_HOST=mq-server
      - MQ_PORT=9000
      - CSV_PATH=/data/metrics.csv
    volumes:
      - ./data:/data:ro
    depends_on:
      collector:
        condition: service_started

  api:
    image: gpu-telemetry-pipeline/api:1.0.0
    ports:
      - "8080:8080"
    environment:
      - GIN_MODE=release
      - STORAGE_TYPE=influxdb
      - INFLUXDB_URL=http://influxdb:8086
      - INFLUXDB_TOKEN=my-super-secret-token
      - INFLUXDB_ORG=cisco
      - INFLUXDB_BUCKET=gpu_telemetry
    depends_on:
      collector:
        condition: service_started

volumes:
  influxdb-data:

networks:
  default:
    driver: bridge
```

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f

# Stop services
docker-compose down
```

### Kubernetes Deployment

#### Using Kind (Local Development)

```bash
# 1. Create Kind cluster with port mappings
kind create cluster --config deployments/kind/kind-config.yaml

# 2. Build all Docker images
docker build -t gpu-telemetry-pipeline/mq-server:latest -f deployments/docker/mq-server.Dockerfile .
docker build -t gpu-telemetry-pipeline/streamer:latest -f deployments/docker/streamer.Dockerfile .
docker build -t gpu-telemetry-pipeline/collector:latest -f deployments/docker/collector.Dockerfile .
docker build -t gpu-telemetry-pipeline/api:latest -f deployments/docker/api.Dockerfile .

# 3. Load images into Kind cluster
kind load docker-image gpu-telemetry-pipeline/mq-server:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-pipeline/streamer:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-pipeline/collector:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-pipeline/api:latest --name gpu-telemetry

# 4. Create namespace and deploy InfluxDB
kubectl create namespace gpu-telemetry
kubectl apply -f deployments/kind/influxdb.yaml -n gpu-telemetry

# 5. Create ConfigMap with telemetry data (first 1000 lines to fit 1MB limit)
head -1000 /path/to/your/telemetry.csv > telemetry_sample.csv
kubectl create configmap gpu-telemetry-gpu-telemetry-pipeline-data \
  --from-file=telemetry.csv=telemetry_sample.csv -n gpu-telemetry

# 6. Deploy with Helm (set InfluxDB token)
helm install gpu-telemetry deployments/helm/gpu-telemetry-pipeline \
  -n gpu-telemetry \
  --set influxdb.token=my-super-secret-token
```

#### Access Services (Kind)

| Service | URL | Description |
|---------|-----|-------------|
| API Swagger UI | http://localhost:8080/swagger/index.html | Interactive API docs |
| API GPUs | http://localhost:8080/api/v1/gpus | List all GPUs |
| InfluxDB UI | http://localhost:8086 | InfluxDB dashboard (admin/adminpassword) |

#### Using Helm (Generic Kubernetes)

```bash
# Install the chart
helm install gpu-telemetry deployments/helm/gpu-telemetry-pipeline \
  --namespace gpu-telemetry \
  --create-namespace \
  --set influxdb.token=your-influxdb-token \
  --set influxdb.url=http://your-influxdb:8086

# With custom values
helm install gpu-telemetry deployments/helm/gpu-telemetry-pipeline \
  --namespace gpu-telemetry \
  --create-namespace \
  --set streamer.data.existingConfigMap=your-csv-configmap \
  --set api.replicaCount=3

# Upgrade existing release
helm upgrade gpu-telemetry deployments/helm/gpu-telemetry-pipeline \
  --namespace gpu-telemetry

# Uninstall
helm uninstall gpu-telemetry --namespace gpu-telemetry
```

#### Manage Deployments

```bash
# Check pods
kubectl get pods -n gpu-telemetry

# Check services
kubectl get svc -n gpu-telemetry

# View logs
kubectl logs -l app.kubernetes.io/component=api -n gpu-telemetry -f
kubectl logs -l app.kubernetes.io/component=collector -n gpu-telemetry -f
kubectl logs -l app.kubernetes.io/component=streamer -n gpu-telemetry -f

# Stop all deployments (scale to 0)
kubectl scale deployment --all --replicas=0 -n gpu-telemetry

# Start all deployments
kubectl scale deployment --all --replicas=1 -n gpu-telemetry

# Delete Kind cluster
kind delete cluster --name gpu-telemetry
```

---

## Configuration

### Environment Variables

#### Message Queue Server
| Variable | Default | Description |
|----------|---------|-------------|
| `TCP_ADDR` | `:9000` | TCP server address |
| `HTTP_ADDR` | `:9001` | HTTP server address |
| `BUFFER_SIZE` | `10000` | Initial message buffer size |

#### Streamer
| Variable | Default | Description |
|----------|---------|-------------|
| `MQ_HOST` | `localhost` | MQ server host |
| `MQ_PORT` | `9000` | MQ server port |
| `CSV_PATH` | (required) | Path to CSV file |
| `BATCH_SIZE` | `100` | Max messages per batch |
| `COLLECT_INTERVAL` | `5s` | How long to collect before publishing |
| `INSTANCE_ID` | `` | Unique instance identifier |

#### Collector
| Variable | Default | Description |
|----------|---------|-------------|
| `MQ_HOST` | `localhost` | MQ server host |
| `MQ_PORT` | `9000` | MQ server port |
| `COLLECTOR_ID` | `collector-1` | Unique collector identifier |
| `INFLUXDB_URL` | `http://localhost:8086` | InfluxDB server URL |
| `INFLUXDB_TOKEN` | (required) | InfluxDB authentication token |
| `INFLUXDB_ORG` | `cisco` | InfluxDB organization |
| `INFLUXDB_BUCKET` | `gpu_telemetry` | InfluxDB bucket name |
| `RETENTION_PERIOD` | `24h` | Data retention period |
| `FLUSH_INTERVAL` | `10s` | How often to flush to storage |

#### API Gateway
| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |
| `GIN_MODE` | `debug` | Gin mode (debug/release) |
| `STORAGE_TYPE` | `memory` | Storage backend (memory/influxdb) |
| `INFLUXDB_URL` | `http://localhost:8086` | InfluxDB server URL (if using influxdb) |
| `INFLUXDB_TOKEN` | | InfluxDB authentication token |
| `INFLUXDB_ORG` | `cisco` | InfluxDB organization |
| `INFLUXDB_BUCKET` | `gpu_telemetry` | InfluxDB bucket name |
| `MAX_PAGE_SIZE` | `1000` | Max items per page |

---

## API Reference

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/health` | Health check (for Kubernetes probes) |
| `GET` | `/ready` | Readiness check (for Kubernetes probes) |
| `GET` | `/api/v1/gpus` | List all GPU IDs |
| `GET` | `/api/v1/gpus/{id}/telemetry` | Get telemetry for a specific GPU |
| `GET` | `/swagger/*` | Swagger UI |

### Example Requests

```bash
# Health check
curl http://localhost:8080/health
# Response: {"status":"healthy"}

# List all GPUs
curl http://localhost:8080/api/v1/gpus
# Response: {"data":["GPU-xxx-...", "GPU-yyy-..."], "count":12}

# Get telemetry for a GPU
curl "http://localhost:8080/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?limit=10"
```

### Query Parameters for Telemetry

| Parameter | Type | Description |
|-----------|------|-------------|
| `start_time` | RFC3339 | Start time filter (e.g., 2024-01-01T00:00:00Z) |
| `end_time` | RFC3339 | End time filter |
| `limit` | int | Maximum results (default: 100) |
| `offset` | int | Offset for pagination |

### Example Requests

```bash
# Health check
curl http://localhost:8080/health

# List all GPUs
curl http://localhost:8080/api/v1/gpus

# List GPUs with pagination
curl "http://localhost:8080/api/v1/gpus?page=1&page_size=50"

# Get specific GPU
curl http://localhost:8080/api/v1/gpus/GPU-abc123

# Get GPU telemetry with time filter
curl "http://localhost:8080/api/v1/gpus/GPU-abc123/telemetry?start_time=2025-07-18T00:00:00Z&end_time=2025-07-18T23:59:59Z"

# Get telemetry filtered by metric name
curl "http://localhost:8080/api/v1/telemetry?metric_name=DCGM_FI_DEV_GPU_UTIL"

# Get pipeline statistics
curl http://localhost:8080/api/v1/stats
```

---

## Sample User Workflows

### Workflow 1: Setting Up Local Development Environment

```bash
# 1. Clone the repository
git clone <repository-url>
cd gpu-telemetry-pipeline

# 2. Install dependencies
go mod tidy

# 3. Build all components
make build

# 4. Start MQ server in terminal 1
./bin/mq-server

# 5. Start collector in terminal 2
export MQ_HOST=localhost
export MQ_PORT=9000
./bin/collector

# 6. Start API in terminal 3
./bin/api

# 7. Stream data in terminal 4
export CSV_PATH=/path/to/metrics.csv
./bin/streamer

# 8. Query data
curl http://localhost:8080/api/v1/gpus
```

### Workflow 2: Deploying to Kubernetes

```bash
# 1. Build and push Docker images
export DOCKER_REGISTRY=myregistry.io/
make docker-build
make docker-push

# 2. Create namespace
kubectl create namespace gpu-telemetry

# 3. Create ConfigMap for CSV data (if needed)
kubectl create configmap gpu-metrics-data \
  --from-file=metrics.csv=./data/dcgm_metrics.csv \
  -n gpu-telemetry

# 4. Deploy with Helm
helm install gpu-telemetry deployments/helm/gpu-telemetry-pipeline \
  --namespace gpu-telemetry \
  --set global.imageRegistry=myregistry.io/

# 5. Verify deployment
kubectl get pods -n gpu-telemetry
kubectl get svc -n gpu-telemetry

# 6. Access the API
kubectl port-forward svc/gpu-telemetry-api 8080:8080 -n gpu-telemetry

# 7. Open Swagger UI
# http://localhost:8080/swagger/index.html
```

### Workflow 3: Monitoring GPU Performance

```bash
# 1. Get list of all GPUs in the cluster
curl http://localhost:8080/api/v1/gpus | jq '.gpus[] | {id: .id, model: .model_name, host: .hostname}'

# 2. Check GPU utilization for a specific GPU over the last hour
START_TIME=$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)
END_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)

curl "http://localhost:8080/api/v1/gpus/GPU-abc123/telemetry?\
start_time=${START_TIME}&end_time=${END_TIME}&\
metric_name=DCGM_FI_DEV_GPU_UTIL" | jq

# 3. Check memory usage across all GPUs
curl "http://localhost:8080/api/v1/telemetry?\
metric_name=DCGM_FI_DEV_FB_USED&page_size=500" | jq

# 4. Get pipeline statistics
curl http://localhost:8080/api/v1/stats | jq
```

---

## Testing

### Run Unit Tests

```bash
# Run all tests
make test

# Run with verbose output
go test -v ./...

# Run specific package tests
go test -v ./internal/mq/...
go test -v ./internal/storage/...
go test -v ./internal/parser/...
go test -v ./internal/api/...
```

### Code Coverage

```bash
# Generate coverage report
make coverage

# View coverage summary
make coverage-summary

# Open coverage HTML report
# (Generated at coverage/coverage.html)
```

### Code Quality

```bash
# Format code
make fmt

# Run go vet
make vet

# Run linter
make lint

# Run all checks
make all
```

---

## AI Usage Documentation

This section documents how AI assistance was used in the development of this project, as required by the project specification.

### AI Tool Used

- **Tool**: GitHub Copilot (Claude-based assistant)
- **Interface**: VS Code Chat/Agent Mode
- **Date**: July 2025

### Development Approach

The entire codebase was developed with AI assistance in an iterative, conversational manner. The developer provided detailed specifications and requirements, and the AI generated code that was reviewed and integrated into the project.

### Specific AI Contributions

#### 1. Architecture Design
- **Prompt Summary**: "Design and implement an elastic, scalable and stable telemetry pipeline for an AI Cluster with messaging queue"
- **AI Contribution**: Designed the overall architecture including:
  - Custom log-based MQ with offset tracking for replay capability
  - Fan-out subscription pattern (all collectors read all messages)
  - Collect-then-batch pattern for efficient streaming
  - InfluxDB integration for time-series storage
- **Human Review**: Approved architecture; requested specific constraints (no external MQ dependencies); iteratively simplified design by removing topics, partitions, and worker pools

#### 2. Core Message Queue Implementation
- **Prompt Summary**: "Build MQ in Go without using ZeroMQ, RabbitMQ, Kafka or any other existing MQ"
- **AI Contribution**: Generated complete MQ implementation including:
  - `internal/mq/queue.go`: Log-based queue with dynamic slice, offset support (OffsetEarliest, OffsetLatest, specific)
  - `internal/mq/client.go`: TCP client with reconnection logic and offset-based subscription
  - `internal/mq/server.go`: TCP server with protocol handling
- **Human Review**: Verified offset tracking, tested fan-out delivery; simplified from partitioned topics to single log

#### 3. Data Models and Configuration
- **Prompt Summary**: Analyzed CSV file structure and generated appropriate models
- **AI Contribution**: 
  - `pkg/models/telemetry.go`: GPUMetric, GPUInfo, MetricBatch structs
  - `pkg/config/config.go`: Configuration structures with environment variable loading, InfluxDB settings
- **Human Review**: Confirmed field mappings match CSV schema

#### 4. CSV Parser
- **Prompt Summary**: "Parse DCGM metrics from CSV with timestamp, metric_name, gpu_id, device, uuid, etc."
- **AI Contribution**: 
  - `internal/parser/csv.go`: Complete CSV parser with validation and batch reading
- **Human Review**: Tested with actual DCGM export files

#### 5. Storage Layer
- **Prompt Summary**: "Implement InfluxDB storage for time-series telemetry data"
- **AI Contribution**:
  - `internal/storage/influxdb.go`: InfluxDB storage with Flux queries for time-range filtering
  - `internal/storage/memory.go`: In-memory storage for development/testing
- **Human Review**: Verified concurrent access safety, tested Flux queries

#### 6. REST API with Swagger
- **Prompt Summary**: "Create REST API endpoints with auto-generated OpenAPI spec using swaggo"
- **AI Contribution**:
  - `internal/api/handlers/handlers.go`: HTTP handlers with Swagger annotations
  - `internal/api/router.go`: Gin router setup with storage backend selection
  - `docs/docs.go`: Swagger documentation template
- **Human Review**: Tested all endpoints, verified Swagger UI functionality

#### 7. Service Entry Points
- **Prompt Summary**: "Create main.go files for each service with proper initialization"
- **AI Contribution**:
  - `cmd/mq-server/main.go`: MQ server with TCP and HTTP listeners
  - `cmd/streamer/main.go`: Two-goroutine design (collect loop + publish loop)
  - `cmd/collector/main.go`: Offset-based subscription with InfluxDB persistence
  - `cmd/api/main.go`: REST API with configurable storage backend
- **Human Review**: Verified signal handling and graceful shutdown

#### 8. Containerization
- **Prompt Summary**: "Create Dockerfiles for all services"
- **AI Contribution**:
  - Multi-stage Dockerfiles for all 4 services
  - Optimized for small image size using Alpine
  - Non-root user configuration
- **Human Review**: Built and tested all images

#### 9. Kubernetes Deployment
- **Prompt Summary**: "Create Helm charts for Kubernetes deployment"
- **AI Contribution**:
  - Complete Helm chart structure
  - Configurable values.yaml
  - Service and deployment templates
- **Human Review**: Deployed to test cluster, verified pod communication

#### 10. Unit Tests
- **Prompt Summary**: "Write comprehensive unit tests with good coverage"
- **AI Contribution**:
  - `internal/mq/queue_test.go`: Queue operation tests
  - `internal/mq/worker_test.go`: Worker pool tests
  - `internal/storage/memory_test.go`: Storage tests
  - `internal/parser/csv_test.go`: Parser tests
  - `internal/api/handlers/handlers_test.go`: API handler tests
- **Human Review**: Ran tests, verified coverage metrics

#### 11. Build System
- **Prompt Summary**: "Create Makefile with build, test, coverage, swagger, docker targets"
- **AI Contribution**:
  - Comprehensive Makefile with all required targets
- **Human Review**: Tested all make targets

### Code Review Process

All AI-generated code underwent the following review process:

1. **Syntax Verification**: Ensured code compiles without errors
2. **Logic Review**: Verified business logic matches requirements
3. **Security Check**: Reviewed for common vulnerabilities
4. **Performance Review**: Checked for obvious performance issues
5. **Test Execution**: Ran unit tests to verify functionality
6. **Integration Testing**: Tested components working together

### Modifications Made to AI Output

- Added additional error handling in edge cases
- Adjusted timeout values based on testing
- Fixed minor formatting inconsistencies
- Enhanced logging messages for better debugging

### AI Limitations Observed

1. **Context Window**: Required breaking large implementations into smaller chunks
2. **Testing Edge Cases**: Some obscure edge cases needed manual test additions
3. **Environment-Specific Configs**: Required manual adjustment for specific deployment environments

### Conclusion

AI assistance significantly accelerated the development process while maintaining code quality. The combination of AI code generation with human review and testing proved effective for building this production-ready telemetry pipeline.

---

## License

This project is provided for evaluation purposes.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Submit a pull request

---

## Support

For issues or questions, please open a GitHub issue.
