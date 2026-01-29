# 🚀 The Journey of 100 GPU Metrics Through the Telemetry Pipeline

This document traces how 100 GPU metrics flow from a CSV file to InfluxDB through our message queue system.

---

## 📊 Overview

```
CSV File → Streamer → MQ Server → Collector → InfluxDB
(100 rows)   (1 batch)  (1 message)  (1 batch)   (100 points)
```

---

## 🎬 Step-by-Step Journey

### **Step 1: Streamer Reads CSV File**

**Location:** `cmd/streamer/main.go`

The streamer reads 100 rows from the CSV file:

```go
// Read 100 metrics from CSV
metrics := []models.GPUMetric{
    {GPUID: "GPU-12345", Temperature: 75.5, PowerDraw: 250.0, ...},
    {GPUID: "GPU-12345", Temperature: 76.0, PowerDraw: 252.0, ...},
    // ... 98 more metrics
}
```

**What we have:** 100 individual GPU metric structs in memory

---

### **Step 2: Streamer Creates a Batch**

**Location:** `cmd/streamer/main.go`

The streamer groups all 100 metrics into a single batch:

```go
batch := models.MetricBatch{
    BatchID:     "batch-abc-123",
    Source:      "streamer-1",
    CollectedAt: time.Now(),
    Metrics:     metrics,  // ← All 100 metrics
}
```

**What we have:** 1 batch object containing 100 metrics

---

### **Step 3: Streamer Serializes to JSON**

**Location:** `cmd/streamer/main.go`

The batch is converted to JSON:

```go
payload, _ := json.Marshal(batch)
```

**Result:**
```json
{
  "batch_id": "batch-abc-123",
  "source": "streamer-1",
  "collected_at": "2024-01-29T10:30:00Z",
  "metrics": [
    {"gpu_id": "GPU-12345", "temperature": 75.5, "power_draw": 250.0, ...},
    {"gpu_id": "GPU-12345", "temperature": 76.0, "power_draw": 252.0, ...},
    ... (100 metric objects)
  ]
}
```

**What we have:** 1 JSON string (~50KB) as bytes

---

### **Step 4: Streamer Publishes to MQ**

**Location:** `cmd/streamer/main.go` → `internal/mq/client.go`

The streamer sends the JSON payload to the MQ server:

```go
client.Publish(ctx, payload)  // ← Sends 1 message
```

**What's sent:** 1 message over TCP containing the JSON payload

---

### **Step 5: MQ Server Stores Message**

**Location:** `internal/mq/queue.go`

The MQ server stores the message in its in-memory log:

```go
func (q *InMemoryQueue) Publish(ctx context.Context, payload []byte) error {
    msg := NewMessage(payload)
    
    q.logMu.Lock()
    q.log = append(q.log, msg)  // ← Store 1 message
    q.logMu.Unlock()
    
    q.notifySubscribers()  // ← Notify collectors
    return nil
}
```

**Queue state:**
```
q.log = [
    Message{
        ID:        "uuid-abc-123",
        Offset:    0,
        Payload:   []byte(`{"batch_id":"...","metrics":[...100 items...]}`),
        Timestamp: 2024-01-29T10:30:00Z,
    }
]
```

**What's stored:** 1 log entry with the entire JSON as raw bytes

---

### **Step 6: MQ Server Notifies Subscribers**

**Location:** `internal/mq/queue.go`

The MQ server sends a signal to all subscribers (collectors):

```go
func (q *InMemoryQueue) notifySubscribers() {
    for _, sub := range q.subscribers {
        select {
        case sub.notify <- struct{}{}:  // ← Send signal
        default:
        }
    }
}
```

**What's sent:** 1 empty signal (not the data itself, just a "wake up" notification)

---

### **Step 7: Collector Receives Signal**

**Location:** `internal/mq/queue.go`

The collector's goroutine wakes up:

```go
func (q *InMemoryQueue) consumeLoop(sub *subscriber) {
    for {
        select {
        case <-sub.notify:  // ← Receives signal
            q.processMessages(sub)
        }
    }
}
```

**What happens:** Collector wakes up and starts processing

---

### **Step 8: Collector Reads Message**

**Location:** `internal/mq/queue.go`

The collector reads the message from the queue log:

```go
func (q *InMemoryQueue) processMessages(sub *subscriber) {
    msg := q.getMessageAtOffset(sub.offset)  // ← Read message at offset 0
    sub.handler(q.ctx, msg)  // ← Call collector's handler
    sub.offset++
}
```

**What's read:** 1 message containing the JSON payload

---

### **Step 9: Collector Parses JSON**

**Location:** `cmd/collector/main.go`

The collector unmarshals the JSON back into a Go struct:

```go
func (c *Collector) handleMessage(ctx context.Context, msg *mq.Message) error {
    var batch models.MetricBatch
    json.Unmarshal(msg.Payload, &batch)  // ← Parse JSON
    
    // batch.Metrics now contains 100 GPUMetric structs
}
```

**What we have:** 1 `MetricBatch` struct with 100 `GPUMetric` structs inside

---

### **Step 10: Collector Extracts Metrics**

**Location:** `cmd/collector/main.go`

The collector extracts the metrics from the batch:

```go
metrics := make([]*models.GPUMetric, len(batch.Metrics))
for i := range batch.Metrics {
    metrics[i] = &batch.Metrics[i]  // ← Get pointer to each metric
}
```

**What we have:** 100 pointers to individual GPU metrics

---

### **Step 11: Collector Stores to InfluxDB**

**Location:** `cmd/collector/main.go` → `internal/storage/influxdb.go`

The collector writes all metrics to InfluxDB in one batch:

```go
c.store.StoreBatch(ctx, metrics)  // ← Write all 100 metrics
```

**What's written:** 100 data points in InfluxDB

---

## 📊 Summary Table

| Stage | Component | Data Format | Count | Size |
|-------|-----------|-------------|-------|------|
| 1. Read | Streamer | CSV rows | 100 rows | - |
| 2. Create | Streamer | Go struct `MetricBatch` | 1 batch | - |
| 3. Serialize | Streamer | JSON bytes | 1 payload | ~50KB |
| 4. Publish | Streamer → MQ | TCP message | 1 message | ~50KB |
| 5. Store | MQ Server | `Message` in log | 1 log entry | ~50KB |
| 6. Notify | MQ Server | Signal | 1 signal | 0 bytes |
| 7. Receive | Collector | Signal | 1 signal | 0 bytes |
| 8. Read | Collector | `Message` from log | 1 message | ~50KB |
| 9. Parse | Collector | Go struct `MetricBatch` | 1 batch | - |
| 10. Extract | Collector | `[]*GPUMetric` slice | 100 pointers | - |
| 11. Store | Collector → InfluxDB | Data points | 100 points | - |

---

## 🔑 Key Insights

### **1. Application-Level Batching**
- The streamer batches 100 metrics into 1 message
- The queue only sees 1 message (doesn't know about the 100 metrics inside)
- The collector unbatches the 1 message back into 100 metrics

### **2. Efficient Messaging**
- **1 message** instead of 100 separate messages
- **1 signal** instead of 100 notifications
- **1 network call** instead of 100 TCP sends

### **3. Decoupled Design**
- **Queue is payload-agnostic:** It just stores and delivers bytes
- **Applications are payload-aware:** They know how to serialize/deserialize
- **Queue doesn't parse:** No JSON parsing overhead in the message queue

### **4. Signal vs. Data**
- The **signal** is just a "wake up" notification (0 bytes)
- The **data** stays in the queue log until the collector reads it
- This allows the collector to process at its own pace

---

## 🎯 Performance Benefits

### **Compared to Sending 100 Individual Messages:**

| Metric | Individual Messages | Batched (Current) | Improvement |
|--------|---------------------|-------------------|-------------|
| **Messages in queue** | 100 | 1 | 100x fewer |
| **Signals sent** | 100 | 1 | 100x fewer |
| **Lock operations** | 100 | 1 | 100x fewer |
| **Network calls** | 100 | 1 | 100x fewer |
| **JSON parse operations** | 100 | 1 | 100x fewer |
| **Memory overhead** | 100 Message objects | 1 Message object | 100x less |

---

## 📈 Scalability

This design scales efficiently:

- **1,000 metrics:** Still 1 message, 1 signal
- **10,000 metrics:** Still 1 message, 1 signal
- **Multiple streamers:** Each sends their own batches independently
- **Multiple collectors:** Each processes messages at their own pace

---

## 🔄 Data Flow Diagram

```
┌─────────────────┐
│   CSV File      │
│  (100 rows)     │
└────────┬────────┘
         │
         ↓ Read
┌────────┴────────┐
│   Streamer      │
│  • Batch 100    │
│  • Marshal JSON │
│  • Publish      │
└────────┬────────┘
         │
         ↓ TCP (1 message)
┌────────┴────────┐
│   MQ Server     │
│  • Store in log │
│  • Send signal  │
└────────┬────────┘
         │
         ↓ Signal (0 bytes)
┌────────┴────────┐
│   Collector     │
│  • Read message │
│  • Parse JSON   │
│  • Extract 100  │
└────────┬────────┘
         │
         ↓ Batch write
┌────────┴────────┐
│   InfluxDB      │
│  (100 points)   │
└─────────────────┘
```

---

## 💡 Why This Design?

1. **Efficiency:** Batching reduces overhead at every layer
2. **Simplicity:** Queue is simple (just stores bytes)
3. **Flexibility:** Applications control batch size
4. **Performance:** Fewer locks, signals, and network calls
5. **Scalability:** Works well with high throughput

---

## 🔍 Message Queue Internals

### **Queue Structure**

The in-memory queue maintains:

```go
type InMemoryQueue struct {
    log         []*Message           // Append-only message log
    subscribers map[string]*subscriber  // Active subscribers
    // ... synchronization primitives
}
```

### **Message Structure**

Each message in the log contains:

```go
type Message struct {
    ID        string            // Unique message ID
    Offset    Offset            // Position in log (0, 1, 2, ...)
    Payload   []byte            // Raw bytes (JSON in our case)
    Timestamp time.Time         // When published
    Metadata  map[string]string // Optional metadata
}
```

### **Subscriber Structure**

Each subscriber tracks:

```go
type subscriber struct {
    id      string            // "collector-1"
    offset  Offset            // Current read position
    handler MessageHandler    // Function to call with messages
    notify  chan struct{}     // Signal channel (buffered, size 1)
}
```

### **How Offsets Work**

```
Message Log:
[msg0, msg1, msg2, msg3, msg4]
  ↑     ↑     ↑     ↑     ↑
  0     1     2     3     4  ← Offsets

Collector-1 offset: 2  (will read msg2 next)
Collector-2 offset: 4  (will read msg4 next)
```

Each subscriber maintains its own offset, allowing:
- **Independent consumption:** Collectors read at their own pace
- **Replay capability:** Can start from any offset (earliest, latest, or specific)
- **No message loss:** Messages stay in log until explicitly removed

---

## 🚦 Concurrency & Thread Safety

### **Synchronization Primitives**

The queue uses several mechanisms for thread safety:

1. **`sync.RWMutex` for log:** Allows multiple readers or one writer
2. **`sync.RWMutex` for subscribers:** Protects subscriber map
3. **`atomic.Bool` for running flag:** Thread-safe state check
4. **`sync.WaitGroup`:** Tracks active goroutines for graceful shutdown
5. **Buffered channels:** Non-blocking notifications

### **Why This Matters**

- **Multiple streamers** can publish concurrently
- **Multiple collectors** can read concurrently
- **No blocking:** Publishers never wait for slow consumers
- **Safe shutdown:** All goroutines exit cleanly

---

## 📝 Configuration

### **Batch Size**

Configured in the streamer:

```go
// cmd/streamer/main.go
batchSize := 100  // Number of metrics per batch
```

**Trade-offs:**
- **Larger batches:** More efficient, but higher latency
- **Smaller batches:** Lower latency, but more overhead

### **Queue Buffer**

Configured in the queue:

```go
// pkg/config/config.go
BufferSize: 10000  // Initial capacity for message log
```

**Note:** The log grows dynamically, so this is just the initial allocation.

---

## 🐛 Debugging Tips

### **Check Streamer Logs**

```bash
kubectl logs -n gpu-telemetry deployment/streamer
```

Look for:
```
Batch sent: 100 metrics (total: 5 batches, 500 metrics)
```

### **Check MQ Server Stats**

```bash
curl http://localhost:30080/api/v1/stats
```

Look for:
```json
{
  "mq_stats": {
    "total_messages": 5,
    "subscriber_count": 1
  }
}
```

### **Check Collector Logs**

```bash
kubectl logs -n gpu-telemetry deployment/collector
```

Look for:
```
Received batch with 100 metrics from streamer-1
Successfully stored 100 metrics to InfluxDB
```

### **Verify InfluxDB Data**

```bash
curl "http://localhost:30086/api/v2/query?org=gpu-telemetry" \
  -H "Authorization: Token your-token" \
  -d 'from(bucket:"gpu-metrics") |> range(start: -1h) |> count()'
```

---

## 🔗 Related Documentation

- **[README.md](README.md):** Project overview and setup instructions
- **[ARCHITECTURE.md](docs/ARCHITECTURE.md):** System architecture details
- **[API.md](docs/API.md):** API endpoint documentation
- **[Message Queue Design](docs/AI_DOCUMENTATION.md):** Detailed MQ implementation

---

**This architecture efficiently moves 100 metrics through the pipeline as a single unit, minimizing overhead while maintaining simplicity and scalability.** 🚀
