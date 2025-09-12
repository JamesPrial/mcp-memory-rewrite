# Build stage - Use Debian for compatibility with runtime
FROM golang:1.23-bookworm AS builder

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build with FTS5 support enabled
# The sqlite_fts5 build tag is required to enable FTS5
RUN CGO_ENABLED=1 \
    GOOS=linux \
    go build -a \
    -tags "sqlite_fts5" \
    -o mcp-memory-server ./cmd/mcp-memory-server

# Runtime stage - Use Debian for better SQLite support
FROM debian:bookworm-slim

# Install runtime dependencies
# Only need ca-certificates since SQLite is statically linked
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -g 1000 mcp && \
    useradd -r -u 1000 -g mcp mcp

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