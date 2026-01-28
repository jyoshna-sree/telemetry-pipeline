// Package mq provides a log-based message queue implementation
// where multiple consumers can read all messages independently using offsets.
package mq

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Common errors returned by the message queue.
var (
	ErrQueueFull          = errors.New("queue is full")
	ErrPublishTimeout     = errors.New("publish timeout")
	ErrQueueShutdown      = errors.New("queue is shutting down")
	ErrInvalidConfig      = errors.New("invalid configuration")
	ErrInvalidOffset      = errors.New("invalid offset")
	ErrSubscriberExists   = errors.New("subscriber already exists")
	ErrSubscriberNotFound = errors.New("subscriber not found")
)

// Offset represents a position in the message log.
type Offset int64

const (
	// OffsetEarliest starts reading from the beginning of the log.
	OffsetEarliest Offset = -2
	// OffsetLatest starts reading from new messages only.
	OffsetLatest Offset = -1
)

// Message represents a message in the queue.
type Message struct {
	ID        string            `json:"id"`
	Offset    Offset            `json:"offset"`
	Payload   []byte            `json:"payload"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// NewMessage creates a new message with the given payload.
func NewMessage(payload []byte) *Message {
	return &Message{
		ID:        uuid.New().String(),
		Payload:   payload,
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}
}

// Clone creates a deep copy of the message.
func (m *Message) Clone() *Message {
	clone := &Message{
		ID:        m.ID,
		Offset:    m.Offset,
		Payload:   make([]byte, len(m.Payload)),
		Timestamp: m.Timestamp,
		Metadata:  make(map[string]string),
	}
	copy(clone.Payload, m.Payload)
	for k, v := range m.Metadata {
		clone.Metadata[k] = v
	}
	return clone
}

// ToJSON serializes the message to JSON.
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON deserializes a message from JSON.
func (m *Message) FromJSON(data []byte) error {
	return json.Unmarshal(data, m)
}

// MessageHandler is a function that processes messages.
type MessageHandler func(ctx context.Context, msg *Message) error

// QueueStats provides statistics about the queue.
type QueueStats struct {
	TotalMessages   int64            `json:"total_messages"`
	OldestOffset    Offset           `json:"oldest_offset"`
	LatestOffset    Offset           `json:"latest_offset"`
	SubscriberCount int              `json:"subscriber_count"`
	Subscribers     []SubscriberInfo `json:"subscribers"`
}

// SubscriberInfo contains info about a subscriber's position.
type SubscriberInfo struct {
	ID            string `json:"id"`
	CurrentOffset Offset `json:"current_offset"`
	Lag           int64  `json:"lag"` // How far behind latest
}

// QueueConfig configures the queue behavior.
type QueueConfig struct {
	BufferSize     int           `json:"buffer_size"` // Initial capacity (grows dynamically)
	PublishTimeout time.Duration `json:"publish_timeout"`
	MaxRetries     int           `json:"max_retries"`
	RetryDelay     time.Duration `json:"retry_delay"`
}

// DefaultQueueConfig returns a queue config with sensible defaults.
func DefaultQueueConfig() QueueConfig {
	return QueueConfig{
		BufferSize:     10000, // Initial capacity
		PublishTimeout: 5 * time.Second,
		MaxRetries:     3,
		RetryDelay:     time.Second,
	}
}

// subscriber tracks a consumer's offset and notification channel.
type subscriber struct {
	id      string
	offset  Offset // Current read position
	handler MessageHandler
	notify  chan struct{} // Signaled when new messages arrive
}

// InMemoryQueue is a log-based in-memory queue.
// Messages are stored in an append-only log that grows dynamically.
// Multiple consumers can read independently using offsets.
type InMemoryQueue struct {
	// Message log - append-only, grows dynamically
	log   []*Message
	logMu sync.RWMutex

	// Subscribers - each tracks their own offset
	subscribers map[string]*subscriber
	subMu       sync.RWMutex

	config  QueueConfig
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running atomic.Bool

	// Stats
	totalPublished int64
}

// NewInMemoryQueue creates a new log-based in-memory queue.
func NewInMemoryQueue(config QueueConfig) *InMemoryQueue {
	if config.BufferSize <= 0 {
		config.BufferSize = 10000
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &InMemoryQueue{
		log:         make([]*Message, 0, config.BufferSize),
		subscribers: make(map[string]*subscriber),
		config:      config,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the queue processing.
func (q *InMemoryQueue) Start(ctx context.Context) error {
	if q.running.Load() {
		return nil
	}
	q.running.Store(true)
	return nil
}

// Shutdown gracefully shuts down the queue.
func (q *InMemoryQueue) Shutdown(ctx context.Context) error {
	if !q.running.Load() {
		return nil
	}
	q.running.Store(false)
	q.cancel()

	// Close all subscriber notify channels
	q.subMu.Lock()
	for _, sub := range q.subscribers {
		close(sub.notify)
	}
	q.subMu.Unlock()

	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Publish publishes a message to the queue.
func (q *InMemoryQueue) Publish(ctx context.Context, payload []byte) error {
	if !q.running.Load() {
		return ErrQueueShutdown
	}

	msg := NewMessage(payload)

	q.logMu.Lock()
	// Offset = index in the log
	msg.Offset = Offset(len(q.log))
	q.log = append(q.log, msg)
	q.logMu.Unlock()

	atomic.AddInt64(&q.totalPublished, 1)

	// Notify all subscribers that new data is available
	q.notifySubscribers()

	return nil
}

// PublishBatch publishes multiple messages to the queue.
func (q *InMemoryQueue) PublishBatch(ctx context.Context, payloads [][]byte) error {
	if !q.running.Load() {
		return ErrQueueShutdown
	}

	q.logMu.Lock()
	for _, payload := range payloads {
		msg := NewMessage(payload)
		msg.Offset = Offset(len(q.log))
		q.log = append(q.log, msg)
	}
	q.logMu.Unlock()

	atomic.AddInt64(&q.totalPublished, int64(len(payloads)))
	q.notifySubscribers()

	return nil
}

// notifySubscribers signals all subscribers that new messages are available.
func (q *InMemoryQueue) notifySubscribers() {
	q.subMu.RLock()
	defer q.subMu.RUnlock()

	for _, sub := range q.subscribers {
		select {
		case sub.notify <- struct{}{}:
		default:
			// Already has pending notification
		}
	}
}

// Subscribe creates a subscriber that starts reading from the specified offset.
// Use OffsetEarliest to start from the beginning, OffsetLatest for new messages only,
// or a specific offset to resume from a saved position.
func (q *InMemoryQueue) Subscribe(ctx context.Context, subscriberID string, startOffset Offset, handler MessageHandler) error {
	q.subMu.Lock()
	defer q.subMu.Unlock()

	if _, exists := q.subscribers[subscriberID]; exists {
		return ErrSubscriberExists
	}

	// Resolve special offsets
	actualOffset := q.resolveOffset(startOffset)

	sub := &subscriber{
		id:      subscriberID,
		offset:  actualOffset,
		handler: handler,
		notify:  make(chan struct{}, 1),
	}

	q.subscribers[subscriberID] = sub

	// Start consumer goroutine for this subscriber
	q.wg.Add(1)
	select {
	case sub.notify <- struct{}{}:
	default:
	}
	go q.consumeLoop(sub)

	return nil
}

// resolveOffset converts special offsets to actual values.
func (q *InMemoryQueue) resolveOffset(offset Offset) Offset {
	q.logMu.RLock()
	defer q.logMu.RUnlock()

	switch offset {
	case OffsetEarliest:
		return 0 // Start from the beginning
	case OffsetLatest:
		return Offset(len(q.log)) // Start from next new message
	default:
		// Clamp to valid range
		if offset < 0 {
			return 0
		}
		if offset > Offset(len(q.log)) {
			return Offset(len(q.log))
		}
		return offset
	}
}

// consumeLoop processes messages for a single subscriber.
func (q *InMemoryQueue) consumeLoop(sub *subscriber) {
	defer q.wg.Done()

	for {
		select {
		case <-q.ctx.Done():
			return
		case _, ok := <-sub.notify:
			if !ok {
				return // Channel closed, shutting down
			}
			q.processMessages(sub)
		}
	}
}

// processMessages delivers available messages to a subscriber.
func (q *InMemoryQueue) processMessages(sub *subscriber) {
	for {
		msg := q.getMessageAtOffset(sub.offset)
		if msg == nil {
			return // No more messages available
		}

		// Deliver message to handler
		err := sub.handler(q.ctx, msg)
		if err != nil {
			// Handler failed - could implement retry logic here
			// For now, we'll skip and continue to allow progress
		}

		// Advance offset
		q.subMu.Lock()
		sub.offset++
		q.subMu.Unlock()
	}
}

// getMessageAtOffset returns the message at the given offset, or nil if not available.
func (q *InMemoryQueue) getMessageAtOffset(offset Offset) *Message {
	q.logMu.RLock()
	defer q.logMu.RUnlock()

	idx := int(offset)
	if idx < 0 || idx >= len(q.log) {
		return nil
	}

	return q.log[idx].Clone()
}

// Unsubscribe removes a subscriber.
func (q *InMemoryQueue) Unsubscribe(subscriberID string) error {
	q.subMu.Lock()
	defer q.subMu.Unlock()

	sub, exists := q.subscribers[subscriberID]
	if !exists {
		return ErrSubscriberNotFound
	}

	close(sub.notify)
	delete(q.subscribers, subscriberID)
	return nil
}

// GetSubscriberOffset returns the current offset for a subscriber.
func (q *InMemoryQueue) GetSubscriberOffset(subscriberID string) (Offset, error) {
	q.subMu.RLock()
	defer q.subMu.RUnlock()

	sub, exists := q.subscribers[subscriberID]
	if !exists {
		return 0, ErrSubscriberNotFound
	}
	return sub.offset, nil
}

// SetSubscriberOffset manually sets a subscriber's offset (for seeking).
func (q *InMemoryQueue) SetSubscriberOffset(subscriberID string, offset Offset) error {
	q.subMu.Lock()
	defer q.subMu.Unlock()

	sub, exists := q.subscribers[subscriberID]
	if !exists {
		return ErrSubscriberNotFound
	}

	// Clamp to valid range
	q.logMu.RLock()
	maxOffset := Offset(len(q.log))
	q.logMu.RUnlock()

	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	sub.offset = offset

	// Notify to process from new position
	select {
	case sub.notify <- struct{}{}:
	default:
	}

	return nil
}

// GetStats returns queue statistics.
func (q *InMemoryQueue) GetStats() QueueStats {
	q.logMu.RLock()
	logLen := len(q.log)
	var oldest, latest Offset
	if logLen > 0 {
		oldest = 0
		latest = Offset(logLen - 1)
	}
	q.logMu.RUnlock()

	q.subMu.RLock()
	subs := make([]SubscriberInfo, 0, len(q.subscribers))
	for _, sub := range q.subscribers {
		lag := int64(latest) - int64(sub.offset)
		if lag < 0 {
			lag = 0
		}
		subs = append(subs, SubscriberInfo{
			ID:            sub.id,
			CurrentOffset: sub.offset,
			Lag:           lag,
		})
	}
	subCount := len(q.subscribers)
	q.subMu.RUnlock()

	return QueueStats{
		TotalMessages:   atomic.LoadInt64(&q.totalPublished),
		OldestOffset:    oldest,
		LatestOffset:    latest,
		SubscriberCount: subCount,
		Subscribers:     subs,
	}
}

// GetLatestOffset returns the offset of the most recent message.
func (q *InMemoryQueue) GetLatestOffset() Offset {
	q.logMu.RLock()
	defer q.logMu.RUnlock()
	if len(q.log) == 0 {
		return 0
	}
	return Offset(len(q.log) - 1)
}

// GetOldestOffset returns the offset of the oldest message (always 0).
func (q *InMemoryQueue) GetOldestOffset() Offset {
	return 0
}

// Len returns the number of messages in the log.
func (q *InMemoryQueue) Len() int {
	q.logMu.RLock()
	defer q.logMu.RUnlock()
	return len(q.log)
}
