// Package storage provides telemetry data storage backends.
package storage

import (
	"context"
	"fmt"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/write"

	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
)

// InfluxDBWriteStorage implements Storage (read + write) for InfluxDB.
// Used by the collector to store telemetry data.
type InfluxDBWriteStorage struct {
	client   influxdb2.Client
	writeAPI api.WriteAPIBlocking
	config   InfluxDBConfig

	// Local cache for GPU info
	gpuCache map[string]*models.GPUInfo

	// Stats
	totalWrites int64
}

// NewInfluxDBWriteStorage creates a new read/write InfluxDB storage backend.
// Used by the collector to store metrics.
func NewInfluxDBWriteStorage(config InfluxDBConfig) (*InfluxDBWriteStorage, error) {
	client := influxdb2.NewClient(config.URL, config.Token)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	health, err := client.Health(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to InfluxDB: %w", err)
	}
	if health.Status != "pass" {
		return nil, fmt.Errorf("InfluxDB health check failed: %s", health.Status)
	}

	return &InfluxDBWriteStorage{
		client:   client,
		writeAPI: client.WriteAPIBlocking(config.Org, config.Bucket),
		config:   config,
		gpuCache: make(map[string]*models.GPUInfo),
	}, nil
}

// Store stores a single metric.
func (s *InfluxDBWriteStorage) Store(ctx context.Context, metric *models.GPUMetric) error {
	point := influxdb2.NewPointWithMeasurement(metric.MetricName).
		AddTag("uuid", metric.UUID).
		AddTag("hostname", metric.Hostname).
		AddTag("gpu_id", fmt.Sprintf("%d", metric.GPUID)).
		AddTag("device", metric.Device).
		AddTag("model", metric.ModelName).
		AddTag("container", metric.Container).
		AddTag("pod", metric.Pod).
		AddTag("namespace", metric.Namespace).
		AddField("value", metric.Value).
		SetTime(metric.Timestamp)

	err := s.writeAPI.WritePoint(ctx, point)
	if err != nil {
		return fmt.Errorf("failed to write to InfluxDB: %w", err)
	}

	s.updateGPUCache(metric)
	s.totalWrites++

	return nil
}

// StoreBatch stores multiple metrics efficiently.
func (s *InfluxDBWriteStorage) StoreBatch(ctx context.Context, metrics []*models.GPUMetric) error {
	points := make([]*write.Point, 0, len(metrics))

	for _, metric := range metrics {
		point := influxdb2.NewPointWithMeasurement(metric.MetricName).
			AddTag("uuid", metric.UUID).
			AddTag("hostname", metric.Hostname).
			AddTag("gpu_id", fmt.Sprintf("%d", metric.GPUID)).
			AddTag("device", metric.Device).
			AddTag("model", metric.ModelName).
			AddTag("container", metric.Container).
			AddTag("pod", metric.Pod).
			AddTag("namespace", metric.Namespace).
			AddField("value", metric.Value).
			SetTime(metric.Timestamp)

		points = append(points, point)
		s.updateGPUCache(metric)
	}

	err := s.writeAPI.WritePoint(ctx, points...)
	if err != nil {
		return fmt.Errorf("failed to write batch to InfluxDB: %w", err)
	}

	s.totalWrites += int64(len(metrics))
	return nil
}

// updateGPUCache updates the local GPU info cache.
func (s *InfluxDBWriteStorage) updateGPUCache(metric *models.GPUMetric) {
	gpu, exists := s.gpuCache[metric.UUID]
	if !exists {
		s.gpuCache[metric.UUID] = &models.GPUInfo{
			UUID:      metric.UUID,
			GPUID:     metric.GPUID,
			Device:    metric.Device,
			ModelName: metric.ModelName,
			Hostname:  metric.Hostname,
			FirstSeen: metric.Timestamp,
			LastSeen:  metric.Timestamp,
		}
	} else {
		if metric.Timestamp.After(gpu.LastSeen) {
			gpu.LastSeen = metric.Timestamp
		}
		if metric.Timestamp.Before(gpu.FirstSeen) {
			gpu.FirstSeen = metric.Timestamp
		}
	}
}

// GetGPUs returns all known GPU IDs from the cache.
func (s *InfluxDBWriteStorage) GetGPUs(ctx context.Context) ([]string, error) {
	gpus := make([]string, 0, len(s.gpuCache))
	for uuid := range s.gpuCache {
		gpus = append(gpus, uuid)
	}
	return gpus, nil
}

// GetGPUByUUID returns a GPU by its UUID.
func (s *InfluxDBWriteStorage) GetGPUByUUID(ctx context.Context, uuid string) (*models.GPUInfo, error) {
	gpu, exists := s.gpuCache[uuid]
	if !exists {
		return nil, nil
	}
	gpuCopy := *gpu
	return &gpuCopy, nil
}

// GetTelemetry is not implemented for write storage - use read storage for queries.
func (s *InfluxDBWriteStorage) GetTelemetry(ctx context.Context, query *models.TelemetryQuery) ([]*models.GPUMetric, error) {
	return nil, fmt.Errorf("GetTelemetry not implemented for write storage")
}

// GetMetricsByGPU is not implemented for write storage.
func (s *InfluxDBWriteStorage) GetMetricsByGPU(ctx context.Context, uuid string, startTime, endTime *time.Time) ([]*models.GPUMetric, error) {
	return nil, fmt.Errorf("GetMetricsByGPU not implemented for write storage")
}

// Cleanup is handled by InfluxDB's built-in retention policies.
func (s *InfluxDBWriteStorage) Cleanup(ctx context.Context, retentionPeriod time.Duration) (int, error) {
	return 0, nil
}

// Stats returns storage statistics.
func (s *InfluxDBWriteStorage) Stats() StorageStats {
	return StorageStats{
		TotalMetrics: s.totalWrites,
		TotalGPUs:    len(s.gpuCache),
	}
}

// Close closes the InfluxDB client.
func (s *InfluxDBWriteStorage) Close() error {
	s.client.Close()
	return nil
}
