# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o mcp-memory-server ./cmd/mcp-memory-server

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache sqlite

# Create non-root user
RUN addgroup -g 1000 mcp && \
    adduser -D -u 1000 -G mcp mcp

# Copy binary from builder
COPY --from=builder /app/mcp-memory-server /usr/local/bin/
RUN chmod +x /usr/local/bin/mcp-memory-server

# Create data directory with proper permissions
RUN mkdir -p /data && \
    chown -R mcp:mcp /data && \
    chmod 755 /data

# Switch to non-root user
USER mcp

# Ensure /data is writable
WORKDIR /data

# Set volume for persistent storage
VOLUME ["/data"]

# Set environment variable for database path
ENV MEMORY_DB_PATH=/data/memory.db

# Set the entrypoint
ENTRYPOINT ["mcp-memory-server"]