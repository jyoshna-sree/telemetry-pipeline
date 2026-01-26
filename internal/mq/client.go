// Package mq provides a custom message queue client for connecting to the MQ server.
package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Client is a TCP-based client for the message queue server.
type Client struct {
	addr         string
	conn         net.Conn
	mu           sync.Mutex
	connected    atomic.Bool
	reconnect    bool
	timeout      time.Duration
	handler      MessageHandler
	handlerMu    sync.RWMutex
	startOffset  Offset // Saved for reconnection
	subscriberID string // Saved for reconnection
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// ClientConfig configures the MQ client.
type ClientConfig struct {
	Host           string        `json:"host"`
	Port           int           `json:"port"`
	Timeout        time.Duration `json:"timeout"`
	AutoReconnect  bool          `json:"auto_reconnect"`
	ReconnectDelay time.Duration `json:"reconnect_delay"`
}

// DefaultClientConfig returns a client config with sensible defaults.
func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Host:           "localhost",
		Port:           9000,
		Timeout:        10 * time.Second,
		AutoReconnect:  true,
		ReconnectDelay: 5 * time.Second,
	}
}

// NewClient creates a new MQ client.
func NewClient(config ClientConfig) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		addr:      fmt.Sprintf("%s:%d", config.Host, config.Port),
		reconnect: config.AutoReconnect,
		timeout:   config.Timeout,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Protocol message types for client-server communication.
const (
	MsgTypePublish     = "publish"
	MsgTypeSubscribe   = "subscribe"
	MsgTypeUnsubscribe = "unsubscribe"
	MsgTypeAck         = "ack"
	MsgTypeNack        = "nack"
	MsgTypeGetStats    = "get_stats"
	// MQ pushes data to Collector
	MsgTypeMessage  = "message"
	MsgTypeResponse = "response"
	MsgTypeError    = "error"
)

// ProtocolMessage is the wire format for client-server messages.
type ProtocolMessage struct {
	Type         string          `json:"type"`
	SubscriberID string          `json:"subscriber_id,omitempty"`
	MessageID    string          `json:"message_id,omitempty"`
	Offset       Offset          `json:"offset,omitempty"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	Error        string          `json:"error,omitempty"`
	Success      bool            `json:"success,omitempty"`
}

// Connect establishes a connection to the MQ server.
func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected.Load() {
		return nil
	}

	conn, err := net.DialTimeout("tcp", c.addr, c.timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to MQ server: %w", err)
	}

	c.conn = conn
	c.connected.Store(true)

	// Start message receiver
	c.wg.Add(1)
	go c.receiveLoop()

	return nil
}

// Close closes the connection to the MQ server.
func (c *Client) Close() error {
	c.cancel()
	c.connected.Store(false)

	c.mu.Lock()
	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		c.mu.Unlock()
		c.wg.Wait()
		return err
	}
	c.mu.Unlock()
	return nil
}

// IsConnected returns whether the client is connected.
func (c *Client) IsConnected() bool {
	return c.connected.Load()
}

// sendMessage sends a protocol message to the server.
func (c *Client) sendMessage(msg *ProtocolMessage) error {
	if !c.connected.Load() {
		return errors.New("not connected")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return errors.New("connection is nil")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write length-prefixed message
	length := uint32(len(data))
	header := []byte{
		byte(length >> 24),
		byte(length >> 16),
		byte(length >> 8),
		byte(length),
	}

	if err := c.conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}

	if _, err := c.conn.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	if _, err := c.conn.Write(data); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

// receiveLoop continuously reads messages from the server.
func (c *Client) receiveLoop() {
	defer c.wg.Done()

	header := make([]byte, 4)
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if !c.connected.Load() || c.conn == nil {
			return
		}

		// Read message length
		if err := c.conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
			continue
		}

		_, err := c.conn.Read(header)
		if err != nil {
			if c.reconnect && c.ctx.Err() == nil {
				c.handleReconnect()
			}
			continue
		}

		length := uint32(header[0])<<24 | uint32(header[1])<<16 | uint32(header[2])<<8 | uint32(header[3])
		if length > 10*1024*1024 { // 10MB max
			continue
		}

		data := make([]byte, length)
		if _, err := c.conn.Read(data); err != nil {
			continue
		}

		var msg ProtocolMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		c.handleMessage(&msg)
	}
}

// handleMessage processes incoming messages from the server.
func (c *Client) handleMessage(msg *ProtocolMessage) {
	if msg.Type == MsgTypeMessage {
		c.handlerMu.RLock()
		handler := c.handler
		c.handlerMu.RUnlock()

		if handler != nil {
			queueMsg := &Message{
				ID:        msg.MessageID,
				Payload:   msg.Payload,
				Timestamp: time.Now(),
			}

			go func() {
				if err := handler(c.ctx, queueMsg); err != nil {
					_ = c.Nack(msg.MessageID)
				} else {
					_ = c.Ack(msg.MessageID)
				}
			}()
		}
	}
}

// handleReconnect attempts to reconnect to the server.
func (c *Client) handleReconnect() {
	c.connected.Store(false)
	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	for c.ctx.Err() == nil {
		time.Sleep(5 * time.Second)
		if err := c.Connect(); err == nil {
			// Re-subscribe if we had a handler
			c.handlerMu.RLock()
			hasHandler := c.handler != nil
			subID := c.subscriberID
			offset := c.startOffset
			c.handlerMu.RUnlock()
			if hasHandler {
				_ = c.sendSubscribe(subID, offset)
			}
			return
		}
	}
}

// Publish publishes a message to the queue.
func (c *Client) Publish(ctx context.Context, payload []byte) error {
	msg := &ProtocolMessage{
		Type:    MsgTypePublish,
		Payload: payload,
	}
	return c.sendMessage(msg)
}

// PublishBatch publishes multiple messages to the queue.
func (c *Client) PublishBatch(ctx context.Context, payloads [][]byte) error {
	for _, payload := range payloads {
		if err := c.Publish(ctx, payload); err != nil {
			return err
		}
	}
	return nil
}

// Subscribe subscribes to the queue with the given handler.
// startOffset can be OffsetEarliest (-2), OffsetLatest (-1), or a specific offset.
func (c *Client) Subscribe(ctx context.Context, subscriberID string, startOffset Offset, handler MessageHandler) error {
	c.handlerMu.Lock()
	c.handler = handler
	c.startOffset = startOffset
	c.subscriberID = subscriberID
	c.handlerMu.Unlock()

	return c.sendSubscribe(subscriberID, startOffset)
}

// sendSubscribe sends a subscribe message to the server.
func (c *Client) sendSubscribe(subscriberID string, offset Offset) error {
	msg := &ProtocolMessage{
		Type:         MsgTypeSubscribe,
		SubscriberID: subscriberID,
		Offset:       offset,
	}
	return c.sendMessage(msg)
}

// Unsubscribe unsubscribes from the queue.
func (c *Client) Unsubscribe(subscriberID string) error {
	c.handlerMu.Lock()
	c.handler = nil
	c.handlerMu.Unlock()

	msg := &ProtocolMessage{
		Type:         MsgTypeUnsubscribe,
		SubscriberID: subscriberID,
	}
	return c.sendMessage(msg)
}

// Ack acknowledges a message.
func (c *Client) Ack(messageID string) error {
	msg := &ProtocolMessage{
		Type:      MsgTypeAck,
		MessageID: messageID,
	}
	return c.sendMessage(msg)
}

// Nack negatively acknowledges a message (triggers retry).
func (c *Client) Nack(messageID string) error {
	msg := &ProtocolMessage{
		Type:      MsgTypeNack,
		MessageID: messageID,
	}
	return c.sendMessage(msg)
}

// GetStats returns queue statistics (requires server response).
func (c *Client) GetStats() QueueStats {
	// This would require a synchronous request-response pattern
	// For now, return empty - implement with response channel if needed
	return QueueStats{}
}
