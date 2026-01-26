// MQ Server - Custom Message Queue Server
//
// This is the standalone message queue server that provides
// pub/sub messaging for the telemetry pipeline.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cisco/gpu-telemetry-pipeline/internal/mq"
	"github.com/cisco/gpu-telemetry-pipeline/pkg/config"
)

func main() {
	// Setup logging
	logger := log.New(os.Stdout, "[MQ-SERVER] ", log.LstdFlags|log.Lmicroseconds)

	// Load configuration from environment variables
	cfg := config.DefaultMQServerConfig()

	// Create server config
	serverCfg := mq.ServerConfig{
		TCPHost:  cfg.TCPHost,
		TCPPort:  cfg.TCPPort,
		HTTPHost: cfg.HTTPHost,
		HTTPPort: cfg.HTTPPort,
		Queue: mq.QueueConfig{
			PublishTimeout: cfg.Queue.PublishTimeout,
			BufferSize:     cfg.Queue.BufferSize,
			MaxRetries:     cfg.Queue.MaxRetries,
			RetryDelay:     cfg.Queue.RetryDelay,
		},
	}

	// Create and start server
	server := mq.NewServer(serverCfg, logger)

	logger.Printf("Starting MQ Server...")
	logger.Printf("  TCP: %s:%d", serverCfg.TCPHost, serverCfg.TCPPort)
	logger.Printf("  HTTP: %s:%d", serverCfg.HTTPHost, serverCfg.HTTPPort)
	logger.Printf("  Buffer Size: %d", serverCfg.Queue.BufferSize)

	if err := server.Start(); err != nil {
		logger.Fatalf("Failed to start server: %v", err)
	}

	logger.Println("MQ Server started successfully")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Printf("Received signal %v, shutting down...", sig)

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Stop(ctx); err != nil {
		logger.Printf("Error during shutdown: %v", err)
	}

	logger.Println("MQ Server stopped")
}
