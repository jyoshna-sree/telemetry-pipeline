# Collector Dockerfile
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
    -o /collector \
    ./cmd/collector

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

# Copy binary from builder
COPY --from=builder /collector /usr/local/bin/collector

# Switch to non-root user
USER appuser

# Default environment variables
ENV MQ_HOST=mq-server
ENV MQ_PORT=9000
ENV MQ_TOPIC=telemetry.metrics
ENV STORAGE_TYPE=memory
ENV RETENTION_PERIOD=120h

# Run the collector
ENTRYPOINT ["collector"]
