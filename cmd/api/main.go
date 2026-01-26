// API Gateway - REST API for GPU Telemetry
//
// @title           GPU Telemetry API
// @version         1.0
// @description     REST API for querying GPU telemetry data from an AI cluster.
//
// @host            localhost:8080
// @BasePath        /
//
// @schemes         http
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cisco/gpu-telemetry-pipeline/internal/api"
	"github.com/cisco/gpu-telemetry-pipeline/internal/storage"
	"github.com/cisco/gpu-telemetry-pipeline/pkg/config"

	_ "github.com/cisco/gpu-telemetry-pipeline/docs"
)

func main() {
	// Setup logging
	logger := log.New(os.Stdout, "[API] ", log.LstdFlags|log.Lmicroseconds)

	// Load configuration from environment variables
	cfg := config.DefaultAPIConfig()

	logger.Printf("Starting API Gateway...")
	logger.Printf("  Host: %s", cfg.Host)
	logger.Printf("  Port: %d", cfg.Port)

	// Create InfluxDB storage from environment variables
	influxCfg := storage.DefaultInfluxDBConfig()
	logger.Printf("Connecting to InfluxDB at %s (org=%s, bucket=%s)", influxCfg.URL, influxCfg.Org, influxCfg.Bucket)

	store, err := storage.NewInfluxDBStorage(influxCfg)
	if err != nil {
		logger.Fatalf("Failed to connect to InfluxDB: %v", err)
	}
	logger.Printf("Connected to InfluxDB")
	defer store.Close()

	// Create router
	routerConfig := api.RouterConfig{
		DefaultLimit: cfg.DefaultLimit,
		MaxLimit:     cfg.MaxLimit,
	}
	router := api.NewRouter(store, routerConfig)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// Start server in goroutine
	go func() {
		logger.Printf("API server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	logger.Printf("Received signal %v, shutting down...", sig)

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Printf("Error during shutdown: %v", err)
	}

	logger.Println("API server stopped")
}
