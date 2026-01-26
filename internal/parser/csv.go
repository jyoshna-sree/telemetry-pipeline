// Package parser provides CSV parsing functionality for telemetry data.
package parser

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
)

// CSVParser parses telemetry data from CSV files.
type CSVParser struct {
	filePath  string
	file      *os.File
	reader    *csv.Reader
	headers   []string
	headerMap map[string]int
}

// Expected CSV columns (case-insensitive)
var expectedColumns = []string{
	"timestamp",
	"metric_name",
	"gpu_id",
	"device",
	"uuid",
	"modelname",
	"hostname",
	"container",
	"pod",
	"namespace",
	"value",
	"labels_raw",
}

// NewCSVParser creates a new CSV parser for the given file.
func NewCSVParser(filePath string) (*CSVParser, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open CSV file: %w", err)
	}

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1 // Allow variable fields
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	// Read header row
	headers, err := reader.Read()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to read CSV headers: %w", err)
	}

	// Build header map (case-insensitive)
	headerMap := make(map[string]int)
	for i, h := range headers {
		headerMap[strings.ToLower(strings.TrimSpace(h))] = i
	}

	return &CSVParser{
		filePath:  filePath,
		file:      file,
		reader:    reader,
		headers:   headers,
		headerMap: headerMap,
	}, nil
}

// Close closes the parser and underlying file.
func (p *CSVParser) Close() error {
	if p.file != nil {
		return p.file.Close()
	}
	return nil
}

// Reset resets the parser to the beginning of the file.
func (p *CSVParser) Reset() error {
	if p.file != nil {
		p.file.Close()
	}

	file, err := os.Open(p.filePath)
	if err != nil {
		return fmt.Errorf("failed to reopen CSV file: %w", err)
	}

	p.file = file
	p.reader = csv.NewReader(file)
	p.reader.FieldsPerRecord = -1
	p.reader.LazyQuotes = true
	p.reader.TrimLeadingSpace = true

	// Skip header row
	if _, err := p.reader.Read(); err != nil {
		return fmt.Errorf("failed to skip header row: %w", err)
	}

	return nil
}

// ReadNext reads and parses the next row from the CSV.
// Returns nil when EOF is reached.
func (p *CSVParser) ReadNext() (*models.GPUMetric, error) {
	record, err := p.reader.Read()
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV row: %w", err)
	}

	return p.parseRecord(record)
}

// ReadBatch reads up to n records from the CSV.
func (p *CSVParser) ReadBatch(n int) ([]*models.GPUMetric, error) {
	metrics := make([]*models.GPUMetric, 0, n)

	for i := 0; i < n; i++ {
		metric, err := p.ReadNext()
		if err != nil {
			return metrics, err
		}
		if metric == nil {
			break // EOF
		}
		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// ReadAll reads all remaining records from the CSV.
func (p *CSVParser) ReadAll() ([]*models.GPUMetric, error) {
	var metrics []*models.GPUMetric

	for {
		metric, err := p.ReadNext()
		if err != nil {
			return metrics, err
		}
		if metric == nil {
			break
		}
		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// parseRecord converts a CSV record to a GPUMetric.
func (p *CSVParser) parseRecord(record []string) (*models.GPUMetric, error) {
	metric := &models.GPUMetric{
		// Use current time as the processing timestamp
		Timestamp: time.Now(),
		Labels:    make(map[string]string),
	}

	// Helper to get field value safely
	getField := func(name string) string {
		if idx, ok := p.headerMap[strings.ToLower(name)]; ok && idx < len(record) {
			return strings.TrimSpace(record[idx])
		}
		return ""
	}

	// Parse fields
	metric.MetricName = getField("metric_name")
	metric.Device = getField("device")
	metric.UUID = getField("uuid")
	metric.ModelName = getField("modelname")
	metric.Hostname = getField("hostname")
	metric.Container = getField("container")
	metric.Pod = getField("pod")
	metric.Namespace = getField("namespace")

	// Parse gpu_id
	if gpuIDStr := getField("gpu_id"); gpuIDStr != "" {
		if gpuID, err := strconv.Atoi(gpuIDStr); err == nil {
			metric.GPUID = gpuID
		}
	}

	// Parse value
	if valueStr := getField("value"); valueStr != "" {
		if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
			metric.Value = value
		}
	}

	// Parse labels_raw (Prometheus-style labels)
	if labelsRaw := getField("labels_raw"); labelsRaw != "" {
		metric.Labels = parseLabels(labelsRaw)
	}

	// Validate required fields
	if metric.UUID == "" {
		return nil, fmt.Errorf("missing required field: uuid")
	}
	if metric.MetricName == "" {
		return nil, fmt.Errorf("missing required field: metric_name")
	}

	return metric, nil
}

// parseLabels parses Prometheus-style labels from a raw string.
// Format: key1="value1",key2="value2"
func parseLabels(raw string) map[string]string {
	labels := make(map[string]string)

	// Handle DCGM label format: key=value,key=value or key="value",key="value"
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if idx := strings.Index(part, "="); idx > 0 {
			key := strings.TrimSpace(part[:idx])
			value := strings.TrimSpace(part[idx+1:])
			// Remove quotes if present
			value = strings.Trim(value, "\"'")
			if key != "" {
				labels[key] = value
			}
		}
	}

	return labels
}

// CountRecords counts the total number of data records in the CSV.
func CountRecords(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	// Skip header
	if _, err := reader.Read(); err != nil {
		return 0, err
	}

	count := 0
	for {
		_, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}

// ValidateCSV checks if the CSV file has the expected format.
func ValidateCSV(filePath string) error {
	parser, err := NewCSVParser(filePath)
	if err != nil {
		return err
	}
	defer parser.Close()

	// Check for required columns
	required := []string{"uuid", "metric_name", "value"}
	for _, col := range required {
		if _, ok := parser.headerMap[col]; !ok {
			return fmt.Errorf("missing required column: %s", col)
		}
	}

	// Try to read first record
	metric, err := parser.ReadNext()
	if err != nil {
		return fmt.Errorf("failed to parse first record: %w", err)
	}
	if metric == nil {
		return fmt.Errorf("CSV file is empty")
	}

	return nil
}
