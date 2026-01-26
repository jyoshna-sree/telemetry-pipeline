// Package handlers provides HTTP handlers for the REST API.
package handlers

import (
	"encoding/json"
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
func (h *Handler) GetGPUTelemetry(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	gpuID := vars["id"]

	if gpuID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "GPU ID is required")
		return
	}

	// Build query
	query := &models.TelemetryQuery{
		UUID:   gpuID,
		Limit:  h.defaultLimit,
		Offset: 0,
	}

	// Parse start_time
	if startTimeStr := r.URL.Query().Get("start_time"); startTimeStr != "" {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request",
				"Invalid start_time format. Use RFC3339 (e.g., 2024-01-01T00:00:00Z)")
			return
		}
		query.StartTime = &startTime
	}

	// Parse end_time
	if endTimeStr := r.URL.Query().Get("end_time"); endTimeStr != "" {
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request",
				"Invalid end_time format. Use RFC3339 (e.g., 2024-01-02T00:00:00Z)")
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

	// Query telemetry
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
