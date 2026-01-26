// Telemetry Collector - Consumes telemetry from MQ and persists it
//
// This component subscribes to the message queue, processes incoming
// telemetry batches, and stores them in the configured storage backend.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cisco/gpu-telemetry-pipeline/internal/mq"
	"github.com/cisco/gpu-telemetry-pipeline/internal/storage"
	"github.com/cisco/gpu-telemetry-pipeline/pkg/config"
	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
)

func main() {
	// Setup logging
	logger := log.New(os.Stdout, "[COLLECTOR] ", log.LstdFlags|log.Lmicroseconds)

	// Load configuration from environment variables
	cfg := config.DefaultCollectorConfig()

	logger.Printf("Starting Telemetry Collector...")
	logger.Printf("  Instance ID: %s", cfg.InstanceID)
	logger.Printf("  MQ Server: %s:%d", cfg.MQ.Host, cfg.MQ.Port)
	logger.Printf("  Retention Period: %v", cfg.RetentionPeriod)

	// Create InfluxDB storage backend from environment variables
	influxCfg := storage.DefaultInfluxDBConfig()
	logger.Printf("Connecting to InfluxDB at %s (org=%s, bucket=%s)", influxCfg.URL, influxCfg.Org, influxCfg.Bucket)

	store, err := storage.NewInfluxDBWriteStorage(influxCfg)
	if err != nil {
		logger.Fatalf("Failed to connect to InfluxDB: %v", err)
	}
	logger.Printf("Connected to InfluxDB")
	defer store.Close()

	// Create MQ client
	client := mq.NewClient(mq.ClientConfig{
		Host:          cfg.MQ.Host,
		Port:          cfg.MQ.Port,
		Timeout:       10 * time.Second,
		AutoReconnect: true,
	})

	// Connect to MQ server
	logger.Println("Connecting to MQ server...")
	if err := client.Connect(); err != nil {
		logger.Fatalf("Failed to connect to MQ server: %v", err)
	}
	defer client.Close()

	logger.Println("Connected to MQ server")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create collector
	collector := &Collector{
		client: client,
		store:  store,
		cfg:    cfg,
		logger: logger,
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Start collection
	if err := collector.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Fatalf("Collector error: %v", err)
	}

	logger.Printf("Collector stopped. Total batches processed: %d, Total metrics stored: %d",
		collector.batchesProcessed, collector.metricsStored)
}

// Collector handles message consumption and storage.
type Collector struct {
	client           *mq.Client
	store            storage.Storage
	cfg              config.CollectorConfig
	logger           *log.Logger
	batchesProcessed int64
	metricsStored    int64
}

// Run starts the collector.
func (c *Collector) Run(ctx context.Context) error {
	// Subscribe to the queue starting from latest (new messages only)
	// Use OffsetEarliest to replay all available messages from the beginning
	err := c.client.Subscribe(ctx, c.cfg.InstanceID, mq.OffsetLatest, c.handleMessage)
	if err != nil {
		return err
	}

	c.logger.Println("Subscribed to message queue")

	// Start cleanup goroutine
	go c.cleanupLoop(ctx)

	// Start stats reporter
	go c.statsLoop(ctx)

	// Wait for shutdown
	<-ctx.Done()

	// Unsubscribe
	c.client.Unsubscribe(c.cfg.InstanceID)

	return nil
}

// handleMessage processes incoming messages.
func (c *Collector) handleMessage(ctx context.Context, msg *mq.Message) error {
	// Parse batch
	var batch models.MetricBatch
	if err := json.Unmarshal(msg.Payload, &batch); err != nil {
		c.logger.Printf("Error unmarshaling batch: %v", err)
		return err
	}

	// Store metrics
	metrics := make([]*models.GPUMetric, len(batch.Metrics))
	for i := range batch.Metrics {
		metrics[i] = &batch.Metrics[i]
	}

	if err := c.store.StoreBatch(ctx, metrics); err != nil {
		c.logger.Printf("Error storing batch: %v", err)
		return err
	}

	atomic.AddInt64(&c.batchesProcessed, 1)
	atomic.AddInt64(&c.metricsStored, int64(len(metrics)))

	return nil
}

// cleanupLoop periodically removes old data.
func (c *Collector) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			removed, err := c.store.Cleanup(ctx, c.cfg.RetentionPeriod)
			if err != nil {
				c.logger.Printf("Cleanup error: %v", err)
			} else if removed > 0 {
				c.logger.Printf("Cleanup: removed %d old metrics", removed)
			}
		}
	}
}

// statsLoop periodically logs statistics.
func (c *Collector) statsLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := c.store.Stats()
			c.logger.Printf("Stats: batches=%d, metrics_stored=%d, total_metrics=%d, gpus=%d",
				atomic.LoadInt64(&c.batchesProcessed),
				atomic.LoadInt64(&c.metricsStored),
				stats.TotalMetrics,
				stats.TotalGPUs)
		}
	}
}
