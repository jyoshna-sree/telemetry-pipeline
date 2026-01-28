# Integration tests for GPU Telemetry Pipeline (PowerShell version)
# This script tests the deployed system end-to-end

param(
    [string]$ApiUrl = "http://localhost:30080",
    [string]$Namespace = "gpu-telemetry",
    [int]$Timeout = 300
)

$ErrorActionPreference = "Stop"

# Test counters
$script:TestsPassed = 0
$script:TestsFailed = 0

# Helper functions
function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Green
}

function Write-Error {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[WARN] $Message" -ForegroundColor Yellow
}

function Test-Endpoint {
    param(
        [string]$Name,
        [string]$Method,
        [string]$Endpoint,
        [int]$ExpectedStatus,
        [string]$Data = $null
    )

    Write-Info "Testing: $Name"
    
    try {
        $uri = "$ApiUrl$Endpoint"
        
        if ($Method -eq "GET") {
            $response = Invoke-WebRequest -Uri $uri -Method GET -UseBasicParsing -ErrorAction Stop
        } elseif ($Method -eq "POST") {
            $response = Invoke-WebRequest -Uri $uri -Method POST -Body $Data -ContentType "application/json" -UseBasicParsing -ErrorAction Stop
        }
        
        if ($response.StatusCode -eq $ExpectedStatus) {
            Write-Info "✓ $Name passed (HTTP $($response.StatusCode))"
            $script:TestsPassed++
            return $true
        } else {
            Write-Error "✗ $Name failed (expected HTTP $ExpectedStatus, got $($response.StatusCode))"
            $script:TestsFailed++
            return $false
        }
    } catch {
        $statusCode = $_.Exception.Response.StatusCode.value__
        if ($statusCode -eq $ExpectedStatus) {
            Write-Info "✓ $Name passed (HTTP $statusCode)"
            $script:TestsPassed++
            return $true
        } else {
            Write-Error "✗ $Name failed (expected HTTP $ExpectedStatus, got $statusCode)"
            Write-Error "Error: $($_.Exception.Message)"
            $script:TestsFailed++
            return $false
        }
    }
}

function Wait-ForPods {
    Write-Info "Waiting for pods to be ready..."
    
    if (-not (Get-Command kubectl -ErrorAction SilentlyContinue)) {
        Write-Warn "kubectl not found, skipping pod checks"
        return $true
    }
    
    $maxAttempts = 60
    $attempt = 0
    
    while ($attempt -lt $maxAttempts) {
        $pods = kubectl get pods -n $Namespace --no-headers 2>$null
        if ($LASTEXITCODE -eq 0) {
            $notReady = $pods | Where-Object { $_ -notmatch "Running|Completed" }
            if (-not $notReady) {
                Write-Info "All pods are ready"
                return $true
            }
        }
        $attempt++
        Start-Sleep -Seconds 5
    }
    
    Write-Error "Timeout waiting for pods to be ready"
    kubectl get pods -n $Namespace
    return $false
}

function Wait-ForApi {
    Write-Info "Waiting for API to be ready..."
    
    $maxAttempts = 60
    $attempt = 0
    
    while ($attempt -lt $maxAttempts) {
        try {
            $response = Invoke-WebRequest -Uri "$ApiUrl/health" -Method GET -UseBasicParsing -TimeoutSec 5 -ErrorAction Stop
            Write-Info "API is ready"
            return $true
        } catch {
            # Continue waiting
        }
        $attempt++
        Start-Sleep -Seconds 5
    }
    
    Write-Error "Timeout waiting for API"
    return $false
}

# Main test execution
function Main {
    Write-Host "=========================================="
    Write-Host "GPU Telemetry Pipeline Integration Tests"
    Write-Host "=========================================="
    Write-Host "API URL: $ApiUrl"
    Write-Host "Namespace: $Namespace"
    Write-Host ""
    
    # Wait for pods
    Wait-ForPods | Out-Null
    
    # Wait for API
    if (-not (Wait-ForApi)) {
        Write-Error "API is not ready, aborting tests"
        exit 1
    }
    
    Write-Host ""
    Write-Info "Starting API endpoint tests..."
    Write-Host ""
    
    # Test 1: Health check
    Test-Endpoint -Name "Health Check" -Method "GET" -Endpoint "/health" -ExpectedStatus 200
    
    # Test 2: Ready check
    Test-Endpoint -Name "Ready Check" -Method "GET" -Endpoint "/ready" -ExpectedStatus 200
    
    # Test 3: List GPUs
    Test-Endpoint -Name "List GPUs" -Method "GET" -Endpoint "/api/v1/gpus" -ExpectedStatus 200
    
    # Test 4: Get GPU list and extract first GPU ID
    Write-Info "Extracting GPU ID for further tests..."
    try {
        $gpuList = Invoke-RestMethod -Uri "$ApiUrl/api/v1/gpus" -Method GET -ErrorAction Stop
        $gpuId = $gpuList.data[0]
        
        if ($gpuId) {
            Write-Info "Using GPU ID: $gpuId"
            Write-Host ""
            
            # Test 5: Get GPU Info
            Test-Endpoint -Name "Get GPU Info" -Method "GET" -Endpoint "/api/v1/gpus/$gpuId" -ExpectedStatus 200
            
            # Test 6: Get GPU Telemetry
            Test-Endpoint -Name "Get GPU Telemetry" -Method "GET" -Endpoint "/api/v1/gpus/$gpuId/telemetry" -ExpectedStatus 200
            
            # Test 7: Get GPU Telemetry with limit
            Test-Endpoint -Name "Get GPU Telemetry with limit" -Method "GET" -Endpoint "/api/v1/gpus/$gpuId/telemetry?limit=10" -ExpectedStatus 200
            
            # Test 8: List metric names for GPU
            Test-Endpoint -Name "List Metric Names for GPU" -Method "GET" -Endpoint "/api/v1/gpus/$gpuId/metrics" -ExpectedStatus 200
        } else {
            Write-Warn "No GPUs found, skipping GPU-specific tests"
        }
    } catch {
        Write-Warn "Could not extract GPU ID, skipping GPU-specific tests"
    }
    
    # Test 9: List all metrics
    Test-Endpoint -Name "List All Metrics" -Method "GET" -Endpoint "/api/v1/metrics" -ExpectedStatus 200
    
    # Test 10: Get stats
    Test-Endpoint -Name "Get System Stats" -Method "GET" -Endpoint "/api/v1/stats" -ExpectedStatus 200
    
    # Test 11: Swagger UI
    try {
        $swaggerStatus = (Invoke-WebRequest -Uri "$ApiUrl/swagger/index.html" -Method GET -UseBasicParsing -ErrorAction Stop).StatusCode
        if ($swaggerStatus -eq 200 -or $swaggerStatus -eq 302) {
            Write-Info "✓ Swagger UI accessible (HTTP $swaggerStatus)"
            $script:TestsPassed++
        }
    } catch {
        Write-Error "✗ Swagger UI not accessible"
        $script:TestsFailed++
    }
    
    # Test 12: Invalid GPU ID
    Test-Endpoint -Name "Invalid GPU ID" -Method "GET" -Endpoint "/api/v1/gpus/invalid-uuid-12345" -ExpectedStatus 404
    
    # Test 13: Invalid endpoint
    Test-Endpoint -Name "Invalid Endpoint" -Method "GET" -Endpoint "/api/v1/invalid" -ExpectedStatus 404
    
    Write-Host ""
    Write-Host "=========================================="
    Write-Host "Test Summary"
    Write-Host "=========================================="
    Write-Host "Tests Passed: $script:TestsPassed"
    Write-Host "Tests Failed: $script:TestsFailed"
    Write-Host ""
    
    if ($script:TestsFailed -eq 0) {
        Write-Info "All tests passed! ✓"
        exit 0
    } else {
        Write-Error "Some tests failed ✗"
        exit 1
    }
}

# Run main function
Main
