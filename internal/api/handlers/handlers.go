// Package handlers provides HTTP handlers for the REST API.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/cisco/gpu-telemetry-pipeline/internal/storage"
	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
)

// Handler handles GPU telemetry API requests.
type Handler struct {
	store        storage.ReadStorage
	defaultLimit int
	maxLimit     int
}

// NewHandler creates a new handler with read-only storage.
func NewHandler(store storage.ReadStorage, defaultLimit, maxLimit int) *Handler {
	return &Handler{
		store:        store,
		defaultLimit: defaultLimit,
		maxLimit:     maxLimit,
	}
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error" example:"internal_error"`
	Message string `json:"message,omitempty" example:"Failed to fetch data"`
}

// GPUListResponse represents the response for listing GPUs.
type GPUListResponse struct {
	Data  []string `json:"data"`
	Count int      `json:"count" example:"256"`
}

// TelemetryResponse represents the response for telemetry queries.
type TelemetryResponse struct {
	Data  []*models.GPUMetric `json:"data"`
	Count int                 `json:"count" example:"100"`
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError writes an error response.
func writeError(w http.ResponseWriter, status int, err string, message string) {
	writeJSON(w, status, ErrorResponse{
		Error:   err,
		Message: message,
	})
}

// ListGPUs godoc
// @Summary      List all GPUs
// @Description  Returns a list of all GPUs for which telemetry data is available
// @Tags         gpus
// @Produce      json
// @Success      200  {object}  GPUListResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/gpus [get]
func (h *Handler) ListGPUs(w http.ResponseWriter, r *http.Request) {
	gpus, err := h.store.GetGPUs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, GPUListResponse{
		Data:  gpus,
		Count: len(gpus),
	})
}

// GetGPUTelemetry godoc
// @Summary      Get GPU telemetry
// @Description  Returns all telemetry entries for a specific GPU, ordered by time
// @Tags         gpus
// @Produce      json
// @Param        id          path      string  true   "GPU UUID"
// @Param        start_time  query     string  false  "Start time filter (RFC3339)"  example(2024-01-01T00:00:00Z)
// @Param        end_time    query     string  false  "End time filter (RFC3339)"    example(2024-01-02T00:00:00Z)
// @Param        limit       query     int     false  "Maximum results"              default(100)
// @Param        offset      query     int     false  "Offset for pagination"        default(0)
// @Success      200  {object}  TelemetryResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/gpus/{id}/telemetry [get]
// @Param        metric_name query string false "Metric name filter (e.g., DCGM_FI_DEV_GPU_UTIL)"
// @Param        hostname    query string false "Hostname filter"
// @Param        gpu_id      query int    false "GPU ID filter"
func (h *Handler) GetGPUTelemetry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gpuID := vars["id"]
	if gpuID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "GPU ID is required")
		return
	}
	query := &models.TelemetryQuery{
		UUID:   gpuID,
		Limit:  h.defaultLimit,
		Offset: 0,
	}
	// Parse start_time
	if startTimeStr := r.URL.Query().Get("start_time"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid start_time format. Use RFC3339 (e.g., 2024-01-01T00:00:00Z)")
			return
		}
		query.StartTime = &startTime
	}
	// Parse end_time
	if endTimeStr := r.URL.Query().Get("end_time"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid end_time format. Use RFC3339 (e.g., 2024-01-02T00:00:00Z)")
			return
		}
		query.EndTime = &endTime
	}
	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid limit parameter")
			return
		}
		if limit > h.maxLimit {
			limit = h.maxLimit
		}
		query.Limit = limit
	}
	// Parse offset
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid offset parameter")
			return
		}
		query.Offset = offset
	}
	// Parse metric_name
	if metricName := r.URL.Query().Get("metric_name"); metricName != "" {
		query.MetricName = metricName
	}
	// Parse hostname
	if hostname := r.URL.Query().Get("hostname"); hostname != "" {
		query.Hostname = hostname
	}
	// Parse gpu_id
	if gpuIDStr := r.URL.Query().Get("gpu_id"); gpuIDStr != "" {
		gpuIDVal, err := strconv.Atoi(gpuIDStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid gpu_id parameter")
			return
		}
		query.GPUID = &gpuIDVal
	}
	metrics, err := h.store.GetTelemetry(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, TelemetryResponse{
		Data:  metrics,
		Count: len(metrics),
	})
}

// MetricNamesResponse represents the response for available metric names.
type MetricNamesResponse struct {
	Data  []string `json:"data"`
	Count int      `json:"count"`
}

// ListMetricNames godoc
// @Summary      List available metric names for a GPU
// @Description  Returns a list of all metric names for a specific GPU
// @Tags         gpus
// @Produce      json
// @Param        id   path  string  true  "GPU UUID"
// @Success      200  {object}  MetricNamesResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/gpus/{id}/metrics [get]
func (h *Handler) ListMetricNames(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gpuID := vars["id"]
	if gpuID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "GPU ID is required")
		return
	}
	// Query all telemetry for this GPU (limit 1000)
	query := &models.TelemetryQuery{
		UUID:  gpuID,
		Limit: 1000,
	}
	metrics, err := h.store.GetTelemetry(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	metricSet := make(map[string]struct{})
	for _, m := range metrics {
		metricSet[m.MetricName] = struct{}{}
	}
	names := make([]string, 0, len(metricSet))
	for name := range metricSet {
		names = append(names, name)
	}
	writeJSON(w, http.StatusOK, MetricNamesResponse{
		Data:  names,
		Count: len(names),
	})
}

// GPUInfoResponse represents the response for GPU information.
type GPUInfoResponse struct {
	UUID      string    `json:"uuid"`
	GPUID     int       `json:"gpu_id"`
	Device    string    `json:"device"`
	ModelName string    `json:"model_name"`
	Hostname  string    `json:"hostname"`
	FirstSeen time.Time `json:"first_seen,omitempty"`
	LastSeen  time.Time `json:"last_seen,omitempty"`
}

// GetGPUInfo godoc
// @Summary      Get GPU information
// @Description  Returns detailed information about a specific GPU
// @Tags         gpus
// @Produce      json
// @Param        id   path  string  true  "GPU UUID"
// @Success      200  {object}  GPUInfoResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/gpus/{id} [get]
func (h *Handler) GetGPUInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gpuID := vars["id"]
	if gpuID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "GPU ID is required")
		return
	}

	// Get telemetry to extract GPU info (get latest first)
	query := &models.TelemetryQuery{
		UUID:  gpuID,
		Limit: 1,
	}
	metrics, err := h.store.GetTelemetry(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if len(metrics) == 0 {
		writeError(w, http.StatusNotFound, "not_found", "GPU not found")
		return
	}

	// Get a larger sample to find first and last timestamps
	allQuery := &models.TelemetryQuery{
		UUID:  gpuID,
		Limit: 1000, // Get more to find oldest
	}
	allMetrics, _ := h.store.GetTelemetry(r.Context(), allQuery)

	var firstSeen, lastSeen time.Time
	if len(allMetrics) > 0 {
		// Metrics are sorted descending, so first is newest, last is oldest
		lastSeen = allMetrics[0].Timestamp
		firstSeen = allMetrics[len(allMetrics)-1].Timestamp
	} else if len(metrics) > 0 {
		// Fallback to single metric
		lastSeen = metrics[0].Timestamp
		firstSeen = metrics[0].Timestamp
	}

	info := GPUInfoResponse{
		UUID:      metrics[0].UUID,
		GPUID:     metrics[0].GPUID,
		Device:    metrics[0].Device,
		ModelName: metrics[0].ModelName,
		Hostname:  metrics[0].Hostname,
		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
	}

	writeJSON(w, http.StatusOK, info)
}

// StatsResponse represents system statistics.
type StatsResponse struct {
	TotalGPUs    int       `json:"total_gpus"`
	TotalMetrics int64     `json:"total_metrics,omitempty"`
	OldestMetric time.Time `json:"oldest_metric,omitempty"`
	NewestMetric time.Time `json:"newest_metric,omitempty"`
}

// GetStats godoc
// @Summary      Get system statistics
// @Description  Returns overall system statistics about GPUs and telemetry data
// @Tags         system
// @Produce      json
// @Success      200  {object}  StatsResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/stats [get]
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	gpus, err := h.store.GetGPUs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	// Try to get storage stats if available
	stats := StatsResponse{
		TotalGPUs: len(gpus),
	}

	// If store implements Stats() method, get detailed stats
	if statsStore, ok := h.store.(interface{ Stats() storage.StorageStats }); ok {
		storageStats := statsStore.Stats()
		stats.TotalMetrics = storageStats.TotalMetrics
		stats.OldestMetric = storageStats.OldestMetric
		stats.NewestMetric = storageStats.NewestMetric
	}

	writeJSON(w, http.StatusOK, stats)
}

// AllMetricsResponse represents the response for all available metric types.
type AllMetricsResponse struct {
	Data  []string `json:"data"`
	Count int      `json:"count"`
}

// ListAllMetrics godoc
// @Summary      List all available metric types
// @Description  Returns a list of all metric types available across all GPUs
// @Tags         system
// @Produce      json
// @Success      200  {object}  AllMetricsResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/metrics [get]
func (h *Handler) ListAllMetrics(w http.ResponseWriter, r *http.Request) {
	gpus, err := h.store.GetGPUs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	metricSet := make(map[string]struct{})
	// Sample from each GPU to get all metric types
	for _, gpuID := range gpus {
		query := &models.TelemetryQuery{
			UUID:  gpuID,
			Limit: 100,
		}
		metrics, err := h.store.GetTelemetry(r.Context(), query)
		if err != nil {
			continue
		}
		for _, m := range metrics {
			metricSet[m.MetricName] = struct{}{}
		}
	}

	names := make([]string, 0, len(metricSet))
	for name := range metricSet {
		names = append(names, name)
	}

	writeJSON(w, http.StatusOK, AllMetricsResponse{
		Data:  names,
		Count: len(names),
	})
}

// ExportGPUTelemetry godoc
// @Summary      Export GPU telemetry data
// @Description  Exports telemetry data for a specific GPU in CSV or JSON format
// @Tags         gpus
// @Produce      plain
// @Produce      json
// @Param        id          path      string  true   "GPU UUID"
// @Param        format      query     string  false  "Output format (csv or json)"    default(json)  enum(csv,json)
// @Param        start_time  query     string  false  "Start time filter (RFC3339)"  example(2024-01-01T00:00:00Z)
// @Param        end_time    query     string  false  "End time filter (RFC3339)"    example(2024-01-02T00:00:00Z)
// @Param        limit       query     int     false  "Maximum results"              default(10000)
// @Param        offset      query     int     false  "Offset for pagination"        default(0)
// @Success      200  {string}    string  "Telemetry data in specified format"
// @Failure      400  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /api/v1/gpus/{id}/telemetry/export [get]
func (h *Handler) ExportGPUTelemetry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gpuID := vars["id"]
	if gpuID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "GPU ID is required")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json" // Default format
	}
	if format != "csv" && format != "json" {
		writeError(w, http.StatusBadRequest, "bad_request", "Invalid format. Must be 'csv' or 'json'")
		return
	}

	query := &models.TelemetryQuery{
		UUID:   gpuID,
		Limit:  10000, // Default for export
		Offset: 0,
	}

	// Parse start_time
	if startTimeStr := r.URL.Query().Get("start_time"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid start_time format. Use RFC3339 (e.g., 2024-01-01T00:00:00Z)")
			return
		}
		query.StartTime = &startTime
	}
	// Parse end_time
	if endTimeStr := r.URL.Query().Get("end_time"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid end_time format. Use RFC3339 (e.g., 2024-01-02T00:00:00Z)")
			return
		}
		query.EndTime = &endTime
	}
	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid limit parameter")
			return
		}
		query.Limit = limit
	}
	// Parse offset
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "bad_request", "Invalid offset parameter")
			return
		}
		query.Offset = offset
	}

	metrics, err := h.store.GetTelemetry(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"telemetry-%s.csv\"", gpuID))

		// Write CSV header
		fmt.Fprintf(w, "Timestamp,MetricName,GPUID,Device,UUID,ModelName,Hostname,Container,Pod,Namespace,Value\n")
		// Write CSV data
		for _, m := range metrics {
			fmt.Fprintf(w, "%s,%s,%d,%s,%s,%s,%s,%s,%s,%s,%.2f\n",
				m.Timestamp.Format(time.RFC3339),
				m.MetricName,
				m.GPUID,
				m.Device,
				m.UUID,
				m.ModelName,
				m.Hostname,
				m.Container,
				m.Pod,
				m.Namespace,
				m.Value,
			)
		}
	} else { // Default to JSON
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, http.StatusOK, TelemetryResponse{
			Data:  metrics,
			Count: len(metrics),
		})
	}
}
