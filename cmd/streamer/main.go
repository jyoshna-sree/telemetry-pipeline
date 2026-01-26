// Telemetry Streamer - Reads CSV telemetry data and streams to MQ
//
// This component continuously reads GPU telemetry from a CSV file,
// buffers it locally, and publishes batches to the message queue
// at configurable intervals.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cisco/gpu-telemetry-pipeline/internal/mq"
	"github.com/cisco/gpu-telemetry-pipeline/internal/parser"
	"github.com/cisco/gpu-telemetry-pipeline/pkg/config"
	"github.com/cisco/gpu-telemetry-pipeline/pkg/models"
	"github.com/google/uuid"
)

func main() {
	// Setup logging
	logger := log.New(os.Stdout, "[STREAMER] ", log.LstdFlags|log.Lmicroseconds)

	// Load configuration from environment variables
	cfg := config.DefaultStreamerConfig()

	logger.Printf("Starting Telemetry Streamer...")
	logger.Printf("  Instance ID: %s", cfg.InstanceID)
	logger.Printf("  CSV Path: %s", cfg.CSVPath)
	logger.Printf("  Collect Interval: %v", cfg.CollectInterval)
	logger.Printf("  Publish Interval: %v", cfg.StreamInterval)
	logger.Printf("  Loop: %v", cfg.Loop)
	logger.Printf("  MQ Server: %s:%d", cfg.MQ.Host, cfg.MQ.Port)

	// Validate CSV file
	if err := parser.ValidateCSV(cfg.CSVPath); err != nil {
		logger.Fatalf("Invalid CSV file: %v", err)
	}

	// Count records for logging
	recordCount, err := parser.CountRecords(cfg.CSVPath)
	if err != nil {
		logger.Printf("Warning: could not count records: %v", err)
	} else {
		logger.Printf("  Total Records: %d", recordCount)
	}

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

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Start streaming
	streamer := &Streamer{
		client:      client,
		cfg:         cfg,
		logger:      logger,
		buffer:      make([]*models.GPUMetric, 0, 1000),
		batchesSent: 0,
		metricsSent: 0,
	}

	if err := streamer.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Fatalf("Streamer error: %v", err)
	}

	logger.Printf("Streamer stopped. Total batches sent: %d, Total metrics sent: %d",
		streamer.batchesSent, streamer.metricsSent)
}

// Streamer handles reading CSV data, buffering, and publishing to MQ.
type Streamer struct {
	client      *mq.Client
	cfg         config.StreamerConfig
	logger      *log.Logger
	buffer      []*models.GPUMetric // Local buffer to collect metrics
	bufferMu    sync.Mutex          // Protect buffer access
	batchesSent int64
	metricsSent int64
}

// Run starts two goroutines:
// 1. Collector - reads CSV and buffers locally
// 2. Publisher - periodically sends buffer to MQ
func (s *Streamer) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	// Channel to signal collector is done (EOF reached, no loop)
	collectorDone := make(chan struct{})

	// Start collector goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(collectorDone)
		s.collectLoop(ctx)
	}()

	// Start publisher goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.publishLoop(ctx, collectorDone)
	}()

	wg.Wait()
	return nil
}

// collectLoop continuously reads from CSV and buffers metrics.
func (s *Streamer) collectLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.CollectInterval)
	defer ticker.Stop()

	for {
		// Create parser for this iteration
		csvParser, err := parser.NewCSVParser(s.cfg.CSVPath)
		if err != nil {
			s.logger.Printf("Error opening CSV: %v", err)
			return
		}

		// Read all records from CSV
		if err := s.readCSV(ctx, csvParser, ticker); err != nil {
			csvParser.Close()
			if ctx.Err() != nil {
				return // Graceful shutdown
			}
			s.logger.Printf("Error reading CSV: %v", err)
			return
		}

		csvParser.Close()

		// Check if we should loop
		if !s.cfg.Loop {
			s.logger.Println("Finished reading CSV (loop disabled)")
			return
		}

		s.logger.Println("Reached end of CSV, restarting from beginning...")

		// Check for shutdown before looping
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// readCSV reads data from CSV and adds to buffer.
func (s *Streamer) readCSV(ctx context.Context, csvParser *parser.CSVParser, ticker *time.Ticker) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Read one metric at a time
			metric, err := csvParser.ReadNext()
			if err != nil {
				s.logger.Printf("Error reading metric: %v", err)
				continue
			}

			// End of file
			if metric == nil {
				return nil
			}

			// Update timestamp to current time
			metric.Timestamp = time.Now()

			// Add to buffer (thread-safe)
			s.bufferMu.Lock()
			s.buffer = append(s.buffer, metric)
			bufLen := len(s.buffer)
			s.bufferMu.Unlock()

			if bufLen%100 == 0 {
				s.logger.Printf("Buffer size: %d metrics", bufLen)
			}
		}
	}
}

// publishLoop periodically sends buffered metrics to MQ.
func (s *Streamer) publishLoop(ctx context.Context, collectorDone <-chan struct{}) {
	ticker := time.NewTicker(s.cfg.StreamInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final flush before shutdown
			s.flushBuffer(ctx)
			return

		case <-collectorDone:
			// Collector finished, do final flush
			s.flushBuffer(ctx)
			return

		case <-ticker.C:
			// Periodic flush
			s.flushBuffer(ctx)
		}
	}
}

// flushBuffer sends all buffered metrics to MQ and clears the buffer.
func (s *Streamer) flushBuffer(ctx context.Context) {
	// Get and clear buffer atomically
	s.bufferMu.Lock()
	if len(s.buffer) == 0 {
		s.bufferMu.Unlock()
		return
	}

	// Take ownership of current buffer
	metrics := s.buffer
	s.buffer = make([]*models.GPUMetric, 0, 1000)
	s.bufferMu.Unlock()

	s.logger.Printf("Flushing %d metrics to MQ...", len(metrics))

	// Create batch
	batch := &models.MetricBatch{
		BatchID:     uuid.New().String(),
		Source:      s.cfg.InstanceID,
		CollectedAt: time.Now(),
		Metrics:     make([]models.GPUMetric, len(metrics)),
	}

	// Copy metrics to batch
	for i, m := range metrics {
		batch.Metrics[i] = *m
	}

	// Serialize
	payload, err := json.Marshal(batch)
	if err != nil {
		s.logger.Printf("Error marshaling batch: %v", err)
		return
	}

	// Publish with retry
	var publishErr error
	for retries := 0; retries < 3; retries++ {
		publishErr = s.client.Publish(ctx, payload)
		if publishErr == nil {
			break
		}
		s.logger.Printf("Publish attempt %d failed: %v", retries+1, publishErr)
		time.Sleep(time.Duration(retries+1) * time.Second)
	}

	if publishErr != nil {
		s.logger.Printf("Failed to publish batch after retries: %v", publishErr)
		return
	}

	s.batchesSent++
	s.metricsSent += int64(len(metrics))

	s.logger.Printf("Batch sent: %d metrics (total: %d batches, %d metrics)",
		len(metrics), s.batchesSent, s.metricsSent)
}
