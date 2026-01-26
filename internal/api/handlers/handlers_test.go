package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cisco/gpu-telemetry-pipeline/internal/storage"
	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
)

// mockStorage is a simple in-memory storage for testing
type mockStorage struct {
	metrics map[string][]*models.GPUMetric
	gpus    map[string]*models.GPUInfo
	mu      sync.RWMutex
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		metrics: make(map[string][]*models.GPUMetric),
		gpus:    make(map[string]*models.GPUInfo),
	}
}

func (s *mockStorage) Store(ctx context.Context, metric *models.GPUMetric) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.gpus[metric.UUID]; !exists {
		s.gpus[metric.UUID] = &models.GPUInfo{
			UUID:      metric.UUID,
			GPUID:     metric.GPUID,
			Device:    metric.Device,
			ModelName: metric.ModelName,
			Hostname:  metric.Hostname,
		}
	}
	s.metrics[metric.UUID] = append(s.metrics[metric.UUID], metric)
	return nil
}

func (s *mockStorage) StoreBatch(ctx context.Context, metrics []*models.GPUMetric) error {
	for _, m := range metrics {
		if err := s.Store(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *mockStorage) GetGPUs(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	gpus := make([]string, 0, len(s.gpus))
	for uuid := range s.gpus {
		gpus = append(gpus, uuid)
	}
	sort.Strings(gpus)
	return gpus, nil
}

func (s *mockStorage) GetGPUByUUID(ctx context.Context, uuid string) (*models.GPUInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	gpu, exists := s.gpus[uuid]
	if !exists {
		return nil, nil
	}
	gpuCopy := *gpu
	return &gpuCopy, nil
}

func (s *mockStorage) GetTelemetry(ctx context.Context, query *models.TelemetryQuery) ([]*models.GPUMetric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []*models.GPUMetric

	uuids := []string{}
	if query.UUID != "" {
		uuids = append(uuids, query.UUID)
	} else {
		for uuid := range s.metrics {
			uuids = append(uuids, uuid)
		}
	}

	for _, uuid := range uuids {
		for _, metric := range s.metrics[uuid] {
			if s.matchesQuery(metric, query) {
				metricCopy := *metric
				results = append(results, &metricCopy)
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	if query.Offset > 0 && query.Offset < len(results) {
		results = results[query.Offset:]
	} else if query.Offset >= len(results) {
		return []*models.GPUMetric{}, nil
	}

	if query.Limit > 0 && query.Limit < len(results) {
		results = results[:query.Limit]
	}

	return results, nil
}

func (s *mockStorage) matchesQuery(metric *models.GPUMetric, query *models.TelemetryQuery) bool {
	if query.StartTime != nil && metric.Timestamp.Before(*query.StartTime) {
		return false
	}
	if query.EndTime != nil && metric.Timestamp.After(*query.EndTime) {
		return false
	}
	return true
}

func (s *mockStorage) GetMetricsByGPU(ctx context.Context, uuid string, startTime, endTime *time.Time) ([]*models.GPUMetric, error) {
	return s.GetTelemetry(ctx, &models.TelemetryQuery{UUID: uuid, StartTime: startTime, EndTime: endTime})
}

func (s *mockStorage) Cleanup(ctx context.Context, retentionPeriod time.Duration) (int, error) {
	return 0, nil
}

func (s *mockStorage) Stats() storage.StorageStats {
	return storage.StorageStats{}
}

func (s *mockStorage) Close() error {
	return nil
}

func setupTestRouter(store storage.ReadStorage) *mux.Router {
	router := mux.NewRouter()
	handler := NewHandler(store, 100, 1000)

	api := router.PathPrefix("/api/v1").Subrouter()
	api.HandleFunc("/gpus", handler.ListGPUs).Methods(http.MethodGet)
	api.HandleFunc("/gpus/{id}/telemetry", handler.GetGPUTelemetry).Methods(http.MethodGet)

	return router
}

func seedTestData(t *testing.T, store *mockStorage) {
	ctx := context.Background()

	// Add multiple GPUs with metrics
	gpus := []struct {
		uuid     string
		hostname string
		gpuID    int
	}{
		{"GPU-12345-AAAA", "host-001", 0},
		{"GPU-12345-BBBB", "host-001", 1},
		{"GPU-67890-CCCC", "host-002", 0},
	}

	for _, gpu := range gpus {
		for i := 0; i < 5; i++ {
			metric := &models.GPUMetric{
				Timestamp:  time.Now().Add(time.Duration(i) * time.Minute),
				MetricName: "DCGM_FI_DEV_GPU_UTIL",
				GPUID:      gpu.gpuID,
				Device:     "nvidia0",
				UUID:       gpu.uuid,
				ModelName:  "NVIDIA H100 80GB HBM3",
				Hostname:   gpu.hostname,
				Value:      float64(i * 20),
			}
			require.NoError(t, store.Store(ctx, metric))
		}
	}
}

func TestListGPUs(t *testing.T) {
	store := newMockStorage()
	defer store.Close()
	seedTestData(t, store)

	router := setupTestRouter(store)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response GPUListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 3, response.Count)
	assert.Len(t, response.Data, 3)
}

func TestListGPUsEmpty(t *testing.T) {
	store := newMockStorage()
	defer store.Close()

	router := setupTestRouter(store)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response GPUListResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Count)
	assert.Empty(t, response.Data)
}

func TestGetGPUTelemetry(t *testing.T) {
	store := newMockStorage()
	defer store.Close()
	seedTestData(t, store)

	router := setupTestRouter(store)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/GPU-12345-AAAA/telemetry", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response TelemetryResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 5, response.Count)
	assert.Len(t, response.Data, 5)
}

func TestGetGPUTelemetryWithLimit(t *testing.T) {
	store := newMockStorage()
	defer store.Close()
	seedTestData(t, store)

	router := setupTestRouter(store)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/GPU-12345-AAAA/telemetry?limit=2", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response TelemetryResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 2, response.Count)
}

func TestGetGPUTelemetryWithTimeFilter(t *testing.T) {
	store := newMockStorage()
	defer store.Close()

	ctx := context.Background()
	baseTime := time.Now().Truncate(time.Second)

	// Store metrics at specific times
	for i := 0; i < 5; i++ {
		metric := &models.GPUMetric{
			Timestamp:  baseTime.Add(time.Duration(i) * time.Hour),
			MetricName: "DCGM_FI_DEV_GPU_UTIL",
			UUID:       "GPU-12345",
			Value:      float64(i * 20),
		}
		require.NoError(t, store.Store(ctx, metric))
	}

	router := setupTestRouter(store)

	// Filter: hours 1-3
	startTime := url.QueryEscape(baseTime.Add(time.Hour).Format(time.RFC3339))
	endTime := url.QueryEscape(baseTime.Add(3 * time.Hour).Format(time.RFC3339))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/GPU-12345/telemetry?start_time="+startTime+"&end_time="+endTime, nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response TelemetryResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 3, response.Count) // Hours 1, 2, 3
}

func TestGetGPUTelemetryInvalidTimeFormat(t *testing.T) {
	store := newMockStorage()
	defer store.Close()
	seedTestData(t, store)

	router := setupTestRouter(store)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/GPU-12345-AAAA/telemetry?start_time=invalid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response ErrorResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "bad_request", response.Error)
}

func TestGetGPUTelemetryEmpty(t *testing.T) {
	store := newMockStorage()
	defer store.Close()

	router := setupTestRouter(store)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/non-existent/telemetry", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response TelemetryResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, 0, response.Count)
}

func TestPagination(t *testing.T) {
	store := newMockStorage()
	defer store.Close()

	ctx := context.Background()

	// Store 10 metrics
	for i := 0; i < 10; i++ {
		metric := &models.GPUMetric{
			Timestamp:  time.Now().Add(time.Duration(i) * time.Minute),
			MetricName: "DCGM_FI_DEV_GPU_UTIL",
			UUID:       "GPU-12345",
			Value:      float64(i),
		}
		require.NoError(t, store.Store(ctx, metric))
	}

	router := setupTestRouter(store)

	// Page 1
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/GPU-12345/telemetry?limit=3&offset=0", nil)
	router.ServeHTTP(w, req)

	var response TelemetryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 3, response.Count)

	// Page 2
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/gpus/GPU-12345/telemetry?limit=3&offset=3", nil)
	router.ServeHTTP(w, req)

	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, 3, response.Count)
}

func TestInvalidLimit(t *testing.T) {
	store := newMockStorage()
	defer store.Close()
	seedTestData(t, store)

	router := setupTestRouter(store)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/GPU-12345-AAAA/telemetry?limit=-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestInvalidOffset(t *testing.T) {
	store := newMockStorage()
	defer store.Close()
	seedTestData(t, store)

	router := setupTestRouter(store)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/gpus/GPU-12345-AAAA/telemetry?offset=-1", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
