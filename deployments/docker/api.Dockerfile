# API Gateway Dockerfile
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
    -o /api \
    ./cmd/api

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

# Copy binary from builder
COPY --from=builder /api /usr/local/bin/api

# Switch to non-root user
USER appuser

# Expose HTTP port
EXPOSE 8080

# Default environment variables
ENV API_HOST=0.0.0.0
ENV API_PORT=8080
ENV ENABLE_SWAGGER=true
ENV DEFAULT_LIMIT=100
ENV MAX_LIMIT=1000

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Run the API server
ENTRYPOINT ["api"]
