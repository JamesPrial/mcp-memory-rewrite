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
	"github.com/jamesprial/mcp-memory-rewrite/pkg/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	httpAddr = flag.String("http", "", "HTTP address to listen on (e.g., :8080). If not set, uses stdio")
	sseMode  = flag.Bool("sse", false, "Use SSE (Server-Sent Events) for HTTP mode")
	portFile = flag.String("portfile", "", "If set with -http, write the actual bound TCP port to this file")
)

func main() {
	flag.Parse()

	// Setup structured logging
	logLevel := logging.GetLogLevel()
	logger := logging.NewLogger("mcp-memory-server", logLevel)
	slog.SetDefault(logger)

	// Log startup information
	logger.Info("starting MCP memory server",
		slog.String("version", "0.1.0"),
		slog.String("log_level", logLevel.String()),
	)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration",
			slog.String("error", err.Error()),
		)
		os.Exit(1)
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
		os.Exit(1)
	}

	// Create the server with logger
	srvLogger := logger.With(slog.String("component", "server"))
	srv := server.NewServerWithLogger(db, srvLogger)

	// Create MCP server
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mcp-memory-server",
			Version: "0.1.0",
		},
		nil,
	)

	// Register all tools
	srv.RegisterTools(mcpServer)

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Channel to listen for interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Channel to signal when server is done
	done := make(chan error, 1)

	// Start the appropriate server based on flags
	if *httpAddr != "" {
		// HTTP mode
		go func() {
			var handler http.Handler
			mode := "HTTP"

			if *sseMode {
				mode = "SSE"
				// SSE handler expects a function that returns the server
				handler = mcp.NewSSEHandler(func(*http.Request) *mcp.Server {
					return mcpServer
				})
			} else {
				// Streamable HTTP handler for standard HTTP
				handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
					return mcpServer
				}, nil)
			}

			logger.Info("starting HTTP server",
				slog.String("mode", mode),
				slog.String("address", *httpAddr),
			)

			httpServer := &http.Server{Handler: handler}

			// Graceful shutdown for HTTP server
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				logger.Info("shutting down HTTP server")
				if err := httpServer.Shutdown(shutdownCtx); err != nil {
					logger.Error("HTTP server shutdown error",
						slog.String("error", err.Error()),
					)
				}
			}()

			// Allow :0 and write the bound port if requested
			ln, err := net.Listen("tcp", *httpAddr)
			if err != nil {
				done <- fmt.Errorf("HTTP listen error: %w", err)
				return
			}
			if *portFile != "" {
				addr := ln.Addr().(*net.TCPAddr)
				// Best-effort write
				if err := os.WriteFile(*portFile, []byte(fmt.Sprintf("%d", addr.Port)), 0644); err != nil {
					logger.Warn("failed writing portfile",
						slog.String("error", err.Error()),
						slog.String("file", *portFile),
					)
				} else {
					logger.Info("wrote port to file",
						slog.Int("port", addr.Port),
						slog.String("file", *portFile),
					)
				}
			}
			if err := httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
				done <- fmt.Errorf("HTTP server error: %w", err)
			} else {
				done <- nil
			}
		}()
	} else {
		// Stdio mode (default)
		go func() {
			logger.Info("starting in stdio mode")
			if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
				done <- err
			} else {
				done <- nil
			}
		}()
	}

	// Wait for either server error or interrupt signal
	select {
	case err := <-done:
		if err != nil {
			logger.Error("server error",
				slog.String("error", err.Error()),
			)
			os.Exit(1)
		}
		// Ensure graceful shutdown of server resources (DB, etc.)
		if sErr := srv.Shutdown(context.Background()); sErr != nil {
			logger.Error("shutdown error",
				slog.String("error", sErr.Error()),
			)
		}
		logger.Info("server stopped")
	case sig := <-sigChan:
		logger.Info("received signal, shutting down gracefully",
			slog.String("signal", sig.String()),
		)

		// Create a timeout context for shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		// Cancel the main context to signal the server to stop
		cancel()

		// Shutdown the application server
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("error during shutdown",
				slog.String("error", err.Error()),
			)
		}

		// Wait for server to finish or timeout
		select {
		case <-done:
			logger.Info("graceful shutdown completed")
		case <-shutdownCtx.Done():
			logger.Warn("shutdown timeout exceeded")
		}
	}
}
