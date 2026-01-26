package mq

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewInMemoryQueue(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	if q == nil {
		t.Fatal("expected queue to be created")
	}
}

func TestPublishAndSubscribe(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	var received int64
	handler := func(ctx context.Context, msg *Message) error {
		atomic.AddInt64(&received, 1)
		return nil
	}

	if err := q.Subscribe(ctx, "test-sub", OffsetEarliest, handler); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Publish messages
	for i := 0; i < 10; i++ {
		if err := q.Publish(ctx, []byte("test message")); err != nil {
			t.Fatalf("failed to publish: %v", err)
		}
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt64(&received) != 10 {
		t.Errorf("expected 10 messages, got %d", received)
	}
}

func TestPublishBatch(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	payloads := [][]byte{
		[]byte("msg1"),
		[]byte("msg2"),
		[]byte("msg3"),
	}

	if err := q.PublishBatch(ctx, payloads); err != nil {
		t.Fatalf("failed to publish batch: %v", err)
	}

	stats := q.GetStats()
	if stats.TotalMessages != 3 {
		t.Errorf("expected 3 total messages, got %d", stats.TotalMessages)
	}
}

func TestQueueStats(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	stats := q.GetStats()
	if stats.TotalMessages != 0 {
		t.Errorf("expected 0 total messages, got %d", stats.TotalMessages)
	}
}

func TestUnsubscribe(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	handler := func(ctx context.Context, msg *Message) error {
		return nil
	}

	if err := q.Subscribe(ctx, "test-sub", OffsetLatest, handler); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	stats := q.GetStats()
	if stats.SubscriberCount != 1 {
		t.Errorf("expected 1 subscriber, got %d", stats.SubscriberCount)
	}

	if err := q.Unsubscribe("test-sub"); err != nil {
		t.Fatalf("failed to unsubscribe: %v", err)
	}

	stats = q.GetStats()
	if stats.SubscriberCount != 0 {
		t.Errorf("expected 0 subscribers, got %d", stats.SubscriberCount)
	}
}

func TestQueueShutdown(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}

	if err := q.Shutdown(ctx); err != nil {
		t.Fatalf("failed to shutdown: %v", err)
	}

	// Publish after shutdown should fail
	err := q.Publish(ctx, []byte("test"))
	if err != ErrQueueShutdown {
		t.Errorf("expected ErrQueueShutdown, got %v", err)
	}
}

func TestMessage(t *testing.T) {
	msg := NewMessage([]byte("test payload"))

	if msg.ID == "" {
		t.Error("expected message ID")
	}
	if string(msg.Payload) != "test payload" {
		t.Error("expected payload to match")
	}
	if msg.Timestamp.IsZero() {
		t.Error("expected timestamp")
	}
}

func TestMessageClone(t *testing.T) {
	msg := NewMessage([]byte("test"))
	msg.Metadata["key"] = "value"

	clone := msg.Clone()

	if clone.ID != msg.ID {
		t.Error("clone ID mismatch")
	}
	if string(clone.Payload) != string(msg.Payload) {
		t.Error("clone payload mismatch")
	}
	if clone.Metadata["key"] != "value" {
		t.Error("clone metadata mismatch")
	}

	// Modify clone shouldn't affect original
	clone.Metadata["key"] = "modified"
	if msg.Metadata["key"] != "value" {
		t.Error("modifying clone affected original")
	}
}

func TestMessageJSON(t *testing.T) {
	msg := NewMessage([]byte("test"))
	msg.Metadata["key"] = "value"

	data, err := msg.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize: %v", err)
	}

	var decoded Message
	if err := decoded.FromJSON(data); err != nil {
		t.Fatalf("failed to deserialize: %v", err)
	}

	if decoded.ID != msg.ID {
		t.Error("decoded ID mismatch")
	}
}

func TestDefaultQueueConfig(t *testing.T) {
	cfg := DefaultQueueConfig()

	if cfg.BufferSize <= 0 {
		t.Error("expected positive buffer size")
	}
	if cfg.PublishTimeout <= 0 {
		t.Error("expected positive publish timeout")
	}
}

func TestDuplicateSubscriber(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	handler := func(ctx context.Context, msg *Message) error {
		return nil
	}

	if err := q.Subscribe(ctx, "test-sub", OffsetLatest, handler); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	err := q.Subscribe(ctx, "test-sub", OffsetLatest, handler)
	if err != ErrSubscriberExists {
		t.Errorf("expected ErrSubscriberExists, got %v", err)
	}
}

func TestUnsubscribeNonExistent(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	err := q.Unsubscribe("non-existent")
	if err != ErrSubscriberNotFound {
		t.Errorf("expected ErrSubscriberNotFound, got %v", err)
	}
}

func TestGetSubscriberOffset(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	handler := func(ctx context.Context, msg *Message) error {
		return nil
	}

	if err := q.Subscribe(ctx, "test-sub", OffsetLatest, handler); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	offset, err := q.GetSubscriberOffset("test-sub")
	if err != nil {
		t.Fatalf("failed to get offset: %v", err)
	}
	if offset < 0 {
		t.Error("expected non-negative offset")
	}

	// Non-existent subscriber
	_, err = q.GetSubscriberOffset("non-existent")
	if err != ErrSubscriberNotFound {
		t.Errorf("expected ErrSubscriberNotFound, got %v", err)
	}
}

func TestSetSubscriberOffset(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	handler := func(ctx context.Context, msg *Message) error {
		return nil
	}

	if err := q.Subscribe(ctx, "test-sub", OffsetLatest, handler); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Set valid offset
	err := q.SetSubscriberOffset("test-sub", 0)
	if err != nil {
		t.Fatalf("failed to set offset: %v", err)
	}

	// Non-existent subscriber
	err = q.SetSubscriberOffset("non-existent", 0)
	if err != ErrSubscriberNotFound {
		t.Errorf("expected ErrSubscriberNotFound, got %v", err)
	}
}

func TestGetLatestOffset(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	// Empty queue
	offset := q.GetLatestOffset()
	if offset != 0 {
		t.Errorf("expected 0 for empty queue, got %d", offset)
	}

	// After publishing
	q.Publish(ctx, []byte("test"))
	offset = q.GetLatestOffset()
	if offset != 0 {
		t.Errorf("expected 0 (first message), got %d", offset)
	}
}

func TestGetOldestOffset(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	offset := q.GetOldestOffset()
	if offset != 0 {
		t.Errorf("expected 0, got %d", offset)
	}
}

func TestQueueLen(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	if q.Len() != 0 {
		t.Error("expected empty queue")
	}

	q.Publish(ctx, []byte("test1"))
	q.Publish(ctx, []byte("test2"))

	if q.Len() != 2 {
		t.Errorf("expected 2 messages, got %d", q.Len())
	}
}

func TestOffsetEarliest(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	// Publish messages before subscribing
	for i := 0; i < 5; i++ {
		q.Publish(ctx, []byte("test"))
	}

	var received int64
	handler := func(ctx context.Context, msg *Message) error {
		atomic.AddInt64(&received, 1)
		return nil
	}

	// Subscribe from earliest - should get all 5
	if err := q.Subscribe(ctx, "test-sub", OffsetEarliest, handler); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	// Trigger processing by notifying
	q.notifySubscribers()

	time.Sleep(200 * time.Millisecond)

	count := atomic.LoadInt64(&received)
	if count != 5 {
		t.Errorf("expected 5 messages from earliest, got %d", count)
	}
}

func TestOffsetLatest(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	// Publish messages before subscribing
	for i := 0; i < 5; i++ {
		q.Publish(ctx, []byte("old"))
	}

	var received int64
	handler := func(ctx context.Context, msg *Message) error {
		atomic.AddInt64(&received, 1)
		return nil
	}

	// Subscribe from latest - should NOT get old messages
	if err := q.Subscribe(ctx, "test-sub", OffsetLatest, handler); err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt64(&received) != 0 {
		t.Errorf("expected 0 messages from latest (before new publish), got %d", received)
	}

	// Now publish new messages
	for i := 0; i < 3; i++ {
		q.Publish(ctx, []byte("new"))
	}

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt64(&received) != 3 {
		t.Errorf("expected 3 new messages, got %d", received)
	}
}

func TestPublishBatchAfterShutdown(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	q.Shutdown(ctx)

	err := q.PublishBatch(ctx, [][]byte{[]byte("test")})
	if err != ErrQueueShutdown {
		t.Errorf("expected ErrQueueShutdown, got %v", err)
	}
}

func TestMultipleSubscribers(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	var received1, received2 int64

	handler1 := func(ctx context.Context, msg *Message) error {
		atomic.AddInt64(&received1, 1)
		return nil
	}

	handler2 := func(ctx context.Context, msg *Message) error {
		atomic.AddInt64(&received2, 1)
		return nil
	}

	q.Subscribe(ctx, "sub1", OffsetEarliest, handler1)
	q.Subscribe(ctx, "sub2", OffsetEarliest, handler2)

	for i := 0; i < 10; i++ {
		q.Publish(ctx, []byte("test"))
	}

	time.Sleep(100 * time.Millisecond)

	// Both subscribers should receive all messages
	if atomic.LoadInt64(&received1) != 10 {
		t.Errorf("subscriber 1 expected 10, got %d", received1)
	}
	if atomic.LoadInt64(&received2) != 10 {
		t.Errorf("subscriber 2 expected 10, got %d", received2)
	}
}

func TestQueueStatsWithSubscribers(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	handler := func(ctx context.Context, msg *Message) error {
		return nil
	}

	q.Subscribe(ctx, "sub1", OffsetLatest, handler)
	q.Subscribe(ctx, "sub2", OffsetLatest, handler)

	stats := q.GetStats()
	if stats.SubscriberCount != 2 {
		t.Errorf("expected 2 subscribers, got %d", stats.SubscriberCount)
	}
	if len(stats.Subscribers) != 2 {
		t.Errorf("expected 2 subscriber infos, got %d", len(stats.Subscribers))
	}
}

func TestResolveOffsetClamping(t *testing.T) {
	q := NewInMemoryQueue(DefaultQueueConfig())
	ctx := context.Background()

	if err := q.Start(ctx); err != nil {
		t.Fatalf("failed to start queue: %v", err)
	}
	defer q.Shutdown(ctx)

	// Publish some messages
	for i := 0; i < 5; i++ {
		q.Publish(ctx, []byte("test"))
	}

	// Test negative offset gets clamped to 0
	resolved := q.resolveOffset(-100)
	if resolved != 0 {
		t.Errorf("expected negative offset clamped to 0, got %d", resolved)
	}

	// Test offset beyond log length gets clamped
	resolved = q.resolveOffset(1000)
	if resolved != 5 {
		t.Errorf("expected offset clamped to 5, got %d", resolved)
	}

	// Test valid offset stays the same
	resolved = q.resolveOffset(3)
	if resolved != 3 {
		t.Errorf("expected offset 3, got %d", resolved)
	}
}
