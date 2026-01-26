# Streamer Dockerfile
# Multi-stage build for minimal production image

# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=1.0.0" \
    -o /streamer \
    ./cmd/streamer

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

# Create data directory
RUN mkdir -p /data && chown appuser:appuser /data

# Copy binary from builder
COPY --from=builder /streamer /usr/local/bin/streamer

# Switch to non-root user
USER appuser

# Set working directory
WORKDIR /data

# Default environment variables
ENV CSV_PATH=/data/telemetry.csv
ENV MQ_HOST=mq-server
ENV MQ_PORT=9000
ENV BATCH_SIZE=100
ENV STREAM_INTERVAL=1s
ENV LOOP=true
ENV MQ_TOPIC=telemetry.metrics

# Run the streamer
ENTRYPOINT ["streamer"]
