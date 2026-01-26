package storage

import (
	"context"
	"testing"
	"time"

	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
)

// Note: InfluxDB storage tests require a live InfluxDB instance.
// These tests cover the interface definitions and mock implementations.
// For integration testing with InfluxDB, use Docker or a test instance.

// mockReadStorage implements ReadStorage for testing
type mockReadStorage struct{}

func (m *mockReadStorage) GetGPUs(ctx context.Context) ([]string, error) {
	return []string{"GPU-1", "GPU-2"}, nil
}

func (m *mockReadStorage) GetTelemetry(ctx context.Context, query *models.TelemetryQuery) ([]*models.GPUMetric, error) {
	return []*models.GPUMetric{}, nil
}

func (m *mockReadStorage) Close() error {
	return nil
}

// mockStorage implements Storage for testing
type mockStorage struct {
	mockReadStorage
	metrics []*models.GPUMetric
	gpus    map[string]*models.GPUInfo
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		metrics: make([]*models.GPUMetric, 0),
		gpus:    make(map[string]*models.GPUInfo),
	}
}

func (m *mockStorage) Store(ctx context.Context, metric *models.GPUMetric) error {
	m.metrics = append(m.metrics, metric)
	if _, exists := m.gpus[metric.UUID]; !exists {
		m.gpus[metric.UUID] = &models.GPUInfo{
			UUID:     metric.UUID,
			Hostname: metric.Hostname,
		}
	}
	return nil
}

func (m *mockStorage) StoreBatch(ctx context.Context, metrics []*models.GPUMetric) error {
	for _, metric := range metrics {
		if err := m.Store(ctx, metric); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockStorage) GetGPUByUUID(ctx context.Context, uuid string) (*models.GPUInfo, error) {
	gpu, exists := m.gpus[uuid]
	if !exists {
		return nil, nil
	}
	return gpu, nil
}

func (m *mockStorage) GetMetricsByGPU(ctx context.Context, uuid string, startTime, endTime *time.Time) ([]*models.GPUMetric, error) {
	var result []*models.GPUMetric
	for _, metric := range m.metrics {
		if metric.UUID == uuid {
			result = append(result, metric)
		}
	}
	return result, nil
}

func (m *mockStorage) Cleanup(ctx context.Context, retentionPeriod time.Duration) (int, error) {
	return 0, nil
}

func (m *mockStorage) Stats() StorageStats {
	return StorageStats{
		TotalMetrics: int64(len(m.metrics)),
		TotalGPUs:    len(m.gpus),
	}
}

func TestStorageStatsStruct(t *testing.T) {
	stats := StorageStats{
		TotalMetrics:  1000,
		TotalGPUs:     8,
		OldestMetric:  time.Now().Add(-24 * time.Hour),
		NewestMetric:  time.Now(),
		MemoryUsageKB: 1024,
	}

	if stats.TotalMetrics != 1000 {
		t.Errorf("expected 1000 metrics, got %d", stats.TotalMetrics)
	}
	if stats.TotalGPUs != 8 {
		t.Errorf("expected 8 GPUs, got %d", stats.TotalGPUs)
	}
	if stats.MemoryUsageKB != 1024 {
		t.Errorf("expected 1024 KB, got %d", stats.MemoryUsageKB)
	}
}

func TestReadStorageInterface(t *testing.T) {
	// This test verifies that the interface is properly defined
	var _ ReadStorage = (*mockReadStorage)(nil)
}

func TestStorageInterface(t *testing.T) {
	// This test verifies that the Storage interface extends ReadStorage
	var _ Storage = (*mockStorage)(nil)
}

func TestMockStorageStore(t *testing.T) {
	store := newMockStorage()
	ctx := context.Background()

	metric := &models.GPUMetric{
		Timestamp:  time.Now(),
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		UUID:       "GPU-12345",
		Hostname:   "host-001",
		Value:      75.5,
	}

	err := store.Store(ctx, metric)
	if err != nil {
		t.Fatalf("failed to store: %v", err)
	}

	stats := store.Stats()
	if stats.TotalMetrics != 1 {
		t.Errorf("expected 1 metric, got %d", stats.TotalMetrics)
	}
	if stats.TotalGPUs != 1 {
		t.Errorf("expected 1 GPU, got %d", stats.TotalGPUs)
	}
}

func TestMockStorageStoreBatch(t *testing.T) {
	store := newMockStorage()
	ctx := context.Background()

	metrics := []*models.GPUMetric{
		{UUID: "GPU-1", Value: 50.0},
		{UUID: "GPU-2", Value: 60.0},
		{UUID: "GPU-1", Value: 55.0},
	}

	err := store.StoreBatch(ctx, metrics)
	if err != nil {
		t.Fatalf("failed to store batch: %v", err)
	}

	stats := store.Stats()
	if stats.TotalMetrics != 3 {
		t.Errorf("expected 3 metrics, got %d", stats.TotalMetrics)
	}
	if stats.TotalGPUs != 2 {
		t.Errorf("expected 2 GPUs, got %d", stats.TotalGPUs)
	}
}

func TestMockStorageGetGPUByUUID(t *testing.T) {
	store := newMockStorage()
	ctx := context.Background()

	// Store a metric to create a GPU
	store.Store(ctx, &models.GPUMetric{
		UUID:     "GPU-12345",
		Hostname: "host-001",
	})

	// Get existing GPU
	gpu, err := store.GetGPUByUUID(ctx, "GPU-12345")
	if err != nil {
		t.Fatalf("failed to get GPU: %v", err)
	}
	if gpu == nil {
		t.Fatal("expected GPU, got nil")
	}
	if gpu.UUID != "GPU-12345" {
		t.Error("UUID mismatch")
	}

	// Get non-existent GPU
	gpu, err = store.GetGPUByUUID(ctx, "non-existent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gpu != nil {
		t.Error("expected nil for non-existent GPU")
	}
}

func TestMockStorageGetMetricsByGPU(t *testing.T) {
	store := newMockStorage()
	ctx := context.Background()

	// Store metrics for different GPUs
	store.Store(ctx, &models.GPUMetric{UUID: "GPU-1", Value: 50.0})
	store.Store(ctx, &models.GPUMetric{UUID: "GPU-1", Value: 55.0})
	store.Store(ctx, &models.GPUMetric{UUID: "GPU-2", Value: 60.0})

	// Get metrics for GPU-1
	metrics, err := store.GetMetricsByGPU(ctx, "GPU-1", nil, nil)
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	if len(metrics) != 2 {
		t.Errorf("expected 2 metrics for GPU-1, got %d", len(metrics))
	}
}

func TestMockStorageCleanup(t *testing.T) {
	store := newMockStorage()
	ctx := context.Background()

	removed, err := store.Cleanup(ctx, 24*time.Hour)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if removed != 0 {
		t.Error("mock cleanup should return 0")
	}
}

func TestMockReadStorageGetGPUs(t *testing.T) {
	store := &mockReadStorage{}
	ctx := context.Background()

	gpus, err := store.GetGPUs(ctx)
	if err != nil {
		t.Fatalf("failed to get GPUs: %v", err)
	}
	if len(gpus) != 2 {
		t.Errorf("expected 2 GPUs, got %d", len(gpus))
	}
}

func TestMockReadStorageClose(t *testing.T) {
	store := &mockReadStorage{}
	err := store.Close()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
