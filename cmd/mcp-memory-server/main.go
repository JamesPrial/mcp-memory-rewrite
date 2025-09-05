package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jamesprial/mcp-memory-rewrite/internal/config"
	"github.com/jamesprial/mcp-memory-rewrite/pkg/database"
	"github.com/jamesprial/mcp-memory-rewrite/pkg/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
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
	defer db.Close()

	// Create the server
	srv := server.NewServer(db)

	// Create MCP server
	mcpServer := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mcp-memory-rewrite",
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

	// Start the MCP server in a goroutine
	go func() {
		log.Println("MCP Memory Server starting...")
		if err := mcpServer.Run(ctx, &mcp.StdioTransport{}); err != nil {
			done <- err
		} else {
			done <- nil
		}
	}()

	// Wait for either server error or interrupt signal
	select {
	case err := <-done:
		if err != nil {
			log.Fatal("Server error:", err)
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
