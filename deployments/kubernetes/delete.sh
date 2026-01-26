#!/bin/bash
# GPU Telemetry Pipeline - Delete Kubernetes Resources

NAMESPACE="gpu-telemetry"

echo "Deleting GPU Telemetry Pipeline resources..."

kubectl delete namespace $NAMESPACE --ignore-not-found

echo "Done!"
