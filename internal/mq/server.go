// Package mq provides a TCP-based message queue server.
package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// Server is a TCP server for the message queue.
type Server struct {
	queue       *InMemoryQueue
	tcpListener net.Listener
	httpServer  *http.Server
	tcpAddr     string
	httpAddr    string
	clients     map[net.Conn]*clientState
	clientsMu   sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	logger      *log.Logger
}

// clientState tracks per-client state.
type clientState struct {
	conn         net.Conn
	subscriberID string
	subscribed   bool
	mu           sync.Mutex
}

// ServerConfig configures the MQ server.
type ServerConfig struct {
	TCPHost  string      `json:"tcp_host"`
	TCPPort  int         `json:"tcp_port"`
	HTTPHost string      `json:"http_host"`
	HTTPPort int         `json:"http_port"`
	Queue    QueueConfig `json:"queue"`
}

// DefaultServerConfig returns a server config with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		TCPHost:  "0.0.0.0",
		TCPPort:  9000,
		HTTPHost: "0.0.0.0",
		HTTPPort: 9001,
		Queue:    DefaultQueueConfig(),
	}
}

// NewServer creates a new MQ server.
func NewServer(config ServerConfig, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}

	ctx, cancel := context.WithCancel(context.Background())
	queue := NewInMemoryQueue(config.Queue)

	return &Server{
		queue:    queue,
		tcpAddr:  fmt.Sprintf("%s:%d", config.TCPHost, config.TCPPort),
		httpAddr: fmt.Sprintf("%s:%d", config.HTTPHost, config.HTTPPort),
		clients:  make(map[net.Conn]*clientState),
		ctx:      ctx,
		cancel:   cancel,
		logger:   logger,
	}
}

// Start starts the MQ server.
func (s *Server) Start() error {
	// Start the queue
	if err := s.queue.Start(s.ctx); err != nil {
		return fmt.Errorf("failed to start queue: %w", err)
	}

	// Start TCP listener
	listener, err := net.Listen("tcp", s.tcpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.tcpAddr, err)
	}
	s.tcpListener = listener
	s.logger.Printf("MQ Server listening on TCP %s", s.tcpAddr)

	// Start HTTP server for health/stats
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)

	s.httpServer = &http.Server{
		Addr:    s.httpAddr,
		Handler: mux,
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.logger.Printf("MQ Server HTTP on %s", s.httpAddr)
		if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			s.logger.Printf("HTTP server error: %v", err)
		}
	}()

	// Accept TCP connections
	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop gracefully stops the MQ server.
func (s *Server) Stop(ctx context.Context) error {
	s.cancel()

	// Close TCP listener
	if s.tcpListener != nil {
		s.tcpListener.Close()
	}

	// Close all client connections
	s.clientsMu.Lock()
	for conn := range s.clients {
		conn.Close()
	}
	s.clientsMu.Unlock()

	// Stop HTTP server
	if s.httpServer != nil {
		s.httpServer.Shutdown(ctx)
	}

	// Stop queue
	s.queue.Shutdown(ctx)

	// Wait for goroutines
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// acceptLoop accepts incoming TCP connections.
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.tcpListener.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			s.logger.Printf("Accept error: %v", err)
			continue
		}

		s.clientsMu.Lock()
		s.clients[conn] = &clientState{
			conn: conn,
		}
		s.clientsMu.Unlock()

		s.wg.Add(1)
		go s.handleClient(conn)
	}
}

// handleClient handles a single client connection.
func (s *Server) handleClient(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, conn)
		s.clientsMu.Unlock()
		conn.Close()
	}()

	s.logger.Printf("Client connected: %s", conn.RemoteAddr())

	header := make([]byte, 4)
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Set read deadline
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Read message length
		_, err := io.ReadFull(conn, header)
		if err != nil {
			if err != io.EOF && s.ctx.Err() == nil {
				s.logger.Printf("Client read error: %v", err)
			}
			return
		}

		length := uint32(header[0])<<24 | uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
		if length > 10*1024*1024 { // 10MB max
			s.logger.Printf("Message too large: %d bytes", length)
			continue
		}

		// Read message body
		data := make([]byte, length)
		_, err = io.ReadFull(conn, data)
		if err != nil {
			s.logger.Printf("Client read body error: %v", err)
			return
		}

		var msg ProtocolMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			s.logger.Printf("Invalid message: %v", err)
			continue
		}

		s.handleMessage(conn, &msg)
	}
}

// handleMessage processes a client message.
func (s *Server) handleMessage(conn net.Conn, msg *ProtocolMessage) {
	switch msg.Type {
	case MsgTypePublish:
		s.handlePublish(conn, msg)
	case MsgTypeSubscribe:
		s.handleSubscribe(conn, msg)
	case MsgTypeUnsubscribe:
		s.handleUnsubscribe(conn, msg)
	case MsgTypeAck:
		s.handleAck(conn, msg)
	case MsgTypeNack:
		s.handleNack(conn, msg)
	case MsgTypeGetStats:
		s.handleGetStats(conn, msg)
	default:
		s.sendError(conn, "unknown message type")
	}
}

// handlePublish handles a publish message.
func (s *Server) handlePublish(conn net.Conn, msg *ProtocolMessage) {
	err := s.queue.Publish(s.ctx, msg.Payload)
	if err != nil {
		s.sendError(conn, err.Error())
		return
	}
	s.sendResponse(conn, true, "")
}

// handleSubscribe handles a subscribe message.
func (s *Server) handleSubscribe(conn net.Conn, msg *ProtocolMessage) {
	s.clientsMu.RLock()
	client := s.clients[conn]
	s.clientsMu.RUnlock()

	if client == nil {
		s.sendError(conn, "client not found")
		return
	}

	subscriberID := msg.SubscriberID
	if subscriberID == "" {
		subscriberID = conn.RemoteAddr().String()
	}

	// Use the offset from the message, default to OffsetLatest for new messages only
	startOffset := msg.Offset
	if startOffset == 0 {
		startOffset = OffsetLatest
	}

	handler := func(ctx context.Context, queueMsg *Message) error {
		// Forward message to client
		response := &ProtocolMessage{
			Type:      MsgTypeMessage,
			MessageID: queueMsg.ID,
			Offset:    queueMsg.Offset,
			Payload:   queueMsg.Payload,
		}
		return s.sendToClient(conn, response)
	}

	err := s.queue.Subscribe(s.ctx, subscriberID, startOffset, handler)
	if err != nil {
		s.sendError(conn, err.Error())
		return
	}

	client.mu.Lock()
	client.subscriberID = subscriberID
	client.subscribed = true
	client.mu.Unlock()

	s.sendResponse(conn, true, "")
}

// handleUnsubscribe handles an unsubscribe message.
func (s *Server) handleUnsubscribe(conn net.Conn, msg *ProtocolMessage) {
	s.clientsMu.RLock()
	client := s.clients[conn]
	s.clientsMu.RUnlock()

	if client == nil {
		s.sendError(conn, "client not found")
		return
	}

	client.mu.Lock()
	subscriberID := client.subscriberID
	client.subscribed = false
	client.mu.Unlock()

	if subscriberID == "" {
		subscriberID = msg.SubscriberID
	}

	err := s.queue.Unsubscribe(subscriberID)
	if err != nil {
		s.sendError(conn, err.Error())
		return
	}

	s.sendResponse(conn, true, "")
}

// handleAck handles an ack message.
func (s *Server) handleAck(conn net.Conn, msg *ProtocolMessage) {
	// Acknowledgment is handled automatically by the queue
	s.sendResponse(conn, true, "")
}

// handleNack handles a nack message.
func (s *Server) handleNack(conn net.Conn, msg *ProtocolMessage) {
	// Negative acknowledgment triggers retry in the queue
	s.sendResponse(conn, true, "")
}

// handleGetStats handles a get stats message.
func (s *Server) handleGetStats(conn net.Conn, msg *ProtocolMessage) {
	stats := s.queue.GetStats()
	data, _ := json.Marshal(stats)

	response := &ProtocolMessage{
		Type:    MsgTypeResponse,
		Payload: data,
		Success: true,
	}
	s.sendToClient(conn, response)
}

// sendResponse sends a response to the client.
func (s *Server) sendResponse(conn net.Conn, success bool, errorMsg string) {
	response := &ProtocolMessage{
		Type:    MsgTypeResponse,
		Success: success,
		Error:   errorMsg,
	}
	s.sendToClient(conn, response)
}

// sendError sends an error response to the client.
func (s *Server) sendError(conn net.Conn, errorMsg string) {
	response := &ProtocolMessage{
		Type:  MsgTypeError,
		Error: errorMsg,
	}
	s.sendToClient(conn, response)
}

// sendToClient sends a message to a client.
func (s *Server) sendToClient(conn net.Conn, msg *ProtocolMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	length := uint32(len(data))
	header := []byte{
		byte(length >> 24),
		byte(length >> 16),
		byte(length >> 8),
		byte(length),
	}

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	if _, err := conn.Write(header); err != nil {
		return err
	}
	if _, err := conn.Write(data); err != nil {
		return err
	}

	return nil
}

// HTTP handlers for health and stats

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats := s.queue.GetStats()
	json.NewEncoder(w).Encode(stats)
}

// GetQueue returns the underlying queue (for testing).
func (s *Server) GetQueue() *InMemoryQueue {
	return s.queue
}
