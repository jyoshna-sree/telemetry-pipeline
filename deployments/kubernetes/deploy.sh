#!/bin/bash
# GPU Telemetry Pipeline - Kubernetes Deployment Script

set -e

NAMESPACE="gpu-telemetry"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "========================================"
echo "  GPU Telemetry Pipeline - K8s Deploy  "
echo "========================================"

# Apply all manifests in order
echo "[1/7] Creating namespace..."
kubectl apply -f "$SCRIPT_DIR/namespace.yaml"

echo "[2/7] Creating ConfigMap..."
kubectl apply -f "$SCRIPT_DIR/configmap.yaml"

echo "[3/7] Creating Secret..."
kubectl apply -f "$SCRIPT_DIR/secret.yaml"

echo "[4/7] Deploying InfluxDB..."
kubectl apply -f "$SCRIPT_DIR/influxdb.yaml"
kubectl wait --for=condition=ready pod -l app=influxdb -n $NAMESPACE --timeout=120s

echo "[5/7] Deploying MQ Server..."
kubectl apply -f "$SCRIPT_DIR/mq-server.yaml"
kubectl wait --for=condition=ready pod -l app=mq-server -n $NAMESPACE --timeout=60s

echo "[6/7] Deploying Collector..."
kubectl apply -f "$SCRIPT_DIR/collector.yaml"

echo "[7/7] Deploying Streamer and API..."
kubectl apply -f "$SCRIPT_DIR/streamer.yaml"
kubectl apply -f "$SCRIPT_DIR/api.yaml"

echo ""
echo "Waiting for all pods to be ready..."
kubectl wait --for=condition=ready pod -l app=api -n $NAMESPACE --timeout=60s

echo ""
echo "========================================"
echo "  Deployment Complete!                 "
echo "========================================"
echo ""
echo "Pods:"
kubectl get pods -n $NAMESPACE
echo ""
echo "Services:"
kubectl get svc -n $NAMESPACE
echo ""
echo "Access API: http://localhost:30080/swagger/index.html"
echo ""
