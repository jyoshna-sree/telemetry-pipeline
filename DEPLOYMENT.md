# Deploying GPU Telemetry Pipeline with KIND

This guide walks you through deploying the GPU Telemetry Pipeline to a local Kubernetes cluster using KIND (Kubernetes IN Docker).

## Prerequisites

### 1. Install Docker Desktop

Docker is required to run containers.

**Windows:**
1. Download from: https://www.docker.com/products/docker-desktop/
2. Run the installer
3. Restart your computer
4. Open Docker Desktop and wait for it to start (whale icon in system tray turns green)

**Verify installation:**
```powershell
docker --version
# Should show: Docker version 24.x.x or higher
```

### 2. Install KIND

KIND creates Kubernetes clusters using Docker containers.

**Windows (PowerShell as Administrator):**
```powershell
# Using Chocolatey (if you have it)
choco install kind

# OR download directly
curl -Lo kind-windows-amd64.exe https://kind.sigs.k8s.io/dl/v0.20.0/kind-windows-amd64
Move-Item kind-windows-amd64.exe C:\Windows\System32\kind.exe
```

**Verify installation:**
```powershell
kind --version
# Should show: kind version 0.20.0 or higher
```

### 3. Install kubectl

kubectl is the command-line tool to interact with Kubernetes.

**Windows (PowerShell as Administrator):**
```powershell
# Using Chocolatey
choco install kubernetes-cli

# OR download directly
curl -LO "https://dl.k8s.io/release/v1.28.0/bin/windows/amd64/kubectl.exe"
Move-Item kubectl.exe C:\Windows\System32\kubectl.exe
```

**Verify installation:**
```powershell
kubectl version --client
# Should show client version info
```

### 4. Install Helm

Helm is a package manager for Kubernetes (like npm for Node.js).

**Windows (PowerShell as Administrator):**
```powershell
# Using Chocolatey
choco install kubernetes-helm

# OR using Scoop
scoop install helm
```

**Verify installation:**
```powershell
helm version
# Should show version info
```

---

## Step-by-Step Deployment

### Step 1: Start Docker Desktop

1. Open Docker Desktop from Start Menu
2. Wait until it shows "Docker Desktop is running" (green icon)
3. Verify: `docker ps` should work without errors

### Step 2: Create KIND Cluster

```powershell
# Navigate to project directory
cd C:\Users\byreddy\Desktop\ciscoproject\gpu-telemetry-pipeline

# Create a KIND cluster with the provided config
kind create cluster --name gpu-telemetry --config deployments/kind/kind-config.yaml

# This takes 1-2 minutes. You'll see:
# Creating cluster "gpu-telemetry" ...
#  ✓ Ensuring node image (kindest/node:v1.27.3) 🖼
#  ✓ Preparing nodes 📦
#  ✓ Writing configuration 📜
#  ✓ Starting control-plane 🕹️
#  ✓ Installing CNI 🔌
#  ✓ Installing StorageClass 💾
# Set kubectl context to "kind-gpu-telemetry"
```

**Verify cluster is running:**
```powershell
kubectl cluster-info
# Should show cluster is running

kubectl get nodes
# Should show one node in "Ready" state
```

### Step 3: Build Docker Images

We need to build our application images and load them into KIND.

```powershell
# Build all Docker images
docker build -t gpu-telemetry-pipeline/mq-server:latest -f deployments/docker/mq-server.Dockerfile .
docker build -t gpu-telemetry-pipeline/streamer:latest -f deployments/docker/streamer.Dockerfile .
docker build -t gpu-telemetry-pipeline/collector:latest -f deployments/docker/collector.Dockerfile .
docker build -t gpu-telemetry-pipeline/api:latest -f deployments/docker/api.Dockerfile .

# Load images into KIND cluster (KIND can't pull from local Docker by default)
kind load docker-image gpu-telemetry-pipeline/mq-server:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-pipeline/streamer:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-pipeline/collector:latest --name gpu-telemetry
kind load docker-image gpu-telemetry-pipeline/api:latest --name gpu-telemetry
```

### Step 4: Deploy InfluxDB

We need a database for telemetry storage.

```powershell
# Create namespace for our application
kubectl create namespace gpu-telemetry

# Deploy InfluxDB
kubectl apply -f deployments/kind/influxdb.yaml -n gpu-telemetry

# Wait for InfluxDB to be ready (takes ~30 seconds)
kubectl wait --for=condition=ready pod -l app=influxdb -n gpu-telemetry --timeout=120s

# Verify InfluxDB is running
kubectl get pods -n gpu-telemetry
# Should show influxdb pod in "Running" state
```

### Step 5: Create Sample Data ConfigMap

```powershell
# Create ConfigMap with sample CSV data
kubectl create configmap gpu-metrics-data --from-file=telemetry.csv=C:\Users\byreddy\Desktop\ciscoproject\dcgm_metrics_20250718_134233.csv -n gpu-telemetry
```

### Step 6: Deploy the Application with Helm

```powershell
# Install the Helm chart
helm install gpu-telemetry deployments/helm/gpu-telemetry-pipeline `
  --namespace gpu-telemetry `
  --set influxdb.url="http://influxdb:8086" `
  --set influxdb.token="my-super-secret-token" `
  --set influxdb.org="cisco" `
  --set influxdb.bucket="gpu_telemetry" `
  --set streamer.data.existingConfigMap="gpu-metrics-data" `
  --set streamer.data.csvPath="/data/telemetry.csv"

# Wait for all pods to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=gpu-telemetry -n gpu-telemetry --timeout=180s
```

### Step 7: Verify Deployment

```powershell
# Check all pods are running
kubectl get pods -n gpu-telemetry

# Expected output:
# NAME                                    READY   STATUS    RESTARTS   AGE
# influxdb-xxx                           1/1     Running   0          2m
# gpu-telemetry-mq-server-xxx            1/1     Running   0          1m
# gpu-telemetry-collector-xxx            1/1     Running   0          1m
# gpu-telemetry-streamer-xxx             1/1     Running   0          1m
# gpu-telemetry-api-xxx                  1/1     Running   0          1m

# Check services
kubectl get svc -n gpu-telemetry

# Check logs of each component
kubectl logs -l app.kubernetes.io/component=mq-server -n gpu-telemetry
kubectl logs -l app.kubernetes.io/component=collector -n gpu-telemetry
kubectl logs -l app.kubernetes.io/component=streamer -n gpu-telemetry
kubectl logs -l app.kubernetes.io/component=api -n gpu-telemetry
```

### Step 8: Access the API

```powershell
# Port-forward the API service to your localhost
kubectl port-forward svc/gpu-telemetry-api 8080:8080 -n gpu-telemetry

# Now open a NEW terminal and test:
curl http://localhost:8080/health
# Should return: {"status":"healthy"}

# Open Swagger UI in browser:
# http://localhost:8080/swagger/index.html

# List GPUs:
curl http://localhost:8080/api/v1/gpus
```

---

## Quick Commands Reference

```powershell
# View all pods
kubectl get pods -n gpu-telemetry

# View logs for a specific component
kubectl logs -l app.kubernetes.io/component=collector -n gpu-telemetry -f

# Scale collectors (e.g., to 3 replicas)
kubectl scale deployment gpu-telemetry-collector --replicas=3 -n gpu-telemetry

# Restart a deployment
kubectl rollout restart deployment gpu-telemetry-streamer -n gpu-telemetry

# Delete and redeploy
helm uninstall gpu-telemetry -n gpu-telemetry
helm install gpu-telemetry deployments/helm/gpu-telemetry-pipeline -n gpu-telemetry

# Delete the entire KIND cluster
kind delete cluster --name gpu-telemetry
```

---

## Troubleshooting

### Problem: Pods stuck in "Pending"
```powershell
kubectl describe pod <pod-name> -n gpu-telemetry
# Look for events at the bottom
```

### Problem: Pods in "CrashLoopBackOff"
```powershell
kubectl logs <pod-name> -n gpu-telemetry --previous
# Shows logs from the crashed container
```

### Problem: Can't connect to API
```powershell
# Check if service exists
kubectl get svc -n gpu-telemetry

# Check endpoints
kubectl get endpoints gpu-telemetry-api -n gpu-telemetry
```

### Problem: InfluxDB connection errors
```powershell
# Check InfluxDB is running
kubectl get pods -l app=influxdb -n gpu-telemetry

# Check InfluxDB logs
kubectl logs -l app=influxdb -n gpu-telemetry
```

---

## Architecture in Kubernetes

```
┌─────────────────────────────────────────────────────────────────────┐
│                    KIND Cluster (gpu-telemetry)                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                 Namespace: gpu-telemetry                       │  │
│  │                                                                │  │
│  │  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐     │  │
│  │  │  Streamer   │────▶│  MQ Server  │────▶│  Collector  │     │  │
│  │  │    Pod      │     │    Pod      │     │    Pod(s)   │     │  │
│  │  └─────────────┘     └─────────────┘     └──────┬──────┘     │  │
│  │        │                   │                     │            │  │
│  │        │                   │                     ▼            │  │
│  │        │                   │              ┌─────────────┐     │  │
│  │  ConfigMap:                │              │  InfluxDB   │     │  │
│  │  gpu-metrics-data          │              │    Pod      │     │  │
│  │  (CSV file)                │              └──────┬──────┘     │  │
│  │                            │                     │            │  │
│  │                      ┌─────────────┐             │            │  │
│  │                      │  API Pod    │◀────────────┘            │  │
│  │                      └──────┬──────┘                          │  │
│  │                             │                                 │  │
│  └─────────────────────────────┼─────────────────────────────────┘  │
│                                │                                     │
└────────────────────────────────┼─────────────────────────────────────┘
                                 │ port-forward :8080
                                 ▼
                          Your Browser
                    http://localhost:8080/swagger/
```
