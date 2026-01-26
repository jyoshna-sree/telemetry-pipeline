package mq

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"testing"
	"time"
)

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

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

func TestNewServer(t *testing.T) {
	cfg := DefaultServerConfig()
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)

	server := NewServer(cfg, logger)
	if server == nil {
		t.Fatal("expected server to be created")
	}

	// Test with nil logger
	server2 := NewServer(cfg, nil)
	if server2 == nil {
		t.Fatal("expected server with nil logger to be created")
	}
}

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()

	if cfg.Host == "" {
		t.Error("expected non-empty host")
	}
	if cfg.Port <= 0 {
		t.Error("expected positive port")
	}
	if cfg.Timeout <= 0 {
		t.Error("expected positive timeout")
	}
}

func TestNewClient(t *testing.T) {
	cfg := DefaultClientConfig()
	client := NewClient(cfg)

	if client == nil {
		t.Fatal("expected client to be created")
	}

	if client.IsConnected() {
		t.Error("expected client to not be connected initially")
	}
}

func TestClientConnectFailure(t *testing.T) {
	cfg := ClientConfig{
		Host:    "localhost",
		Port:    59999, // Unlikely to have anything running here
		Timeout: 100 * time.Millisecond,
	}
	client := NewClient(cfg)

	err := client.Connect()
	if err == nil {
		t.Error("expected connection to fail")
		client.Close()
	}
}

func TestClientClose(t *testing.T) {
	cfg := DefaultClientConfig()
	client := NewClient(cfg)

	// Close without connecting should not error
	err := client.Close()
	if err != nil {
		t.Errorf("expected no error closing unconnected client, got %v", err)
	}
}

func TestProtocolMessage(t *testing.T) {
	msg := ProtocolMessage{
		Type:         MsgTypePublish,
		SubscriberID: "test-sub",
		MessageID:    "msg-123",
		Payload:      json.RawMessage(`{"test": "data"}`),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ProtocolMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Type != MsgTypePublish {
		t.Error("type mismatch")
	}
	if decoded.SubscriberID != "test-sub" {
		t.Error("subscriber ID mismatch")
	}
}

func TestMessageTypes(t *testing.T) {
	// Verify message type constants are defined
	types := []string{
		MsgTypePublish,
		MsgTypeSubscribe,
		MsgTypeUnsubscribe,
		MsgTypeAck,
		MsgTypeNack,
		MsgTypeGetStats,
		MsgTypeMessage,
		MsgTypeResponse,
		MsgTypeError,
	}

	for _, typ := range types {
		if typ == "" {
			t.Error("found empty message type")
		}
	}
}

func TestIntegrationServerClient(t *testing.T) {
	// Start server on random ports
	cfg := ServerConfig{
		TCPHost:  "127.0.0.1",
		TCPPort:  0, // Will fail to bind to 0, use specific port
		HTTPHost: "127.0.0.1",
		HTTPPort: 0,
		Queue:    DefaultQueueConfig(),
	}

	// Use specific ports for testing
	cfg.TCPPort = 19876
	cfg.HTTPPort = 19877

	logger := log.New(os.Stdout, "[TEST-SERVER] ", log.LstdFlags)
	server := NewServer(cfg, logger)

	if err := server.Start(); err != nil {
		t.Skipf("Could not start server (port may be in use): %v", err)
	}
	defer server.Stop(context.Background())

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Create and connect client
	clientCfg := ClientConfig{
		Host:          "127.0.0.1",
		Port:          cfg.TCPPort,
		Timeout:       5 * time.Second,
		AutoReconnect: false,
	}
	client := NewClient(clientCfg)

	if err := client.Connect(); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer client.Close()

	if !client.IsConnected() {
		t.Error("expected client to be connected")
	}

	// Test publish
	ctx := context.Background()
	err := client.Publish(ctx, []byte(`{"test": "data"}`))
	if err != nil {
		t.Errorf("failed to publish: %v", err)
	}
}

func TestClientPublishNotConnected(t *testing.T) {
	cfg := DefaultClientConfig()
	client := NewClient(cfg)

	ctx := context.Background()
	err := client.Publish(ctx, []byte("test"))
	if err == nil {
		t.Error("expected error publishing when not connected")
	}
}

func TestOffsetConstants(t *testing.T) {
	if OffsetEarliest >= 0 {
		t.Error("OffsetEarliest should be negative")
	}
	if OffsetLatest >= 0 {
		t.Error("OffsetLatest should be negative")
	}
	if OffsetEarliest == OffsetLatest {
		t.Error("OffsetEarliest and OffsetLatest should be different")
	}
}

func TestQueueErrors(t *testing.T) {
	// Test error values are properly defined
	errors := []error{
		ErrQueueFull,
		ErrPublishTimeout,
		ErrQueueShutdown,
		ErrInvalidConfig,
		ErrInvalidOffset,
		ErrSubscriberExists,
		ErrSubscriberNotFound,
	}

	for _, err := range errors {
		if err == nil {
			t.Error("found nil error")
		}
		if err.Error() == "" {
			t.Error("error has empty message")
		}
	}
}
