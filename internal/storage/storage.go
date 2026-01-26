// Package storage provides telemetry data storage backends.
package storage

import (
	"context"
	"time"

	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
)

// ReadStorage defines read-only interface for querying telemetry data.
// Used by: API
type ReadStorage interface {
	// GetGPUs returns all known GPU IDs
	GetGPUs(ctx context.Context) ([]string, error)

	// GetTelemetry returns telemetry for a specific GPU
	GetTelemetry(ctx context.Context, query *models.TelemetryQuery) ([]*models.GPUMetric, error)

	// Close closes the storage
	Close() error
}

// Storage defines the full interface for telemetry data storage.
// Used by: Collector
type Storage interface {
	ReadStorage

	// Store stores a single metric
	Store(ctx context.Context, metric *models.GPUMetric) error

	// StoreBatch stores multiple metrics
	StoreBatch(ctx context.Context, metrics []*models.GPUMetric) error

	// GetGPUByUUID returns a GPU by its UUID
	GetGPUByUUID(ctx context.Context, uuid string) (*models.GPUInfo, error)

	// GetMetricsByGPU returns all metrics for a specific GPU UUID
	GetMetricsByGPU(ctx context.Context, uuid string, startTime, endTime *time.Time) ([]*models.GPUMetric, error)

	// Cleanup removes old data based on retention period
	Cleanup(ctx context.Context, retentionPeriod time.Duration) (int, error)

	// Stats returns storage statistics
	Stats() StorageStats
}

// StorageStats provides storage statistics.
type StorageStats struct {
	TotalMetrics  int64     `json:"total_metrics"`
	TotalGPUs     int       `json:"total_gpus"`
	OldestMetric  time.Time `json:"oldest_metric"`
	NewestMetric  time.Time `json:"newest_metric"`
	MemoryUsageKB int64     `json:"memory_usage_kb,omitempty"`
}
