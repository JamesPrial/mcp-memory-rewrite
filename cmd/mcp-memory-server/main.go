package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jamesprial/mcp-memory-rewrite/internal/config"
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

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Initialize database
	db, err := database.NewDB(cfg.DBPath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}

	// Create the server
	srv := server.NewServer(db)

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

			if *sseMode {
				log.Printf("MCP Memory Server starting in SSE mode on %s...", *httpAddr)
				// SSE handler expects a function that returns the server
				handler = mcp.NewSSEHandler(func(*http.Request) *mcp.Server {
					return mcpServer
				})
			} else {
				log.Printf("MCP Memory Server starting in HTTP mode on %s...", *httpAddr)
				// Streamable HTTP handler for standard HTTP
				handler = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
					return mcpServer
				}, nil)
			}

			httpServer := &http.Server{Handler: handler}

			// Graceful shutdown for HTTP server
			go func() {
				<-ctx.Done()
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := httpServer.Shutdown(shutdownCtx); err != nil {
					log.Printf("HTTP server shutdown error: %v", err)
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
					log.Printf("failed writing portfile: %v", err)
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
			log.Println("MCP Memory Server starting in stdio mode...")
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
			log.Fatal("Server error:", err)
		}
		// Ensure graceful shutdown of server resources (DB, etc.)
		if sErr := srv.Shutdown(context.Background()); sErr != nil {
			log.Printf("Shutdown error: %v", sErr)
		}
		log.Println("Server stopped")
	case sig := <-sigChan:
		log.Printf("Received signal: %v. Shutting down gracefully...", sig)

		// Create a timeout context for shutdown
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		// Cancel the main context to signal the server to stop
		cancel()

		// Shutdown the application server
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}

		// Wait for server to finish or timeout
		select {
		case <-done:
			log.Println("Graceful shutdown completed")
		case <-shutdownCtx.Done():
			log.Println("Shutdown timeout exceeded")
		}
	}
}
