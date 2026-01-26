package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleCSV = `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-5fd4f087-86f3-1234-5678-abcdef123456,NVIDIA H100 80GB HBM3,mtv5-dgx1-hgpu-001,,,,100,DCGM_FI_DRIVER_VERSION=535.129.03
2025-07-18T20:42:34Z,DCGM_FI_DEV_MEM_COPY_UTIL,0,nvidia0,GPU-5fd4f087-86f3-1234-5678-abcdef123456,NVIDIA H100 80GB HBM3,mtv5-dgx1-hgpu-001,,,,45,DCGM_FI_DRIVER_VERSION=535.129.03
2025-07-18T20:42:34Z,DCGM_FI_DEV_SM_CLOCK,1,nvidia1,GPU-6ae5f188-97g4-2345-6789-bcdefg234567,NVIDIA H100 80GB HBM3,mtv5-dgx1-hgpu-001,,,,1980,DCGM_FI_DRIVER_VERSION=535.129.03
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-7bf6g289-08h5-3456-7890-cdefgh345678,NVIDIA H100 80GB HBM3,mtv5-dgx1-hgpu-002,,,,85.5,DCGM_FI_DRIVER_VERSION=535.129.03
`

func createTestCSV(t *testing.T, content string) string {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test.csv")
	err := os.WriteFile(csvPath, []byte(content), 0644)
	require.NoError(t, err)
	return csvPath
}

func TestNewCSVParser(t *testing.T) {
	csvPath := createTestCSV(t, sampleCSV)

	parser, err := NewCSVParser(csvPath)
	require.NoError(t, err)
	require.NotNil(t, parser)
	defer parser.Close()

	// Verify headers were parsed
	assert.Contains(t, parser.headerMap, "timestamp")
	assert.Contains(t, parser.headerMap, "metric_name")
	assert.Contains(t, parser.headerMap, "uuid")
	assert.Contains(t, parser.headerMap, "value")
}

func TestNewCSVParserFileNotFound(t *testing.T) {
	parser, err := NewCSVParser("/non/existent/file.csv")
	assert.Error(t, err)
	assert.Nil(t, parser)
}

func TestReadNext(t *testing.T) {
	csvPath := createTestCSV(t, sampleCSV)

	parser, err := NewCSVParser(csvPath)
	require.NoError(t, err)
	defer parser.Close()

	// Read first metric
	metric, err := parser.ReadNext()
	require.NoError(t, err)
	require.NotNil(t, metric)

	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", metric.MetricName)
	assert.Equal(t, 0, metric.GPUID)
	assert.Equal(t, "nvidia0", metric.Device)
	assert.Equal(t, "GPU-5fd4f087-86f3-1234-5678-abcdef123456", metric.UUID)
	assert.Equal(t, "NVIDIA H100 80GB HBM3", metric.ModelName)
	assert.Equal(t, "mtv5-dgx1-hgpu-001", metric.Hostname)
	assert.Equal(t, 100.0, metric.Value)
	assert.NotZero(t, metric.Timestamp)
}

func TestReadBatch(t *testing.T) {
	csvPath := createTestCSV(t, sampleCSV)

	parser, err := NewCSVParser(csvPath)
	require.NoError(t, err)
	defer parser.Close()

	// Read batch of 2
	metrics, err := parser.ReadBatch(2)
	require.NoError(t, err)
	assert.Len(t, metrics, 2)

	// Read remaining
	metrics, err = parser.ReadBatch(10)
	require.NoError(t, err)
	assert.Len(t, metrics, 2) // Only 2 remaining
}

func TestReadAll(t *testing.T) {
	csvPath := createTestCSV(t, sampleCSV)

	parser, err := NewCSVParser(csvPath)
	require.NoError(t, err)
	defer parser.Close()

	metrics, err := parser.ReadAll()
	require.NoError(t, err)
	assert.Len(t, metrics, 4)
}

func TestReset(t *testing.T) {
	csvPath := createTestCSV(t, sampleCSV)

	parser, err := NewCSVParser(csvPath)
	require.NoError(t, err)
	defer parser.Close()

	// Read all
	metrics1, err := parser.ReadAll()
	require.NoError(t, err)
	assert.Len(t, metrics1, 4)

	// Reset and read again
	err = parser.Reset()
	require.NoError(t, err)

	metrics2, err := parser.ReadAll()
	require.NoError(t, err)
	assert.Len(t, metrics2, 4)
}

func TestCountRecords(t *testing.T) {
	csvPath := createTestCSV(t, sampleCSV)

	count, err := CountRecords(csvPath)
	require.NoError(t, err)
	assert.Equal(t, 4, count)
}

func TestValidateCSV(t *testing.T) {
	csvPath := createTestCSV(t, sampleCSV)

	err := ValidateCSV(csvPath)
	require.NoError(t, err)
}

func TestValidateCSVMissingColumns(t *testing.T) {
	// CSV without required columns
	invalidCSV := `col1,col2
value1,value2
`
	csvPath := createTestCSV(t, invalidCSV)

	err := ValidateCSV(csvPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required column")
}

func TestValidateCSVEmpty(t *testing.T) {
	// CSV with only headers
	emptyCSV := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
`
	csvPath := createTestCSV(t, emptyCSV)

	err := ValidateCSV(csvPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected map[string]string
	}{
		{
			name: "simple labels",
			raw:  "key1=value1,key2=value2",
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "quoted values",
			raw:  `key1="value1",key2="value2"`,
			expected: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name:     "empty string",
			raw:      "",
			expected: map[string]string{},
		},
		{
			name: "DCGM format",
			raw:  "DCGM_FI_DRIVER_VERSION=535.129.03,DCGM_EXPORTER=dgx_dcgm_exporter:9400",
			expected: map[string]string{
				"DCGM_FI_DRIVER_VERSION": "535.129.03",
				"DCGM_EXPORTER":          "dgx_dcgm_exporter:9400",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLabels(tt.raw)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseFloatValue(t *testing.T) {
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,H100,host1,,,,85.5,
2025-07-18T20:42:34Z,DCGM_FI_DEV_SM_CLOCK,0,nvidia0,GPU-12345,H100,host1,,,,1980,
2025-07-18T20:42:34Z,DCGM_FI_DEV_TEMP,0,nvidia0,GPU-12345,H100,host1,,,,45.25,
`
	csvPath := createTestCSV(t, csvContent)

	parser, err := NewCSVParser(csvPath)
	require.NoError(t, err)
	defer parser.Close()

	metrics, err := parser.ReadAll()
	require.NoError(t, err)
	require.Len(t, metrics, 3)

	assert.Equal(t, 85.5, metrics[0].Value)
	assert.Equal(t, 1980.0, metrics[1].Value)
	assert.Equal(t, 45.25, metrics[2].Value)
}

func TestReadNextEOF(t *testing.T) {
	csvPath := createTestCSV(t, sampleCSV)

	parser, err := NewCSVParser(csvPath)
	require.NoError(t, err)
	defer parser.Close()

	// Read all records
	for i := 0; i < 4; i++ {
		metric, err := parser.ReadNext()
		require.NoError(t, err)
		require.NotNil(t, metric)
	}

	// Next read should return nil (EOF)
	metric, err := parser.ReadNext()
	require.NoError(t, err)
	assert.Nil(t, metric)
}

func TestCaseInsensitiveHeaders(t *testing.T) {
	csvContent := `TIMESTAMP,Metric_Name,GPU_ID,Device,UUID,ModelName,HOSTNAME,Container,Pod,Namespace,VALUE,Labels_Raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-12345,H100,host1,,,,85.5,
`
	csvPath := createTestCSV(t, csvContent)

	parser, err := NewCSVParser(csvPath)
	require.NoError(t, err)
	defer parser.Close()

	metric, err := parser.ReadNext()
	require.NoError(t, err)
	require.NotNil(t, metric)

	assert.Equal(t, "DCGM_FI_DEV_GPU_UTIL", metric.MetricName)
	assert.Equal(t, "GPU-12345", metric.UUID)
	assert.Equal(t, 85.5, metric.Value)
}

func TestMissingRequiredField(t *testing.T) {
	// Missing UUID
	csvContent := `timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,,H100,host1,,,,85.5,
`
	csvPath := createTestCSV(t, csvContent)

	parser, err := NewCSVParser(csvPath)
	require.NoError(t, err)
	defer parser.Close()

	metric, err := parser.ReadNext()
	assert.Error(t, err)
	assert.Nil(t, metric)
	assert.Contains(t, err.Error(), "uuid")
}
