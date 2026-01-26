// Package config provides configuration structures and loading for all pipeline components.
package config

import (
	"os"
	"strconv"
	"time"
)

// MQClientConfig holds configuration for clients connecting to the MQ server.
// Used by: Streamer, Collector, API
type MQClientConfig struct {
	// Host is the MQ server host to connect to
	Host string `yaml:"host" json:"host"`

	// Port is the MQ server port to connect to
	Port int `yaml:"port" json:"port"`

	// MaxRetries is the maximum number of retry attempts for failed connections
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration `yaml:"retry_delay" json:"retry_delay"`

	// PublishTimeout is the timeout for publishing messages
	PublishTimeout time.Duration `yaml:"publish_timeout" json:"publish_timeout"`
}

// MQQueueConfig holds configuration for the MQ server's internal queue.
// Used by: MQ Server only
type MQQueueConfig struct {
	// BufferSize is the buffer size for the queue
	BufferSize int `yaml:"buffer_size" json:"buffer_size"`

	// MaxRetries is the maximum number of retry attempts for failed messages
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration `yaml:"retry_delay" json:"retry_delay"`

	// PublishTimeout is the timeout for publishing messages
	PublishTimeout time.Duration `yaml:"publish_timeout" json:"publish_timeout"`
}

// MQConfig is kept for backward compatibility - combines client and queue config.
// Deprecated: Use MQClientConfig for clients and MQQueueConfig for server.
type MQConfig struct {
	Host           string        `yaml:"host" json:"host"`
	Port           int           `yaml:"port" json:"port"`
	BufferSize     int           `yaml:"buffer_size" json:"buffer_size"`
	MaxRetries     int           `yaml:"max_retries" json:"max_retries"`
	RetryDelay     time.Duration `yaml:"retry_delay" json:"retry_delay"`
	PublishTimeout time.Duration `yaml:"publish_timeout" json:"publish_timeout"`
}

// StreamerConfig holds configuration for the telemetry streamer.
type StreamerConfig struct {
	// InstanceID uniquely identifies this streamer instance
	InstanceID string `yaml:"instance_id" json:"instance_id"`

	// CSVPath is the path to the telemetry CSV file
	CSVPath string `yaml:"csv_path" json:"csv_path"`

	// BatchSize is the number of metrics to send in each batch
	BatchSize int `yaml:"batch_size" json:"batch_size"`

	// CollectInterval is how often to read metrics from CSV into buffer
	CollectInterval time.Duration `yaml:"collect_interval" json:"collect_interval"`

	// StreamInterval is how often to publish buffered metrics to MQ
	StreamInterval time.Duration `yaml:"stream_interval" json:"stream_interval"`

	// Loop indicates whether to loop the CSV data continuously
	Loop bool `yaml:"loop" json:"loop"`

	// MQ is the message queue configuration
	MQ MQConfig `yaml:"mq" json:"mq"`

	// HostFilter optionally filters which hosts this streamer handles
	HostFilter []string `yaml:"host_filter" json:"host_filter"`
}

// CollectorConfig holds configuration for the telemetry collector.
type CollectorConfig struct {
	// InstanceID uniquely identifies this collector instance
	InstanceID string `yaml:"instance_id" json:"instance_id"`

	// MQ is the message queue configuration
	MQ MQConfig `yaml:"mq" json:"mq"`

	// InfluxDB configuration
	InfluxURL    string `yaml:"influx_url" json:"influx_url"`
	InfluxToken  string `yaml:"influx_token" json:"influx_token"`
	InfluxOrg    string `yaml:"influx_org" json:"influx_org"`
	InfluxBucket string `yaml:"influx_bucket" json:"influx_bucket"`

	// RetentionPeriod is how long to keep telemetry data
	RetentionPeriod time.Duration `yaml:"retention_period" json:"retention_period"`

	// FlushInterval is how often to flush data to storage
	FlushInterval time.Duration `yaml:"flush_interval" json:"flush_interval"`
}

// APIConfig holds configuration for the REST API gateway.
type APIConfig struct {
	// Host is the API server host
	Host string `yaml:"host" json:"host"`

	// Port is the API server port
	Port int `yaml:"port" json:"port"`

	// ReadTimeout is the HTTP read timeout
	ReadTimeout time.Duration `yaml:"read_timeout" json:"read_timeout"`

	// WriteTimeout is the HTTP write timeout
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`

	// DefaultLimit is the default pagination limit
	DefaultLimit int `yaml:"default_limit" json:"default_limit"`

	// MaxLimit is the maximum pagination limit
	MaxLimit int `yaml:"max_limit" json:"max_limit"`
}

// MQServerConfig holds configuration for the message queue server.
type MQServerConfig struct {
	// TCPHost is the TCP server host
	TCPHost string `yaml:"tcp_host" json:"tcp_host"`

	// TCPPort is the TCP server port for client connections
	TCPPort int `yaml:"tcp_port" json:"tcp_port"`

	// HTTPHost is the HTTP server host (for health/metrics)
	HTTPHost string `yaml:"http_host" json:"http_host"`

	// HTTPPort is the HTTP server port
	HTTPPort int `yaml:"http_port" json:"http_port"`

	// Queue is the internal queue configuration (no host/port needed)
	Queue MQQueueConfig `yaml:"queue" json:"queue"`
}

// DefaultMQClientConfig returns a default MQ client configuration.
func DefaultMQClientConfig() MQClientConfig {
	return MQClientConfig{
		Host:           getEnv("MQ_HOST", "localhost"),
		Port:           getEnvInt("MQ_PORT", 9000),
		MaxRetries:     getEnvInt("MQ_MAX_RETRIES", 3),
		RetryDelay:     getEnvDuration("MQ_RETRY_DELAY", time.Second),
		PublishTimeout: getEnvDuration("MQ_PUBLISH_TIMEOUT", 5*time.Second),
	}
}

// DefaultMQQueueConfig returns a default MQ queue configuration.
func DefaultMQQueueConfig() MQQueueConfig {
	return MQQueueConfig{
		BufferSize:     getEnvInt("MQ_BUFFER_SIZE", 10000),
		MaxRetries:     getEnvInt("MQ_MAX_RETRIES", 3),
		RetryDelay:     getEnvDuration("MQ_RETRY_DELAY", time.Second),
		PublishTimeout: getEnvDuration("MQ_PUBLISH_TIMEOUT", 5*time.Second),
	}
}

// DefaultMQConfig returns a default MQ configuration (for backward compatibility).
// Deprecated: Use DefaultMQClientConfig or DefaultMQQueueConfig instead.
func DefaultMQConfig() MQConfig {
	return MQConfig{
		Host:           getEnv("MQ_HOST", "localhost"),
		Port:           getEnvInt("MQ_PORT", 9000),
		BufferSize:     getEnvInt("MQ_BUFFER_SIZE", 10000),
		MaxRetries:     getEnvInt("MQ_MAX_RETRIES", 3),
		RetryDelay:     getEnvDuration("MQ_RETRY_DELAY", time.Second),
		PublishTimeout: getEnvDuration("MQ_PUBLISH_TIMEOUT", 5*time.Second),
	}
}

// DefaultStreamerConfig returns a default Streamer configuration.
func DefaultStreamerConfig() StreamerConfig {
	return StreamerConfig{
		InstanceID:      getEnv("STREAMER_ID", "streamer-1"),
		CSVPath:         getEnv("CSV_PATH", "/data/telemetry.csv"),
		BatchSize:       getEnvInt("BATCH_SIZE", 100),
		CollectInterval: getEnvDuration("COLLECT_INTERVAL", 100*time.Millisecond),
		StreamInterval:  getEnvDuration("STREAM_INTERVAL", time.Second),
		Loop:            getEnvBool("LOOP", true),
		MQ:              DefaultMQConfig(),
		HostFilter:      nil,
	}
}

// DefaultCollectorConfig returns a default Collector configuration.
func DefaultCollectorConfig() CollectorConfig {
	return CollectorConfig{
		InstanceID:      getEnv("COLLECTOR_ID", "collector-1"),
		MQ:              DefaultMQConfig(),
		InfluxURL:       getEnv("INFLUXDB_URL", "http://localhost:8086"),
		InfluxToken:     getEnv("INFLUXDB_TOKEN", ""),
		InfluxOrg:       getEnv("INFLUXDB_ORG", "cisco"),
		InfluxBucket:    getEnv("INFLUXDB_BUCKET", "gpu_telemetry"),
		RetentionPeriod: getEnvDuration("RETENTION_PERIOD", 24*time.Hour),
		FlushInterval:   getEnvDuration("FLUSH_INTERVAL", 10*time.Second),
	}
}

// DefaultAPIConfig returns a default API configuration.
func DefaultAPIConfig() APIConfig {
	return APIConfig{
		Host:         getEnv("API_HOST", "0.0.0.0"),
		Port:         getEnvInt("API_PORT", 8080),
		ReadTimeout:  getEnvDuration("API_READ_TIMEOUT", 10*time.Second),
		WriteTimeout: getEnvDuration("API_WRITE_TIMEOUT", 10*time.Second),
		DefaultLimit: getEnvInt("DEFAULT_LIMIT", 100),
		MaxLimit:     getEnvInt("MAX_LIMIT", 1000),
	}
}

// DefaultMQServerConfig returns a default MQ Server configuration.
func DefaultMQServerConfig() MQServerConfig {
	return MQServerConfig{
		TCPHost:  getEnv("TCP_HOST", "0.0.0.0"),
		TCPPort:  getEnvInt("TCP_PORT", 9000),
		HTTPHost: getEnv("HTTP_HOST", "0.0.0.0"),
		HTTPPort: getEnvInt("HTTP_PORT", 9001),
		Queue:    DefaultMQQueueConfig(),
	}
}

// Helper functions for environment variable parsing.

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
