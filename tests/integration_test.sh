#!/bin/bash
# Integration tests for GPU Telemetry Pipeline
# This script tests the deployed system end-to-end

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
API_URL="${API_URL:-http://localhost:30080}"
NAMESPACE="${NAMESPACE:-gpu-telemetry}"
TIMEOUT="${TIMEOUT:-300}" # 5 minutes

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

test_endpoint() {
    local name=$1
    local method=$2
    local endpoint=$3
    local expected_status=$4
    local data=$5

    log_info "Testing: $name"
    
    if [ "$method" = "GET" ]; then
        response=$(curl -s -w "\n%{http_code}" -X GET "$API_URL$endpoint" || echo "000")
    elif [ "$method" = "POST" ]; then
        response=$(curl -s -w "\n%{http_code}" -X POST "$API_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data" || echo "000")
    fi

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | sed '$d')

    if [ "$http_code" = "$expected_status" ]; then
        log_info "✓ $name passed (HTTP $http_code)"
        ((TESTS_PASSED++))
        return 0
    else
        log_error "✗ $name failed (expected HTTP $expected_status, got $http_code)"
        log_error "Response: $body"
        ((TESTS_FAILED++))
        return 1
    fi
}

test_json_response() {
    local name=$1
    local endpoint=$2
    local json_path=$3
    local expected_value=$4

    log_info "Testing: $name"
    
    response=$(curl -s "$API_URL$endpoint" || echo "")
    if [ -z "$response" ]; then
        log_error "✗ $name failed (empty response)"
        ((TESTS_FAILED++))
        return 1
    fi

    # Use jq if available, otherwise basic grep
    if command -v jq &> /dev/null; then
        value=$(echo "$response" | jq -r "$json_path" 2>/dev/null || echo "")
    else
        # Fallback: basic extraction (not perfect but works for simple cases)
        value=$(echo "$response" | grep -o "\"$json_path\"[^,}]*" | cut -d'"' -f4 || echo "")
    fi

    if [ "$value" = "$expected_value" ] || [ -n "$value" ]; then
        log_info "✓ $name passed"
        ((TESTS_PASSED++))
        return 0
    else
        log_error "✗ $name failed (expected value not found)"
        log_error "Response: $response"
        ((TESTS_FAILED++))
        return 1
    fi
}

wait_for_pods() {
    log_info "Waiting for pods to be ready..."
    local max_attempts=60
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if kubectl get pods -n "$NAMESPACE" --no-headers 2>/dev/null | grep -v Running | grep -v Completed | wc -l | grep -q "^0$"; then
            log_info "All pods are ready"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 5
    done

    log_error "Timeout waiting for pods to be ready"
    kubectl get pods -n "$NAMESPACE"
    return 1
}

wait_for_api() {
    log_info "Waiting for API to be ready..."
    local max_attempts=60
    local attempt=0

    while [ $attempt -lt $max_attempts ]; do
        if curl -s -f "$API_URL/health" > /dev/null 2>&1; then
            log_info "API is ready"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 5
    done

    log_error "Timeout waiting for API"
    return 1
}

# Main test execution
main() {
    echo "=========================================="
    echo "GPU Telemetry Pipeline Integration Tests"
    echo "=========================================="
    echo "API URL: $API_URL"
    echo "Namespace: $NAMESPACE"
    echo ""

    # Check prerequisites
    if ! command -v curl &> /dev/null; then
        log_error "curl is required but not installed"
        exit 1
    fi

    if ! command -v kubectl &> /dev/null; then
        log_warn "kubectl not found, skipping pod checks"
    else
        # Wait for pods
        if ! wait_for_pods; then
            log_error "Pods are not ready, some tests may fail"
        fi
    fi

    # Wait for API
    if ! wait_for_api; then
        log_error "API is not ready, aborting tests"
        exit 1
    fi

    echo ""
    log_info "Starting API endpoint tests..."
    echo ""

    # Test 1: Health check
    test_endpoint "Health Check" "GET" "/health" "200"

    # Test 2: Ready check
    test_endpoint "Ready Check" "GET" "/ready" "200"

    # Test 3: List GPUs
    test_endpoint "List GPUs" "GET" "/api/v1/gpus" "200"

    # Test 4: Get GPU list and extract first GPU ID
    log_info "Extracting GPU ID for further tests..."
    gpu_list=$(curl -s "$API_URL/api/v1/gpus" || echo "")
    if [ -z "$gpu_list" ]; then
        log_warn "No GPUs found, some tests will be skipped"
        GPU_ID=""
    else
        # Extract first GPU UUID (basic extraction)
        if command -v jq &> /dev/null; then
            GPU_ID=$(echo "$gpu_list" | jq -r '.data[0]' 2>/dev/null || echo "")
        else
            # Fallback: extract from JSON manually
            GPU_ID=$(echo "$gpu_list" | grep -o '"[a-f0-9-]\{36\}"' | head -1 | tr -d '"' || echo "")
        fi

        if [ -n "$GPU_ID" ] && [ "$GPU_ID" != "null" ]; then
            log_info "Using GPU ID: $GPU_ID"
            echo ""

            # Test 5: Get GPU Info
            test_endpoint "Get GPU Info" "GET" "/api/v1/gpus/$GPU_ID" "200"

            # Test 6: Get GPU Telemetry
            test_endpoint "Get GPU Telemetry" "GET" "/api/v1/gpus/$GPU_ID/telemetry" "200"

            # Test 7: Get GPU Telemetry with time filter
            start_time=$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-1H +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "")
            if [ -n "$start_time" ]; then
                test_endpoint "Get GPU Telemetry with time filter" "GET" "/api/v1/gpus/$GPU_ID/telemetry?start_time=$start_time" "200"
            fi

            # Test 8: Get GPU Telemetry with limit
            test_endpoint "Get GPU Telemetry with limit" "GET" "/api/v1/gpus/$GPU_ID/telemetry?limit=10" "200"

            # Test 9: List metric names for GPU
            test_endpoint "List Metric Names for GPU" "GET" "/api/v1/gpus/$GPU_ID/metrics" "200"
        else
            log_warn "Could not extract GPU ID, skipping GPU-specific tests"
        fi
    fi

    # Test 10: List all metrics
    test_endpoint "List All Metrics" "GET" "/api/v1/metrics" "200"

    # Test 11: Get stats
    test_endpoint "Get System Stats" "GET" "/api/v1/stats" "200"

    # Test 12: Swagger UI (should return HTML)
    swagger_status=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/swagger/index.html" || echo "000")
    if [ "$swagger_status" = "200" ] || [ "$swagger_status" = "302" ]; then
        log_info "✓ Swagger UI accessible (HTTP $swagger_status)"
        ((TESTS_PASSED++))
    else
        log_error "✗ Swagger UI not accessible (HTTP $swagger_status)"
        ((TESTS_FAILED++))
    fi

    # Test 13: Invalid GPU ID (should return 404 or 400)
    test_endpoint "Invalid GPU ID" "GET" "/api/v1/gpus/invalid-uuid-12345" "404" || \
    test_endpoint "Invalid GPU ID" "GET" "/api/v1/gpus/invalid-uuid-12345" "400"

    # Test 14: Invalid endpoint (should return 404)
    test_endpoint "Invalid Endpoint" "GET" "/api/v1/invalid" "404"

    echo ""
    echo "=========================================="
    echo "Test Summary"
    echo "=========================================="
    echo "Tests Passed: $TESTS_PASSED"
    echo "Tests Failed: $TESTS_FAILED"
    echo ""

    if [ $TESTS_FAILED -eq 0 ]; then
        log_info "All tests passed! ✓"
        exit 0
    else
        log_error "Some tests failed ✗"
        exit 1
    fi
}

# Run main function
main
