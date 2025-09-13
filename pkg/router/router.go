package router

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	HEALTH = "/healthz"
	READY  = "/readyz"
	HTTP   = "/mcp/stream"
	SSE    = "/mcp/sse"
)

// RouterConfig configures the HTTP router that wraps MCP handlers.
type RouterConfig struct {
	// BasePath to mount the router under, e.g. "/api" (optional).
	BasePath string
	// StreamOptions passed to the MCP streamable HTTP handler (nil = defaults).
	StreamOptions *mcp.StreamableHTTPOptions
	// EnableSSE registers the SSE endpoint at <BasePath>/mcp/sse.
	EnableSSE bool
	// EnableStream registers the streamable HTTP endpoint at <BasePath>/mcp/stream.
	EnableStream bool
	McpName      string
	McpVersion   string
}

// NewRouter returns an http.Handler that mounts health, info, and MCP endpoints.
//
// Endpoints (relative to cfg.BasePath):
//
//	GET  /                 - basic info and available endpoints
//	GET  /healthz          - liveness probe ("ok")
//	GET  /readyz           - readiness probe ("ok")
//	GET  /mcp/sse          - MCP over Server-Sent Events (if EnableSSE)
//	POST /mcp/stream       - MCP streamable HTTP (if EnableStream)
//
// The MCP endpoints are provided by github.com/modelcontextprotocol/go-sdk/mcp.
func NewRouter(mcpServer *mcp.Server, logger *slog.Logger, cfg *RouterConfig) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg == nil {
		cfg = &RouterConfig{EnableStream: true}
	}

	mux := http.NewServeMux()

	// Utility to join base and path cleanly.
	join := func(base, path string) string {
		b := strings.TrimRight(base, "/")
		p := strings.TrimLeft(path, "/")
		if b == "" {
			return "/" + p
		}
		return b + "/" + p
	}

	// Health endpoints
	mux.Handle(join(cfg.BasePath, HEALTH), requestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})))
	mux.Handle(join(cfg.BasePath, READY), requestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})))

	// Root info endpoint: advertises available endpoints.
	// Only respond to exact match of the root path, not as a catch-all
	rootPath := join(cfg.BasePath, "/")
	mux.Handle(rootPath, requestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only handle exact path match
		if r.URL.Path != rootPath {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		type endpoints struct {
			Health string `json:"health"`
			Ready  string `json:"ready"`
			SSE    string `json:"sse,omitempty"`
			Stream string `json:"stream,omitempty"`
		}
		info := struct {
			Name      string    `json:"name"`
			Version   string    `json:"version"`
			Timestamp time.Time `json:"timestamp"`
			Endpoints endpoints `json:"endpoints"`
		}{
			Name:      cfg.McpName,
			Version:   cfg.McpVersion,
			Timestamp: time.Now().UTC(),
			Endpoints: endpoints{
				Health: join(cfg.BasePath, HEALTH),
				Ready:  join(cfg.BasePath, READY),
				SSE:    "",
				Stream: "",
			},
		}
		if cfg.EnableSSE {
			info.Endpoints.SSE = join(cfg.BasePath, SSE)
		}
		if cfg.EnableStream {
			info.Endpoints.Stream = join(cfg.BasePath, HTTP)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	})))

	// MCP handlers (mounted under /mcp/...)
	if cfg.EnableSSE {
		// SSE handler provided by the MCP SDK.
		sseHandler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return mcpServer })
		mux.Handle(join(cfg.BasePath, SSE), requestLogger(logger, sseHandler))
	}
	if cfg.EnableStream {
		// Streamable HTTP handler provided by the MCP SDK.
		streamHandler := mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return mcpServer },
			cfg.StreamOptions,
		)
		mux.Handle(join(cfg.BasePath, HTTP), requestLogger(logger, streamHandler))
	}

	// Return the mux directly - logging is already applied to individual handlers
	return mux
}

// requestLogger is a lightweight HTTP middleware that logs request/response details.
func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(lw, r)
		logger.Info("http_request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", lw.status),
			slog.Int64("bytes", lw.bytes),
			slog.String("remote", r.RemoteAddr),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.status = code
	lw.ResponseWriter.WriteHeader(code)
}

func (lw *loggingResponseWriter) Write(b []byte) (int, error) {
	n, err := lw.ResponseWriter.Write(b)
	lw.bytes += int64(n)
	return n, err
}
