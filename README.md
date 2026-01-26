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


---

## Testing

```bash
# Run all tests
make test

# Run tests with coverage report
make coverage
```

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

### 5. CSV Data File

The pipeline reads GPU telemetry from `dcgm_metrics_20250718_134233.csv`. When using KIND, this file is automatically copied to the cluster node at `/data/dcgm_metrics.csv`.

---

## AI Usage Documentation

This section documents how AI assistance was used in the development of this project, as required by the project specification.

### AI Tool Used

- **Tool**: GitHub Copilot (Claude-based assistant)
- **Interface**: VS Code Chat/Agent Mode

### Specific AI Contributions

**Prompts used:** "Plan the project and then first implement message queue - an in-memory slice that is append-only and can support writes from various clients. Streamers which can read CSV and push into the queue, and collector to read and push into InfluxDB. API registry with auto-generated API spec with GET API calls as in the doc."

**Issues encountered:** AI initially used topics and worker pools which were not needed for this use case. It also didn't implement support for multiple streamers and was not collecting metrics data over a period of time (batching).

### AI Limitations Observed

1. **Context Window**: Required breaking large implementations into smaller chunks.
2. **Over-engineering**: Most of the time it complicates simpler tasks by adding too many abstractions and handling too many edge cases.
3. **Environment-Specific Configs**: Required manual adjustment for specific deployment environments.



