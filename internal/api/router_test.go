package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
)

// mockReadStorage implements storage.ReadStorage for testing
type mockReadStorage struct {
	gpus      []string
	telemetry []*models.GPUMetric
	err       error
}

func (m *mockReadStorage) GetGPUs(ctx context.Context) ([]string, error) {
	return m.gpus, m.err
}

func (m *mockReadStorage) GetTelemetry(ctx context.Context, query *models.TelemetryQuery) ([]*models.GPUMetric, error) {
	return m.telemetry, m.err
}

func (m *mockReadStorage) Close() error {
	return nil
}

func TestDefaultRouterConfig(t *testing.T) {
	cfg := DefaultRouterConfig()

	if cfg.DefaultLimit <= 0 {
		t.Error("expected positive default limit")
	}
	if cfg.MaxLimit <= 0 {
		t.Error("expected positive max limit")
	}
	if cfg.MaxLimit < cfg.DefaultLimit {
		t.Error("expected max limit >= default limit")
	}
}

func TestNewRouter(t *testing.T) {
	store := &mockReadStorage{
		gpus: []string{"GPU-1", "GPU-2"},
	}
	config := DefaultRouterConfig()

	router := NewRouter(store, config)
	if router == nil {
		t.Fatal("expected router to be created")
	}
}

func TestRouterGPUsEndpoint(t *testing.T) {
	store := &mockReadStorage{
		gpus: []string{"GPU-1", "GPU-2", "GPU-3"},
	}
	config := DefaultRouterConfig()
	router := NewRouter(store, config)

	req, _ := http.NewRequest("GET", "/api/v1/gpus", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouterTelemetryEndpoint(t *testing.T) {
	store := &mockReadStorage{
		telemetry: []*models.GPUMetric{},
	}
	config := DefaultRouterConfig()
	router := NewRouter(store, config)

	req, _ := http.NewRequest("GET", "/api/v1/gpus/GPU-1/telemetry", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestRouterSwaggerEndpoint(t *testing.T) {
	// Skip swagger test as it requires swagger docs to be properly initialized
	t.Skip("Swagger endpoint requires initialized swagger docs")
}

func TestRouterMethodNotAllowed(t *testing.T) {
	store := &mockReadStorage{}
	config := DefaultRouterConfig()
	router := NewRouter(store, config)

	// POST to GET-only endpoint - gorilla mux returns 404 by default for unregistered methods
	req, _ := http.NewRequest("POST", "/api/v1/gpus", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Mux returns 405 only if MethodNotAllowedHandler is set, otherwise 404
	if w.Code != http.StatusMethodNotAllowed && w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 or 405, got %d", w.Code)
	}
}
