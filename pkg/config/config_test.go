package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultMQClientConfig(t *testing.T) {
	cfg := DefaultMQClientConfig()

	if cfg.Host == "" {
		t.Error("expected non-empty host")
	}
	if cfg.Port <= 0 {
		t.Error("expected positive port")
	}
	if cfg.MaxRetries <= 0 {
		t.Error("expected positive max retries")
	}
	if cfg.RetryDelay <= 0 {
		t.Error("expected positive retry delay")
	}
	if cfg.PublishTimeout <= 0 {
		t.Error("expected positive publish timeout")
	}
}

func TestDefaultMQQueueConfig(t *testing.T) {
	cfg := DefaultMQQueueConfig()

	if cfg.BufferSize <= 0 {
		t.Error("expected positive buffer size")
	}
	if cfg.MaxRetries <= 0 {
		t.Error("expected positive max retries")
	}
}

func TestDefaultMQConfig(t *testing.T) {
	cfg := DefaultMQConfig()

	if cfg.Host == "" {
		t.Error("expected non-empty host")
	}
	if cfg.Port <= 0 {
		t.Error("expected positive port")
	}
	if cfg.BufferSize <= 0 {
		t.Error("expected positive buffer size")
	}
}

func TestDefaultStreamerConfig(t *testing.T) {
	cfg := DefaultStreamerConfig()

	if cfg.InstanceID == "" {
		t.Error("expected non-empty instance ID")
	}
	if cfg.CSVPath == "" {
		t.Error("expected non-empty CSV path")
	}
	if cfg.BatchSize <= 0 {
		t.Error("expected positive batch size")
	}
	if cfg.CollectInterval <= 0 {
		t.Error("expected positive collect interval")
	}
	if cfg.StreamInterval <= 0 {
		t.Error("expected positive stream interval")
	}
}

func TestDefaultCollectorConfig(t *testing.T) {
	cfg := DefaultCollectorConfig()

	if cfg.InstanceID == "" {
		t.Error("expected non-empty instance ID")
	}
	if cfg.InfluxURL == "" {
		t.Error("expected non-empty InfluxDB URL")
	}
	if cfg.InfluxOrg == "" {
		t.Error("expected non-empty InfluxDB org")
	}
	if cfg.InfluxBucket == "" {
		t.Error("expected non-empty InfluxDB bucket")
	}
	if cfg.RetentionPeriod <= 0 {
		t.Error("expected positive retention period")
	}
}

func TestDefaultAPIConfig(t *testing.T) {
	cfg := DefaultAPIConfig()

	if cfg.Host == "" {
		t.Error("expected non-empty host")
	}
	if cfg.Port <= 0 {
		t.Error("expected positive port")
	}
	if cfg.ReadTimeout <= 0 {
		t.Error("expected positive read timeout")
	}
	if cfg.WriteTimeout <= 0 {
		t.Error("expected positive write timeout")
	}
	if cfg.DefaultLimit <= 0 {
		t.Error("expected positive default limit")
	}
	if cfg.MaxLimit <= 0 {
		t.Error("expected positive max limit")
	}
}

func TestDefaultMQServerConfig(t *testing.T) {
	cfg := DefaultMQServerConfig()

	if cfg.TCPHost == "" {
		t.Error("expected non-empty TCP host")
	}
	if cfg.TCPPort <= 0 {
		t.Error("expected positive TCP port")
	}
	if cfg.HTTPHost == "" {
		t.Error("expected non-empty HTTP host")
	}
	if cfg.HTTPPort <= 0 {
		t.Error("expected positive HTTP port")
	}
}

func TestGetEnv(t *testing.T) {
	// Test default value
	result := getEnv("NONEXISTENT_KEY_12345", "default")
	if result != "default" {
		t.Errorf("expected 'default', got '%s'", result)
	}

	// Test with env var set
	os.Setenv("TEST_KEY_CONFIG", "custom_value")
	defer os.Unsetenv("TEST_KEY_CONFIG")

	result = getEnv("TEST_KEY_CONFIG", "default")
	if result != "custom_value" {
		t.Errorf("expected 'custom_value', got '%s'", result)
	}
}

func TestGetEnvInt(t *testing.T) {
	// Test default value
	result := getEnvInt("NONEXISTENT_KEY_12345", 42)
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}

	// Test with valid int
	os.Setenv("TEST_INT_KEY", "100")
	defer os.Unsetenv("TEST_INT_KEY")

	result = getEnvInt("TEST_INT_KEY", 42)
	if result != 100 {
		t.Errorf("expected 100, got %d", result)
	}

	// Test with invalid int
	os.Setenv("TEST_INT_INVALID", "not_a_number")
	defer os.Unsetenv("TEST_INT_INVALID")

	result = getEnvInt("TEST_INT_INVALID", 42)
	if result != 42 {
		t.Errorf("expected 42 for invalid int, got %d", result)
	}
}

func TestGetEnvBool(t *testing.T) {
	// Test default value
	result := getEnvBool("NONEXISTENT_KEY_12345", true)
	if result != true {
		t.Error("expected true")
	}

	// Test with valid bool
	os.Setenv("TEST_BOOL_KEY", "false")
	defer os.Unsetenv("TEST_BOOL_KEY")

	result = getEnvBool("TEST_BOOL_KEY", true)
	if result != false {
		t.Error("expected false")
	}

	// Test with invalid bool
	os.Setenv("TEST_BOOL_INVALID", "not_a_bool")
	defer os.Unsetenv("TEST_BOOL_INVALID")

	result = getEnvBool("TEST_BOOL_INVALID", true)
	if result != true {
		t.Error("expected true for invalid bool")
	}
}

func TestGetEnvDuration(t *testing.T) {
	// Test default value
	result := getEnvDuration("NONEXISTENT_KEY_12345", 5*time.Second)
	if result != 5*time.Second {
		t.Errorf("expected 5s, got %v", result)
	}

	// Test with valid duration
	os.Setenv("TEST_DUR_KEY", "10s")
	defer os.Unsetenv("TEST_DUR_KEY")

	result = getEnvDuration("TEST_DUR_KEY", 5*time.Second)
	if result != 10*time.Second {
		t.Errorf("expected 10s, got %v", result)
	}

	// Test with invalid duration
	os.Setenv("TEST_DUR_INVALID", "not_a_duration")
	defer os.Unsetenv("TEST_DUR_INVALID")

	result = getEnvDuration("TEST_DUR_INVALID", 5*time.Second)
	if result != 5*time.Second {
		t.Errorf("expected 5s for invalid duration, got %v", result)
	}
}

func TestConfigWithEnvOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("API_HOST", "127.0.0.1")
	os.Setenv("API_PORT", "9999")
	os.Setenv("DEFAULT_LIMIT", "50")
	defer func() {
		os.Unsetenv("API_HOST")
		os.Unsetenv("API_PORT")
		os.Unsetenv("DEFAULT_LIMIT")
	}()

	cfg := DefaultAPIConfig()

	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected host '127.0.0.1', got '%s'", cfg.Host)
	}
	if cfg.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Port)
	}
	if cfg.DefaultLimit != 50 {
		t.Errorf("expected default limit 50, got %d", cfg.DefaultLimit)
	}
}
