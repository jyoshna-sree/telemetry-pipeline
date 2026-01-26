# GPU Telemetry Pipeline - KIND Deployment Script
# Run this script from the project root directory

param(
    [switch]$Clean,      # Delete existing cluster first
    [switch]$BuildOnly,  # Only build images, don't deploy
    [switch]$SkipBuild   # Skip building images
)

$ErrorActionPreference = "Stop"
$ClusterName = "gpu-telemetry"
$Namespace = "gpu-telemetry"
$ProjectRoot = $PSScriptRoot | Split-Path -Parent | Split-Path -Parent

Write-Host "============================================" -ForegroundColor Cyan
Write-Host "  GPU Telemetry Pipeline - KIND Deployment  " -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""

# Check prerequisites
function Check-Prerequisites {
    Write-Host "[1/7] Checking prerequisites..." -ForegroundColor Yellow
    
    $missing = @()
    
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
        $missing += "docker"
    }
    if (-not (Get-Command kind -ErrorAction SilentlyContinue)) {
        $missing += "kind"
    }
    if (-not (Get-Command kubectl -ErrorAction SilentlyContinue)) {
        $missing += "kubectl"
    }
    if (-not (Get-Command helm -ErrorAction SilentlyContinue)) {
        $missing += "helm"
    }
    
    if ($missing.Count -gt 0) {
        Write-Host "ERROR: Missing required tools: $($missing -join ', ')" -ForegroundColor Red
        Write-Host "Please install them first. See DEPLOYMENT.md for instructions." -ForegroundColor Red
        exit 1
    }
    
    # Check Docker is running
    try {
        docker ps | Out-Null
    } catch {
        Write-Host "ERROR: Docker is not running. Please start Docker Desktop." -ForegroundColor Red
        exit 1
    }
    
    Write-Host "  All prerequisites OK" -ForegroundColor Green
}

# Clean up existing cluster
function Remove-ExistingCluster {
    if ($Clean) {
        Write-Host "[*] Cleaning up existing cluster..." -ForegroundColor Yellow
        kind delete cluster --name $ClusterName 2>$null
        Write-Host "  Cluster deleted" -ForegroundColor Green
    }
}

# Create KIND cluster
function Create-Cluster {
    Write-Host "[2/7] Creating KIND cluster..." -ForegroundColor Yellow
    
    $existing = kind get clusters 2>$null | Where-Object { $_ -eq $ClusterName }
    if ($existing) {
        Write-Host "  Cluster '$ClusterName' already exists, using it" -ForegroundColor Cyan
    } else {
        kind create cluster --name $ClusterName --config "$ProjectRoot\deployments\kind\kind-config.yaml"
        Write-Host "  Cluster created" -ForegroundColor Green
    }
    
    # Set kubectl context
    kubectl cluster-info --context kind-$ClusterName | Out-Null
}

# Build Docker images
function Build-Images {
    if ($SkipBuild) {
        Write-Host "[3/7] Skipping image build (--SkipBuild flag)" -ForegroundColor Yellow
        return
    }
    
    Write-Host "[3/7] Building Docker images..." -ForegroundColor Yellow
    
    Push-Location $ProjectRoot
    
    $images = @(
        @{ Name = "mq-server"; Dockerfile = "deployments/docker/mq-server.Dockerfile" },
        @{ Name = "streamer"; Dockerfile = "deployments/docker/streamer.Dockerfile" },
        @{ Name = "collector"; Dockerfile = "deployments/docker/collector.Dockerfile" },
        @{ Name = "api"; Dockerfile = "deployments/docker/api.Dockerfile" }
    )
    
    foreach ($img in $images) {
        Write-Host "  Building $($img.Name)..." -ForegroundColor Cyan
        docker build -t "gpu-telemetry-pipeline/$($img.Name):latest" -f $img.Dockerfile . | Out-Null
        if ($LASTEXITCODE -ne 0) {
            Write-Host "ERROR: Failed to build $($img.Name)" -ForegroundColor Red
            exit 1
        }
    }
    
    Pop-Location
    Write-Host "  All images built" -ForegroundColor Green
}

# Load images into KIND
function Load-Images {
    if ($SkipBuild) {
        Write-Host "[4/7] Skipping image load (--SkipBuild flag)" -ForegroundColor Yellow
        return
    }
    
    Write-Host "[4/7] Loading images into KIND cluster..." -ForegroundColor Yellow
    
    $images = @("mq-server", "streamer", "collector", "api")
    
    foreach ($img in $images) {
        Write-Host "  Loading $img..." -ForegroundColor Cyan
        kind load docker-image "gpu-telemetry-pipeline/${img}:latest" --name $ClusterName
    }
    
    Write-Host "  All images loaded" -ForegroundColor Green
}

# Deploy InfluxDB
function Deploy-InfluxDB {
    Write-Host "[5/7] Deploying InfluxDB..." -ForegroundColor Yellow
    
    # Create namespace if not exists
    kubectl create namespace $Namespace 2>$null
    
    # Deploy InfluxDB
    kubectl apply -f "$ProjectRoot\deployments\kind\influxdb.yaml" -n $Namespace
    
    # Wait for InfluxDB
    Write-Host "  Waiting for InfluxDB to be ready..." -ForegroundColor Cyan
    kubectl wait --for=condition=ready pod -l app=influxdb -n $Namespace --timeout=120s
    
    Write-Host "  InfluxDB deployed" -ForegroundColor Green
}

# Create sample data ConfigMap
function Create-SampleData {
    Write-Host "[6/7] Creating sample data ConfigMap..." -ForegroundColor Yellow
    
    $csvPath = "$ProjectRoot\..\dcgm_metrics_20250718_134233.csv"
    if (-not (Test-Path $csvPath)) {
        # Try alternate location
        $csvPath = "C:\Users\byreddy\Desktop\ciscoproject\dcgm_metrics_20250718_134233.csv"
    }
    
    if (Test-Path $csvPath) {
        kubectl delete configmap gpu-metrics-data -n $Namespace 2>$null
        kubectl create configmap gpu-metrics-data --from-file=telemetry.csv=$csvPath -n $Namespace
        Write-Host "  ConfigMap created from $csvPath" -ForegroundColor Green
    } else {
        Write-Host "  WARNING: CSV file not found, skipping ConfigMap creation" -ForegroundColor Yellow
        Write-Host "  You can create it manually with:" -ForegroundColor Yellow
        Write-Host "  kubectl create configmap gpu-metrics-data --from-file=telemetry.csv=<your-csv-path> -n $Namespace" -ForegroundColor Gray
    }
}

# Deploy application with Helm
function Deploy-Application {
    if ($BuildOnly) {
        Write-Host "[7/7] Skipping deployment (--BuildOnly flag)" -ForegroundColor Yellow
        return
    }
    
    Write-Host "[7/7] Deploying application with Helm..." -ForegroundColor Yellow
    
    # Uninstall existing release
    helm uninstall gpu-telemetry -n $Namespace 2>$null
    
    # Install Helm chart
    helm install gpu-telemetry "$ProjectRoot\deployments\helm\gpu-telemetry-pipeline" `
        --namespace $Namespace `
        --set influxdb.url="http://influxdb:8086" `
        --set influxdb.token="my-super-secret-token" `
        --set influxdb.org="cisco" `
        --set influxdb.bucket="gpu_telemetry" `
        --set streamer.data.existingConfigMap="gpu-metrics-data" `
        --set streamer.data.csvPath="/data/telemetry.csv" `
        --set collector.replicaCount=2
    
    # Wait for pods
    Write-Host "  Waiting for pods to be ready..." -ForegroundColor Cyan
    Start-Sleep -Seconds 5  # Give time for pods to be created
    kubectl wait --for=condition=ready pod -l app.kubernetes.io/instance=gpu-telemetry -n $Namespace --timeout=180s
    
    Write-Host "  Application deployed" -ForegroundColor Green
}

# Show status
function Show-Status {
    Write-Host ""
    Write-Host "============================================" -ForegroundColor Cyan
    Write-Host "  Deployment Complete!                      " -ForegroundColor Cyan
    Write-Host "============================================" -ForegroundColor Cyan
    Write-Host ""
    
    Write-Host "Pods:" -ForegroundColor Yellow
    kubectl get pods -n $Namespace
    
    Write-Host ""
    Write-Host "Services:" -ForegroundColor Yellow
    kubectl get svc -n $Namespace
    
    Write-Host ""
    Write-Host "============================================" -ForegroundColor Cyan
    Write-Host "  How to Access                             " -ForegroundColor Cyan
    Write-Host "============================================" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "1. Access API (in a new terminal):" -ForegroundColor Yellow
    Write-Host "   kubectl port-forward svc/gpu-telemetry-api 8080:8080 -n $Namespace" -ForegroundColor White
    Write-Host ""
    Write-Host "2. Open Swagger UI:" -ForegroundColor Yellow
    Write-Host "   http://localhost:8080/swagger/index.html" -ForegroundColor White
    Write-Host ""
    Write-Host "3. Test API:" -ForegroundColor Yellow
    Write-Host "   curl http://localhost:8080/health" -ForegroundColor White
    Write-Host "   curl http://localhost:8080/api/v1/gpus" -ForegroundColor White
    Write-Host ""
    Write-Host "4. View logs:" -ForegroundColor Yellow
    Write-Host "   kubectl logs -l app.kubernetes.io/component=collector -n $Namespace -f" -ForegroundColor White
    Write-Host ""
    Write-Host "5. Scale collectors:" -ForegroundColor Yellow
    Write-Host "   kubectl scale deployment gpu-telemetry-collector --replicas=3 -n $Namespace" -ForegroundColor White
    Write-Host ""
}

# Main execution
Check-Prerequisites
Remove-ExistingCluster
Create-Cluster
Build-Images
Load-Images

if (-not $BuildOnly) {
    Deploy-InfluxDB
    Create-SampleData
    Deploy-Application
    Show-Status
} else {
    Write-Host ""
    Write-Host "Images built successfully. Run without --BuildOnly to deploy." -ForegroundColor Green
}
