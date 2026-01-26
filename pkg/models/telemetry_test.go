package models

import (
	"testing"
	"time"
)

func TestGPUMetricToJSON(t *testing.T) {
	metric := GPUMetric{
		Timestamp:  time.Now(),
		MetricName: MetricGPUUtil,
		GPUID:      0,
		Device:     "nvidia0",
		UUID:       "GPU-12345",
		ModelName:  "NVIDIA H100",
		Hostname:   "host-001",
		Value:      75.5,
	}

	data, err := metric.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}

func TestGPUMetricFromJSON(t *testing.T) {
	original := GPUMetric{
		Timestamp:  time.Now().Truncate(time.Second),
		MetricName: MetricGPUUtil,
		GPUID:      0,
		Device:     "nvidia0",
		UUID:       "GPU-12345",
		ModelName:  "NVIDIA H100",
		Hostname:   "host-001",
		Value:      75.5,
	}

	data, err := original.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	var decoded GPUMetric
	if err := decoded.FromJSON(data); err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if decoded.UUID != original.UUID {
		t.Error("UUID mismatch")
	}
	if decoded.Value != original.Value {
		t.Error("Value mismatch")
	}
	if decoded.MetricName != original.MetricName {
		t.Error("MetricName mismatch")
	}
}

func TestMetricBatchToJSON(t *testing.T) {
	batch := MetricBatch{
		BatchID:     "batch-123",
		Source:      "streamer-1",
		CollectedAt: time.Now(),
		Metrics: []GPUMetric{
			{
				Timestamp:  time.Now(),
				MetricName: MetricGPUUtil,
				UUID:       "GPU-1",
				Value:      50.0,
			},
			{
				Timestamp:  time.Now(),
				MetricName: MetricTemperature,
				UUID:       "GPU-1",
				Value:      65.0,
			},
		},
	}

	data, err := batch.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}

func TestMetricBatchFromJSON(t *testing.T) {
	original := MetricBatch{
		BatchID:     "batch-123",
		Source:      "streamer-1",
		CollectedAt: time.Now().Truncate(time.Second),
		Metrics: []GPUMetric{
			{
				Timestamp:  time.Now().Truncate(time.Second),
				MetricName: MetricGPUUtil,
				UUID:       "GPU-1",
				Value:      50.0,
			},
		},
	}

	data, err := original.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	var decoded MetricBatch
	if err := decoded.FromJSON(data); err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if decoded.BatchID != original.BatchID {
		t.Error("BatchID mismatch")
	}
	if decoded.Source != original.Source {
		t.Error("Source mismatch")
	}
	if len(decoded.Metrics) != len(original.Metrics) {
		t.Error("Metrics count mismatch")
	}
}

func TestMetricUnit(t *testing.T) {
	tests := []struct {
		metricName string
		expected   string
	}{
		{MetricGPUUtil, "%"},
		{MetricMemCopyUtil, "%"},
		{MetricSMClock, "MHz"},
		{MetricMemClock, "MHz"},
		{MetricPowerUsage, "W"},
		{MetricTemperature, "°C"},
		{MetricMemUsed, "MiB"},
		{MetricMemFree, "MiB"},
		{"UNKNOWN_METRIC", ""},
	}

	for _, tt := range tests {
		t.Run(tt.metricName, func(t *testing.T) {
			unit := MetricUnit(tt.metricName)
			if unit != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, unit)
			}
		})
	}
}

func TestMetricConstants(t *testing.T) {
	// Verify all constants are non-empty
	constants := []string{
		MetricGPUUtil,
		MetricMemCopyUtil,
		MetricSMClock,
		MetricMemClock,
		MetricPowerUsage,
		MetricTemperature,
		MetricMemUsed,
		MetricMemFree,
	}

	for _, c := range constants {
		if c == "" {
			t.Error("found empty metric constant")
		}
	}
}

func TestGPUInfo(t *testing.T) {
	info := GPUInfo{
		UUID:      "GPU-12345",
		GPUID:     0,
		Device:    "nvidia0",
		ModelName: "NVIDIA H100",
		Hostname:  "host-001",
		FirstSeen: time.Now().Add(-24 * time.Hour),
		LastSeen:  time.Now(),
	}

	if info.UUID != "GPU-12345" {
		t.Error("UUID mismatch")
	}
	if info.GPUID != 0 {
		t.Error("GPUID mismatch")
	}
	if info.LastSeen.Before(info.FirstSeen) {
		t.Error("LastSeen should be after FirstSeen")
	}
}

func TestTelemetryQuery(t *testing.T) {
	now := time.Now()
	gpuID := 0
	query := TelemetryQuery{
		UUID:       "GPU-12345",
		Hostname:   "host-001",
		GPUID:      &gpuID,
		MetricName: MetricGPUUtil,
		StartTime:  &now,
		EndTime:    &now,
		Limit:      100,
		Offset:     0,
	}

	if query.UUID != "GPU-12345" {
		t.Error("UUID mismatch")
	}
	if query.GPUID == nil || *query.GPUID != 0 {
		t.Error("GPUID mismatch")
	}
	if query.Limit != 100 {
		t.Error("Limit mismatch")
	}
}

func TestGPUMetricWithLabels(t *testing.T) {
	metric := GPUMetric{
		Timestamp:  time.Now(),
		MetricName: MetricGPUUtil,
		UUID:       "GPU-12345",
		Value:      75.5,
		Labels: map[string]string{
			"cluster": "prod",
			"region":  "us-west-1",
		},
	}

	if metric.Labels["cluster"] != "prod" {
		t.Error("label mismatch")
	}

	// Test JSON roundtrip with labels
	data, err := metric.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	var decoded GPUMetric
	if err := decoded.FromJSON(data); err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if decoded.Labels["cluster"] != "prod" {
		t.Error("label not preserved in JSON roundtrip")
	}
}

func TestGPUMetricWithKubernetesFields(t *testing.T) {
	metric := GPUMetric{
		Timestamp:  time.Now(),
		MetricName: MetricGPUUtil,
		UUID:       "GPU-12345",
		Value:      75.5,
		Container:  "training-container",
		Pod:        "training-job-abc123",
		Namespace:  "ml-workloads",
	}

	if metric.Container != "training-container" {
		t.Error("Container mismatch")
	}
	if metric.Pod != "training-job-abc123" {
		t.Error("Pod mismatch")
	}
	if metric.Namespace != "ml-workloads" {
		t.Error("Namespace mismatch")
	}

	// Test JSON roundtrip
	data, err := metric.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	var decoded GPUMetric
	if err := decoded.FromJSON(data); err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if decoded.Container != metric.Container {
		t.Error("Container not preserved")
	}
}
