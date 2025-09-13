package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jamesprial/mcp-memory-rewrite/internal/config"
	"github.com/jamesprial/mcp-memory-rewrite/internal/logging"
	"github.com/jamesprial/mcp-memory-rewrite/pkg/database"
	"github.com/jamesprial/mcp-memory-rewrite/pkg/router"
	"github.com/jamesprial/mcp-memory-rewrite/pkg/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	MCP_NAME              = "mcp-memory-server"
	VERSION               = "1.0.0"
	FLAG_HTTP             = "http"
	FLAG_HTTP_DEFAULT     = ""
	FLAG_SSE              = "sse"
	FLAG_SSE_DEFAULT      = false
	FLAG_PORTFILE         = "portfile"
	FLAG_PORTFILE_DEFAULT = ""
)

var (
	httpAddr = flag.String("http", "", "HTTP address to listen on (e.g., :8080). If not set, uses stdio")
	sseMode  = flag.Bool("sse", false, "Use SSE (Server-Sent Events) for HTTP mode")
	portFile = flag.String("portfile", "", "If set with -http, write the actual bound TCP port to this file")
)

func main() {
	flag.Parse()

	logLevel := logging.GetLogLevel()
	logger := logging.NewLogger(MCP_NAME, logLevel)
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("application exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	logger.Info("graceful shutdown complete")
}

func run(logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Log startup information
	logger.Info("starting MCP memory server",
		slog.String("version", VERSION),
		slog.String("log_level", logging.GetLogLevel().String()),
	)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration",
			slog.String("error", err.Error()),
		)
		return err
	}

	logger.Info("configuration loaded",
		slog.String("db_path", cfg.DBPath),
	)

	// Initialize database with logging
	dbLogger := logger.With(slog.String("component", "database"))
	db, err := database.NewDBWithLogger(cfg.DBPath, dbLogger)
	if err != nil {
		logger.Error("failed to initialize database",
			slog.String("error", err.Error()),
			slog.String("path", cfg.DBPath),
		)
		return err
	}

	// Create the server with logger
	srvLogger := logger.With(slog.String("component", "server"))
	srv := server.NewServerWithLogger(db, srvLogger)

	// Create MCP server
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    MCP_NAME,
			Version: VERSION,
		},
		nil,
	)

	// Register all tools
	srv.RegisterTools(mcpServer)

	// Channel to listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Channel to signal when server is done
	done := make(chan error, 1)
	var httpServer *http.Server

	// Start the appropriate server based on flags
	if *httpAddr != "" {
		var err error
		httpServer, err = startHTTPServer(logger, mcpServer, done)
		if err != nil {
			return err
		}
	} else {
		startStdioServer(ctx, logger, mcpServer, done)
	}

	// Wait for either server error or interrupt signal
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("server stopped with error: %w", err)
		}
		logger.Info("server stopped cleanly")
	case sig := <-sigChan:
		logger.Info("received signal, shutting down gracefully",
			slog.String("signal", sig.String()),
		)
	}

	// Perform graceful shutdown
	shutdown(logger, httpServer, srv)

	return nil
}

func shutdown(logger *slog.Logger, httpServer *http.Server, srv *server.Server) {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if httpServer != nil {
		logger.Info("shutting down HTTP server...")
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP server shutdown error", slog.String("error", err.Error()))
		}
	}

	logger.Info("shutting down application server...")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("application server shutdown error", slog.String("error", err.Error()))
	}

}

func startHTTPServer(logger *slog.Logger, mcpServer *mcp.Server, done chan<- error) (*http.Server, error) {
	routerCfg := &router.RouterConfig{
		EnableSSE:    *sseMode,
		EnableStream: true, // Always enable stream endpoint in HTTP mode
		McpName:      MCP_NAME,
		McpVersion:   VERSION,
	}
	handler := router.NewRouter(mcpServer, logger, routerCfg)
	httpServer := &http.Server{Addr: *httpAddr, Handler: handler}

	ln, err := net.Listen("tcp", httpServer.Addr)
	if err != nil {
		return nil, fmt.Errorf("HTTP listen error: %w", err)
	}

	if *portFile != "" {
		addr := ln.Addr().(*net.TCPAddr)
		if err := os.WriteFile(*portFile, []byte(fmt.Sprintf("%d", addr.Port)), 0644); err != nil {
			logger.Warn("failed writing portfile", slog.String("error", err.Error()), slog.String("file", *portFile))
		} else {
			logger.Info("wrote port to file", slog.Int("port", addr.Port), slog.String("file", *portFile))
		}
	}

	go func() {
		logger.Info("starting HTTP server", slog.Bool("sse_enabled", *sseMode), slog.String("address", ln.Addr().String()))
		if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			done <- fmt.Errorf("HTTP server error: %w", err)
		} else {
			done <- nil
		}
	}()
	return httpServer, nil
}

func startStdioServer(ctx context.Context, logger *slog.Logger, mcpServer *mcp.Server, done chan<- error) {
	go func() {
		logger.Info("starting in stdio mode")
		if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
			done <- err
		} else {
			done <- nil
		}
	}()
}
