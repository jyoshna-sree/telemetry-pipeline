# Telemetry Pipeline 

## Table of Contents

- [Technologies](#technologies)
- [Architecture Overview](#architecture-overview)
- [Components](#components)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [API Reference](#api-reference)
- [Testing](#testing)
- [AI Usage Documentation](#ai-usage-documentation)

---

## Technologies

Golang, InfluxDB, Docker, Kubernetes.

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
  │   │   CSV       │  │   CSV       │  │   CSV       │          │
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

1. **Telemetry Streamers** (1 or more) read GPU metrics from CSV files.
2. Each streamer collects metrics locally in a buffer for a configurable interval (default: 5s)
3. Batched metrics are published to the **Custom Message Queue** over TCP (all streamers publish to the same MQ)
4. The MQ appends messages to a log-based structure (dynamic slice) - messages from all streamers are interleaved. Chronological order of messages is maintained
5. **Telemetry Collectors** (1 or more) subscribe with offset support - each collector reads ALL messages independently
6. Collectors persist metrics to **InfluxDB** (time-series database) for efficient time-based queries
7. **API Gateway** provides REST endpoints for querying stored telemetry data from InfluxDB

### Key Design Decisions

- **Custom Log-Based MQ**: Built from scratch using a dynamic slice (append-only log). No external MQ dependencies. Uses TCP protocol with length-prefixed JSON messages as REST(HTTP) calls would be expensive.
- **Multiple Streamers**: Any number of streamers can publish to the same MQ simultaneously.
- **Collect-Then-Batch Pattern**: Each streamer collects metrics locally for an interval before publishing, reducing MQ traffic and improving efficiency.
- **Offset-Based Consumption**: Subscribers can specify where to start reading: `OffsetEarliest` (beginning), `OffsetLatest` (new messages only), or a specific offset. Each collector tracks its own position.
- **Fan-Out Pattern**: Every collector receives ALL messages (not load-balanced). This allows multiple independent consumers to process the same data stream.
- **InfluxDB Time-Series Storage**: Telemetry is persisted to InfluxDB for efficient time-range queries, retention policies, and high write throughput.

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

Reads metrics from CSV files and streams them to the message queue:
- **Multiple instances**: Deploy multiple streamers reading from the same CSV source for increased throughput
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
- **Docker Desktop** - For containerized deployment
- **Make** - For build commands (`winget install ezwinports.make` on Windows)
- **KIND** - For local Kubernetes cluster ([kind.sigs.k8s.io](https://kind.sigs.k8s.io/))
- **kubectl** - Kubernetes CLI

---

## Quick Start

### Prerequisites

- **Go 1.22+** - [Download Go](https://go.dev/dl/)
- **Docker Desktop** - For containerized deployment
- **Make** - For build commands (`winget install ezwinports.make` on Windows)
- **KIND** - For local Kubernetes cluster
- **kubectl** - Kubernetes CLI

### Make Commands

| Command | Description |
|---------|-------------|
| `make build` | Build Go binaries |
| `make docker-build` | Build Docker images |
| `make kind-setup` | Full setup: create KIND cluster, build images, load, copy CSV, deploy |
| `make kind-delete` | Delete KIND cluster |
| `make load-kind` | Load Docker images into existing KIND cluster |
| `make k8s-deploy` | Deploy to Kubernetes |
| `make k8s-delete` | Delete from Kubernetes |
| `make k8s-status` | Show pod/service status |
| `make test` | Run tests |
| `make coverage` | Run tests with coverage |
| `make clean` | Remove build artifacts |

### Deploy to KIND (Recommended)

**One-command setup:**
```bash
make kind-setup
```

This will:
1. Build all Docker images
2. Create KIND cluster with port mappings
3. Load images into cluster
4. Copy CSV data file to cluster
5. Deploy all services

**Note:** On Windows, if `kind` is not in PATH, run these manually:
```powershell
# Add kind to PATH (one-time)
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";$env:USERPROFILE", "User")

# Or run kind directly
& "$env:USERPROFILE\kind.exe" create cluster --name gpu-telemetry --config deployments/kind/kind-config.yaml
```

### Access Services

**Service URLs:**
- **API:** http://localhost:30080
- **InfluxDB:** http://localhost:30086

| Service | URL | Description |
|---------|-----|-------------|
| API Swagger UI | http://localhost:30080/swagger/index.html | Interactive API docs |
| API Health | http://localhost:30080/health | Health check |
| InfluxDB UI | http://localhost:30086 | InfluxDB dashboard |

**InfluxDB Credentials:**
- Username: `admin`
- Password: `admin123`
- Token: `my-super-secret-token`


### CSV Data File

The pipeline reads GPU telemetry from `dcgm_metrics_20250718_134233.csv`. When using KIND, this file is automatically copied to the cluster node at `/data/dcgm_metrics.csv`.

## Testing

```bash
# Run all tests
make test

# Run tests with coverage report
make coverage
```

---

## AI Usage Documentation

This section documents how AI assistance was used in the development of this project, as required by the project specification.

### AI Tool Used

- **Tool**: GitHub Copilot (Claude-based assistant)
- **Interface**: VS Code Chat/Agent Mode

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



