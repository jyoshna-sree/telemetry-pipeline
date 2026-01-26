// Package models defines the core data structures for GPU telemetry.
package models

import (
	"encoding/json"
	"time"
)

// GPUMetric represents a single DCGM telemetry data point collected from a GPU.
// This is the primary data structure used throughout the pipeline.
type GPUMetric struct {
	// Timestamp when the metric was processed (pipeline processing time)
	Timestamp time.Time `json:"timestamp"`

	// MetricName is the DCGM metric identifier (e.g., DCGM_FI_DEV_GPU_UTIL)
	MetricName string `json:"metric_name"`

	// GPUID is the local GPU index on the host (0-7 for DGX systems)
	GPUID int `json:"gpu_id"`

	// Device is the device name (e.g., nvidia0)
	Device string `json:"device"`

	// UUID is the unique identifier for the GPU across the cluster
	UUID string `json:"uuid"`

	// ModelName is the GPU model (e.g., NVIDIA H100 80GB HBM3)
	ModelName string `json:"model_name"`

	// Hostname is the host where the GPU is located
	Hostname string `json:"hostname"`

	// Container is the Kubernetes container name (optional)
	Container string `json:"container,omitempty"`

	// Pod is the Kubernetes pod name (optional)
	Pod string `json:"pod,omitempty"`

	// Namespace is the Kubernetes namespace (optional)
	Namespace string `json:"namespace,omitempty"`

	// Value is the metric value (utilization %, clock MHz, etc.)
	Value float64 `json:"value"`

	// Labels contains additional key-value metadata from the original telemetry
	Labels map[string]string `json:"labels,omitempty"`
}

// GPUInfo represents summary information about a GPU.
type GPUInfo struct {
	UUID      string    `json:"uuid"`
	GPUID     int       `json:"gpu_id"`
	Device    string    `json:"device"`
	ModelName string    `json:"model_name"`
	Hostname  string    `json:"hostname"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// MetricBatch represents a collection of metrics for batch processing.
type MetricBatch struct {
	// BatchID is a unique identifier for this batch
	BatchID string `json:"batch_id"`

	// Source identifies the streamer that created this batch
	Source string `json:"source"`

	// CollectedAt is when the batch was created
	CollectedAt time.Time `json:"collected_at"`

	// Metrics is the list of GPU metrics in this batch
	Metrics []GPUMetric `json:"metrics"`
}

// TelemetryQuery represents query parameters for fetching telemetry.
type TelemetryQuery struct {
	// UUID filters by GPU UUID
	UUID string `json:"uuid,omitempty"`

	// Hostname filters by hostname
	Hostname string `json:"hostname,omitempty"`

	// GPUID filters by local GPU ID
	GPUID *int `json:"gpu_id,omitempty"`

	// MetricName filters by metric type
	MetricName string `json:"metric_name,omitempty"`

	// StartTime is the inclusive start of the time window
	StartTime *time.Time `json:"start_time,omitempty"`

	// EndTime is the inclusive end of the time window
	EndTime *time.Time `json:"end_time,omitempty"`

	// Limit is the maximum number of results to return
	Limit int `json:"limit,omitempty"`

	// Offset for pagination
	Offset int `json:"offset,omitempty"`
}

// ToJSON serializes the GPUMetric to JSON bytes.
func (m *GPUMetric) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON deserializes JSON bytes into a GPUMetric.
func (m *GPUMetric) FromJSON(data []byte) error {
	return json.Unmarshal(data, m)
}

// ToJSON serializes the MetricBatch to JSON bytes.
func (b *MetricBatch) ToJSON() ([]byte, error) {
	return json.Marshal(b)
}

// FromJSON deserializes JSON bytes into a MetricBatch.
func (b *MetricBatch) FromJSON(data []byte) error {
	return json.Unmarshal(data, b)
}

// Common DCGM metric names.
const (
	MetricGPUUtil     = "DCGM_FI_DEV_GPU_UTIL"
	MetricMemCopyUtil = "DCGM_FI_DEV_MEM_COPY_UTIL"
	MetricSMClock     = "DCGM_FI_DEV_SM_CLOCK"
	MetricMemClock    = "DCGM_FI_DEV_MEM_CLOCK"
	MetricPowerUsage  = "DCGM_FI_DEV_POWER_USAGE"
	MetricTemperature = "DCGM_FI_DEV_GPU_TEMP"
	MetricMemUsed     = "DCGM_FI_DEV_FB_USED"
	MetricMemFree     = "DCGM_FI_DEV_FB_FREE"
)

// MetricUnit returns the unit for a given metric name.
func MetricUnit(metricName string) string {
	units := map[string]string{
		MetricGPUUtil:     "%",
		MetricMemCopyUtil: "%",
		MetricSMClock:     "MHz",
		MetricMemClock:    "MHz",
		MetricPowerUsage:  "W",
		MetricTemperature: "°C",
		MetricMemUsed:     "MiB",
		MetricMemFree:     "MiB",
	}
	if unit, ok := units[metricName]; ok {
		return unit
	}
	return ""
}
