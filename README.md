# Telemetry Pipeline 

An in-memory message queue system where streamers periodically publish telemetry data and collectors consume and persist it to InfluxDB. Includes a REST API for querying telemetry data.

## Table of Contents

- [Quick Start](#quick-start)
- [Technologies](#technologies)
- [Architecture Overview](#architecture-overview)
- [Testing](#testing)
- [Components](#components)
- [AI Usage Documentation](#ai-usage-documentation)

---

## Quick Start

### Prerequisites

- **Go 1.22+** - [Download Go](https://go.dev/dl/)
- **Docker Desktop** - For containerized deployment
- **Make** - For build commands (`winget install ezwinports.make` on Windows)
- **KIND** - For local Kubernetes cluster
- **kubectl** - Kubernetes CLI


### Deploy to KIND 

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

### Deploy with Helm

**Using Helm charts:**
```bash
# Install with Helm
helm install telemetry-pipeline ./helm/telemetry-pipeline

# Or with custom values
helm install telemetry-pipeline ./helm/telemetry-pipeline -f custom-values.yaml

# Upgrade existing deployment
helm upgrade telemetry-pipeline ./helm/telemetry-pipeline

# Uninstall
helm uninstall telemetry-pipeline
```

The Helm charts include:
- Complete InfluxDB deployment with PVC
- All components with proper configuration
- Resource limits and health checks
- Configurable via values.yaml

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
| API Swagger UI | http://localhost:30080/swagger/ | Interactive API docs & testing |
| API Health | http://localhost:30080/health | Health check |
| API Stats | http://localhost:30080/api/v1/stats | System statistics |
| List GPUs | http://localhost:30080/api/v1/gpus | Get all GPU UUIDs |
| InfluxDB UI | http://localhost:30086 | InfluxDB dashboard |

**InfluxDB Credentials:**
- Username: `admin`
- Password: `admin123`
- Token: `my-super-secret-token`

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

---

## Technologies

Golang, InfluxDB, Docker, Kubernetes.

---

## Architecture Overview

```
                                  GPU Telemetry Pipeline Architecture
                                  ===================================

┌───────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                      KIND CLUSTER (gpu-telemetry)                                     │
│                                                                                                       │
│  ┌─────────────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │                                Shared CSV File (hostPath: /data)                                │  │
│  │                                       dcgm_metrics.csv                                          │  │
│  └──────────────────────────────────────────────┬──────────────────────────────────────────────────┘  │
│                                                 │                                                     │
│                      ┌──────────────────────────┼──────────────────────────┐                          │
│                      │                          │                          │                          │
│                      ▼                          ▼                          ▼                          │
│  ┌─────────────────────────────────────────────────────────────────────────────────────────────────┐  │
│  │                                   Telemetry Streamers (Pods)                                    │  │
│  │     ┌─────────────┐          ┌─────────────┐          ┌─────────────┐                           │  │
│  │     │ Streamer-1  │          │ Streamer-2  │          │ Streamer-N  │                           │  │
│  │     │   (Pod)     │          │   (Pod)     │          │   (Pod)     │                           │  │
│  │     │  reads CSV  │          │  reads CSV  │          │  reads CSV  │                           │  │
│  │     └──────┬──────┘          └──────┬──────┘          └──────┬──────┘                           │  │
│  │            │                        │                        │                                  │  │
│  │            │     All streamers publish to the same MQ        │                                  │  │
│  └────────────┼────────────────────────┼────────────────────────┼──────────────────────────────────┘  │
│               │                        │                        │                                     │
│               └────────────────────────┼────────────────────────┘                                     │
│                                        ▼                                                              │
│  ┌─────────────────────────────────────────────────────────────────┐    ┌───────────────────────┐     │
│  │                Custom Log-Based Message Queue (Pod)             │    │                       │     │
│  │                                                                 │    │     REST API          │     │
│  │  ┌───────────────────────────────────────────────────────────┐  │    │     Gateway           │     │
│  │  │                 Message Log (Dynamic Slice)               │  │    │      (Pod)            │     │
│  │  │    [msg0][msg1][msg2][msg3][msg4][msg5][msg6]...          │  │    │                       │     │
│  │  │      ↑                              ↑                     │  │    │    Swagger UI         │     │
│  │  │      │                              │                     │  │    │    /swagger/*         │     │
│  │  │    offset=0                    offset=6 (latest)          │  │    │                       │     │
│  │  └───────────────────────────────────────────────────────────┘  │    │    /api/v1/*          │     │
│  │                                                                 │    └───────────┬───────────┘     │
│  │  ┌──────────────────────┐      ┌──────────────────────┐         │                │                 │
│  │  │     TCP Server       │      │     HTTP Server      │         │                │                 │
│  │  │     Port: 9000       │      │     Port: 9001       │         │                │                 │
│  │  │     - Publish        │      │     - /health        │         │                │                 │
│  │  │     - Subscribe      │      │     - /stats         │         │                │                 │
│  │  │     (with offset)    │      │                      │         │                │                 │
│  │  └──────────────────────┘      └──────────────────────┘         │                │                 │
│  │                                                                 │                │                 │
│  └─────────────────────────────────────────────────────────────────┘                │                 │
│                                  │                                                  │                 │
│                                  │ Subscribe (offset-based)                         │                 │
│                                  ▼                                                  ▼                 │
│  ┌─────────────────────────────────────────────────────────────────┐    ┌───────────────────────────┐ │
│  │                      Telemetry Collectors (Pods)                │    │                           │ │
│  │     ┌─────────────┐  ┌─────────────┐  ┌─────────────┐           │    │      InfluxDB (Pod)       │ │
│  │     │ Collector-1 │  │ Collector-2 │  │ Collector-N │           │    │    (Time-Series DB)       │ │
│  │     │  offset=0   │  │  offset=0   │  │  offset=3   │           │───▶│                           │ │
│  │     │ (earliest)  │  │ (earliest)  │  │ (specific)  │           │    │    - GPU Metrics Store    │ │
│  │     └─────────────┘  └─────────────┘  └─────────────┘           │    │    - Time-based Queries   │ │
│  │                                                                 │    │    - Data Retention       │ │
│  │     Each collector reads ALL messages independently             │    │                           │ │
│  └─────────────────────────────────────────────────────────────────┘    └───────────────────────────┘ │
│                                                                                                       │
│  NodePorts: API (30080), InfluxDB (30086)                                                             │
└───────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **CSV File** is stored on the KIND cluster node at `/data/dcgm_metrics.csv` (mounted as hostPath volume)
2. **Telemetry Streamers** (1 or more pods) all read GPU metrics from the same shared CSV file.
3. Each streamer collects metrics locally in a buffer for a configurable interval (default: 5s)
4. Batched metrics are published to the **Custom Message Queue** over TCP (all streamers publish to the same MQ)
5. The MQ appends messages to a log-based structure (dynamic slice) - messages from all streamers are interleaved. Chronological order of messages is maintained
6. **Telemetry Collectors** (1 or more pods) subscribe with offset support - each collector reads ALL messages independently
7. Collectors persist metrics to **InfluxDB** (time-series database) for efficient time-based queries
8. **API Gateway** provides REST endpoints for querying stored telemetry data from InfluxDB

### Key Design Decisions

- **InfluxDB Storage**: Telemetry is persisted to InfluxDB which is a timeseries DB  which does efficient time-range queries, retention policies, and high write throughput.
- **Custom Log-Based MQ**: Built from scratch using a dynamic slice (append-only log). No external MQ dependencies. Uses TCP protocol with length-prefixed JSON messages as REST (HTTP) calls would be expensive. This is append-only with mutex locking - each append takes ~50-200 nanoseconds. Even with 10 streamers arriving simultaneously, the last request waits only ~1-2 microseconds (lock contention + 10 sequential appends).
- **Offset-Based Consumption**: Subscribers can specify where to start reading: `OffsetEarliest` (beginning), `OffsetLatest` (new messages only), or a specific offset. Each collector tracks its own position.
- **Fan-Out Pattern**: Every collector receives ALL messages (not load-balanced). This allows multiple independent consumers to process the same data stream.

### Why TCP over gRPC?
- **Simplicity:** TCP sockets are easier to debug and require less setup than gRPC for a custom log-based queue.
- **Performance:** For simple message passing, TCP with length-prefixed JSON is fast and has minimal overhead.
- **Portability:** No need for .proto files or code generation; easier for new contributors.
- **Extensibility:** If future requirements demand richer contracts, gRPC can be added later without major refactor.

### Why Helm?
- **Abstraction:** Helm simplifies Kubernetes deployments, templating, and configuration management.
- **Portability:** Users can deploy the stack with a single command and override values as needed.
- **Best Practice:** Helm is the de facto standard for packaging and distributing Kubernetes applications.

### Why Port Forwarding?
- **Developer Experience:** Port-forwarding allows local access to cluster services (API, InfluxDB) without exposing them externally.
- **Security:** Reduces attack surface by not exposing NodePorts in production.

### Docker
All services are containerized using Docker for consistent builds and deployments. Dockerfiles were generated with AI assistance and manually refined for multi-stage builds and best practices.

### Makefile Enhancements
- Targets for build, test, coverage, integration, Helm, and Kubernetes operations.
- `make openapi-gen` generates the OpenAPI spec.
- `make port-forward-api` and `make port-forward-influxdb` for local access.

### Developer Experience
- System integration tests are provided in both Bash and PowerShell for cross-platform validation.
- The Makefile is enhanced for a smooth developer workflow.

---

## Testing

```bash
# Run all unit tests
make test

# Run tests with coverage report
make coverage

# Run integration tests (requires deployed system)
make integration-test

# Deploy to KIND and run integration tests
make integration-test-kind
```

### Integration Tests

Integration tests verify the complete pipeline end-to-end:
- API health and readiness checks
- GPU listing and querying
- Telemetry retrieval with filters
- Metric name listing
- System statistics
- Error handling

Tests are available in both bash (`tests/integration_test.sh`) and PowerShell (`tests/integration_test.ps1`) formats.

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

**Available Endpoints:**
- `GET /api/v1/gpus` - List all GPUs with pagination
- `GET /api/v1/gpus/{id}` - Get GPU details by ID (model, hostname, first/last seen)
- `GET /api/v1/gpus/{id}/metrics` - List available metric names for a specific GPU
- `GET /api/v1/gpus/{id}/telemetry` - Query telemetry data with filters (time range, metric name, pagination)
- `GET /api/v1/gpus/{id}/telemetry/export` - Export telemetry data in JSON or CSV format
- `GET /api/v1/metrics` - List all available metric types across the system
- `GET /api/v1/stats` - Get system statistics (total GPUs, metric counts)
- `GET /health` - Health check endpoint
- `GET /ready` - Readiness check endpoint
- `GET /swagger/` - Interactive Swagger UI documentation

**Features:**
- Supports both in-memory storage (for development) and InfluxDB (for production)
- Export telemetry data in JSON or CSV format for analysis
- Time-based filtering with RFC3339 timestamps
- Pagination support for large datasets
- Interactive API testing via Swagger UI

### 5. CSV Data File

The pipeline reads GPU telemetry from `dcgm_metrics_20250718_134233.csv`. When using KIND, this file is automatically copied to the cluster node at `/data/dcgm_metrics.csv`.

---

## AI Usage Documentation

This project extensively used AI assistance throughout development. For comprehensive documentation of all AI prompts, issues encountered, solutions, and lessons learned, see:

**[Complete AI Documentation](./docs/AI_DOCUMENTATION.md)**

### Quick Summary

**AI Tool Used:**
- **Tool**: GitHub Copilot (Claude-based assistant)
- **Interface**: VS Code Chat/Agent Mode

**Key AI Contributions:**
- Project architecture and initial code structure
- Message queue implementation
- CSV parser and InfluxDB integration
- REST API with Swagger documentation (including telemetry export)
- Docker and Kubernetes configurations
- Unit test frameworks

**Issues Encountered & Fixed:**
1. **Broken Telemetry Query**: AI used `tail()` instead of `skip()` for pagination offset - manually fixed
2. **Incomplete Helm Charts**: Missing InfluxDB, secrets, namespace, PVCs - completed manually
3. **Missing Features**: Additional REST endpoints (including telemetry export), integration tests - added manually

**Lessons Learned:**
- AI excels at code structure and boilerplate generation
- Human oversight essential for domain-specific knowledge (InfluxDB, Kubernetes)
- Always review and test AI-generated code thoroughly
- Production-ready configurations require manual refinement

See [AI_DOCUMENTATION.md](./docs/AI_DOCUMENTATION.md) for detailed prompts, issues, and solutions.



