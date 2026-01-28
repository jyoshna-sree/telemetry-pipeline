// Package api provides the REST API router.
package api

import (
	"net/http"

	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"

	"github.com/cisco/gpu-telemetry-pipeline/internal/api/handlers"
	"github.com/cisco/gpu-telemetry-pipeline/internal/storage"
)

// RouterConfig configures the API router.
type RouterConfig struct {
	// DefaultLimit is the default pagination limit
	DefaultLimit int

	// MaxLimit is the maximum pagination limit
	MaxLimit int
}

// DefaultRouterConfig returns a router config with sensible defaults.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		DefaultLimit: 100,
		MaxLimit:     1000,
	}
}

// NewRouter creates a new mux router with all routes configured.
func NewRouter(store storage.ReadStorage, config RouterConfig) *mux.Router {
	router := mux.NewRouter()

	// Create handler
	handler := handlers.NewHandler(store, config.DefaultLimit, config.MaxLimit)

	// Health check endpoints for Kubernetes probes
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy"}`))
	}).Methods(http.MethodGet)

	router.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
	}).Methods(http.MethodGet)

	// Swagger UI
	router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	// API v1 routes
	api := router.PathPrefix("/api/v1").Subrouter()

	// GET /api/v1/gpus - List all GPUs
	api.HandleFunc("/gpus", handler.ListGPUs).Methods(http.MethodGet)

	// GET /api/v1/gpus/{id} - Get GPU information
	api.HandleFunc("/gpus/{id}", handler.GetGPUInfo).Methods(http.MethodGet)

	// GET /api/v1/gpus/{id}/telemetry - Get telemetry for a GPU
	api.HandleFunc("/gpus/{id}/telemetry", handler.GetGPUTelemetry).Methods(http.MethodGet)

	// GET /api/v1/gpus/{id}/metrics - List available metric names for a GPU
	api.HandleFunc("/gpus/{id}/metrics", handler.ListMetricNames).Methods(http.MethodGet)

	// GET /api/v1/metrics - List all available metric types
	api.HandleFunc("/metrics", handler.ListAllMetrics).Methods(http.MethodGet)

	// GET /api/v1/stats - Get system statistics
	api.HandleFunc("/stats", handler.GetStats).Methods(http.MethodGet)

	// GET /api/v1/gpus/{id}/telemetry/export - Export telemetry data for a GPU as CSV or JSON
	api.HandleFunc("/gpus/{id}/telemetry/export", handler.ExportGPUTelemetry).Methods(http.MethodGet)

	return router
}
