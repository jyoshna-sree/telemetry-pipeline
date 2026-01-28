// Package storage provides telemetry data storage backends.
package storage

import (
	"context"
	"fmt"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	"github.com/influxdata/influxdb-client-go/v2/api/query"

	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
)

// InfluxDBConfig holds InfluxDB connection settings.
type InfluxDBConfig struct {
	URL    string `json:"url"`    // e.g., "http://localhost:8086"
	Token  string `json:"token"`  // API token
	Org    string `json:"org"`    // Organization name
	Bucket string `json:"bucket"` // Bucket name
}

// DefaultInfluxDBConfig returns sensible defaults from environment variables.
func DefaultInfluxDBConfig() InfluxDBConfig {
	return InfluxDBConfig{
		URL:    getEnv("INFLUXDB_URL", "http://localhost:8086"),
		Token:  os.Getenv("INFLUXDB_TOKEN"),
		Org:    getEnv("INFLUXDB_ORG", "cisco"),
		Bucket: getEnv("INFLUXDB_BUCKET", "gpu_telemetry"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// InfluxDBStorage implements ReadStorage for read-only InfluxDB access.
// Used by the API to query telemetry data.
type InfluxDBStorage struct {
	client   influxdb2.Client
	queryAPI api.QueryAPI
	config   InfluxDBConfig
}

// NewInfluxDBStorage creates a new read-only InfluxDB storage backend.
func NewInfluxDBStorage(config InfluxDBConfig) (*InfluxDBStorage, error) {
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

	return &InfluxDBStorage{
		client:   client,
		queryAPI: client.QueryAPI(config.Org),
		config:   config,
	}, nil
}

// GetGPUs returns all known GPU IDs by querying distinct UUIDs from InfluxDB.
func (s *InfluxDBStorage) GetGPUs(ctx context.Context) ([]string, error) {
	// Query to get distinct GPU UUIDs
	fluxQuery := fmt.Sprintf(`
		from(bucket: "%s")
			|> range(start: -24h)
			|> filter(fn: (r) => r._field == "value")
			|> group(columns: ["uuid"])
			|> last()
	`, s.config.Bucket)

	result, err := s.queryAPI.Query(ctx, fluxQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query GPUs: %w", err)
	}
	defer result.Close()

	gpuIDs := make(map[string]struct{})
	for result.Next() {
		record := result.Record()
		values := record.Values()

		uuid, _ := values["uuid"].(string)
		if uuid == "" {
			continue
		}

		gpuIDs[uuid] = struct{}{}
	}

	if result.Err() != nil {
		return nil, fmt.Errorf("query error: %w", result.Err())
	}

	gpus := make([]string, 0, len(gpuIDs))
	for uuid := range gpuIDs {
		gpus = append(gpus, uuid)
	}

	return gpus, nil
}

// GetTelemetry returns telemetry matching the query.
func (s *InfluxDBStorage) GetTelemetry(ctx context.Context, query *models.TelemetryQuery) ([]*models.GPUMetric, error) {
	start := time.Now().Add(-24 * time.Hour)
	stop := time.Now()

	if query.StartTime != nil {
		start = *query.StartTime
	}
	if query.EndTime != nil {
		stop = *query.EndTime
	}

	// Build Flux query
	fluxQuery := fmt.Sprintf(`
		from(bucket: "%s")
			|> range(start: %s, stop: %s)
	`, s.config.Bucket,
		start.Format(time.RFC3339),
		stop.Format(time.RFC3339))

	// Add metric name filter if specified
	if query.MetricName != "" {
		fluxQuery += fmt.Sprintf(`|> filter(fn: (r) => r._measurement == "%s")`, query.MetricName)
	}

	// Add UUID filter if specified
	if query.UUID != "" {
		fluxQuery += fmt.Sprintf(`|> filter(fn: (r) => r.uuid == "%s")`, query.UUID)
	}

	// Add hostname filter if specified
	if query.Hostname != "" {
		fluxQuery += fmt.Sprintf(`|> filter(fn: (r) => r.hostname == "%s")`, query.Hostname)
	}

	// Add GPU ID filter if specified
	if query.GPUID != nil {
		fluxQuery += fmt.Sprintf(`|> filter(fn: (r) => r.gpu_id == "%d")`, *query.GPUID)
	}

	// Sort by time descending
	fluxQuery += `|> sort(columns: ["_time"], desc: true)`

	// Apply offset and limit
	// Note: In Flux, we need to handle offset by skipping records
	// Since we want the most recent records first (desc order), we:
	// 1. Sort descending
	// 2. Skip offset records
	// 3. Take limit records
	if query.Offset > 0 {
		fluxQuery += fmt.Sprintf(`|> skip(n: %d)`, query.Offset)
	}
	if query.Limit > 0 {
		fluxQuery += fmt.Sprintf(`|> limit(n: %d)`, query.Limit)
	}

	result, err := s.queryAPI.Query(ctx, fluxQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query InfluxDB: %w", err)
	}
	defer result.Close()

	metrics := make([]*models.GPUMetric, 0)
	for result.Next() {
		record := result.Record()
		metric := s.recordToMetric(record)
		if metric != nil {
			metrics = append(metrics, metric)
		}
	}

	if result.Err() != nil {
		return nil, fmt.Errorf("query error: %w", result.Err())
	}

	return metrics, nil
}

// recordToMetric converts an InfluxDB FluxRecord to a GPUMetric.
func (s *InfluxDBStorage) recordToMetric(record *query.FluxRecord) *models.GPUMetric {
	values := record.Values()

	metric := &models.GPUMetric{
		Timestamp:  record.Time(),
		MetricName: record.Measurement(),
	}

	// Extract value
	if v, ok := record.Value().(float64); ok {
		metric.Value = v
	}

	// Extract tags
	if v, ok := values["uuid"].(string); ok {
		metric.UUID = v
	}
	if v, ok := values["hostname"].(string); ok {
		metric.Hostname = v
	}
	if v, ok := values["device"].(string); ok {
		metric.Device = v
	}
	if v, ok := values["model"].(string); ok {
		metric.ModelName = v
	}
	if v, ok := values["container"].(string); ok {
		metric.Container = v
	}
	if v, ok := values["pod"].(string); ok {
		metric.Pod = v
	}
	if v, ok := values["namespace"].(string); ok {
		metric.Namespace = v
	}
	if v, ok := values["gpu_id"].(string); ok {
		fmt.Sscanf(v, "%d", &metric.GPUID)
	}

	return metric
}

// Close closes the InfluxDB client.
func (s *InfluxDBStorage) Close() error {
	s.client.Close()
	return nil
}
