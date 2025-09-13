package router

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestNewRouter provides comprehensive testing for the router, ensuring all
// endpoints are correctly registered and the info endpoint is accurate.
func TestNewRouter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// join is a local copy of the unexported join function from router.go for test setup.
	join := func(base, path string) string {
		b := strings.TrimRight(base, "/")
		p := strings.TrimLeft(path, "/")
		if b == "" {
			return "/" + p
		}
		return b + "/" + p
	}

	testCases := []struct {
		name         string
		config       *RouterConfig
		expectStream bool
		expectSSE    bool
	}{
		{
			name: "default config (stream only)",
			config: &RouterConfig{
				EnableStream: true,
				EnableSSE:    false,
				McpName:      "test-server",
				McpVersion:   "v1.2.3",
			},
			expectStream: true,
			expectSSE:    false,
		},
		{
			name: "sse and stream enabled",
			config: &RouterConfig{
				EnableStream: true,
				EnableSSE:    true,
				McpName:      "test-server",
				McpVersion:   "v1.2.3",
			},
			expectStream: true,
			expectSSE:    true,
		},
		{
			name: "all mcp disabled",
			config: &RouterConfig{
				EnableStream: false,
				EnableSSE:    false,
				McpName:      "test-server",
				McpVersion:   "v1.2.3",
			},
			expectStream: false,
			expectSSE:    false,
		},
		{
			name: "with base path",
			config: &RouterConfig{
				BasePath:     "/api/v1",
				EnableStream: true,
				EnableSSE:    true,
				McpName:      "test-server",
				McpVersion:   "v1.2.3",
			},
			expectStream: true,
			expectSSE:    true,
		},
		{
			name:         "nil config (defaults to stream only)",
			config:       nil,
			expectStream: true,
			expectSSE:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a fresh MCP server for each test to avoid contamination
			mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "v1.2.3"}, nil)

			// Determine effective config for this test case, as NewRouter handles nil.
			effectiveConfig := tc.config
			if effectiveConfig == nil {
				effectiveConfig = &RouterConfig{EnableStream: true} // This is the default in NewRouter
			}

			handler := NewRouter(mcpServer, logger, tc.config)

			// --- Test endpoint registration and methods ---
			testEndpoint := func(method, path string, expectedStatus int) {
				t.Helper()
				req := httptest.NewRequest(method, path, nil)
				rr := httptest.NewRecorder()
				handler.ServeHTTP(rr, req)
				if rr.Code != expectedStatus {
					t.Errorf("%s %s: expected status %d, got %d", method, path, expectedStatus, rr.Code)
				}
			}

			basePath := effectiveConfig.BasePath
			// Always-on endpoints
			testEndpoint(http.MethodGet, join(basePath, HEALTH), http.StatusOK)
			testEndpoint(http.MethodPost, join(basePath, HEALTH), http.StatusMethodNotAllowed)
			testEndpoint(http.MethodGet, join(basePath, READY), http.StatusOK)
			testEndpoint(http.MethodPost, join(basePath, READY), http.StatusMethodNotAllowed)
			testEndpoint(http.MethodGet, join(basePath, "/"), http.StatusOK)
			testEndpoint(http.MethodPost, join(basePath, "/"), http.StatusMethodNotAllowed)

			// Conditional stream endpoint
			streamPath := join(basePath, HTTP)
			if tc.expectStream {
				testEndpoint(http.MethodPost, streamPath, http.StatusBadRequest) // 400 for empty body is correct for mounted route
				testEndpoint(http.MethodGet, streamPath, http.StatusBadRequest) // Streamable handler returns 400 for GET
			} else {
				testEndpoint(http.MethodPost, streamPath, http.StatusNotFound)
				testEndpoint(http.MethodGet, streamPath, http.StatusNotFound)
			}

			// Conditional SSE endpoint
			ssePath := join(basePath, SSE)
			if tc.expectSSE {
				// SSE endpoint opens a persistent connection, so we need to test it differently
				// We'll just verify it's mounted and responds, but close the connection immediately
				req := httptest.NewRequest(http.MethodGet, ssePath, nil)
				rr := httptest.NewRecorder()

				// Use a goroutine with timeout to prevent hanging
				done := make(chan bool, 1)
				go func() {
					handler.ServeHTTP(rr, req)
					done <- true
				}()

				// Wait briefly for the SSE handler to start responding
				select {
				case <-done:
					// Handler completed (shouldn't happen for SSE)
				case <-time.After(10 * time.Millisecond):
					// SSE handler started, which is what we expect
				}

				// For SSE, just check that it started responding (status will be 200)
				// We can't easily check the full response without a proper SSE client
				testEndpoint(http.MethodPost, ssePath, http.StatusBadRequest) // SSE handler returns 400 for POST
			} else {
				testEndpoint(http.MethodGet, ssePath, http.StatusNotFound)
				testEndpoint(http.MethodPost, ssePath, http.StatusNotFound)
			}

			// --- Test info endpoint content ---
			infoReq := httptest.NewRequest(http.MethodGet, join(basePath, "/"), nil)
			infoRR := httptest.NewRecorder()
			handler.ServeHTTP(infoRR, infoReq)

			if infoRR.Code != http.StatusOK {
				t.Fatalf("info endpoint: expected status %d, got %d", http.StatusOK, infoRR.Code)
			}

			var infoResponse struct {
				Name      string `json:"name"`
				Version   string `json:"version"`
				Endpoints struct {
					Health string `json:"health"`
					Ready  string `json:"ready"`
					SSE    string `json:"sse,omitempty"`
					Stream string `json:"stream,omitempty"`
				} `json:"endpoints"`
			}

			if err := json.NewDecoder(infoRR.Body).Decode(&infoResponse); err != nil {
				t.Fatalf("info endpoint: failed to decode JSON response: %v", err)
			}

			if infoResponse.Name != effectiveConfig.McpName {
				t.Errorf("info.Name: expected %q, got %q", effectiveConfig.McpName, infoResponse.Name)
			}
			if infoResponse.Version != effectiveConfig.McpVersion {
				t.Errorf("info.Version: expected %q, got %q", effectiveConfig.McpVersion, infoResponse.Version)
			}

			if infoResponse.Endpoints.Health != join(basePath, HEALTH) {
				t.Errorf("info.Endpoints.Health: expected %q, got %q", join(basePath, HEALTH), infoResponse.Endpoints.Health)
			}
			if infoResponse.Endpoints.Ready != join(basePath, READY) {
				t.Errorf("info.Endpoints.Ready: expected %q, got %q", join(basePath, READY), infoResponse.Endpoints.Ready)
			}

			if tc.expectStream && infoResponse.Endpoints.Stream != streamPath {
				t.Errorf("info.Endpoints.Stream: expected %q, got %q", streamPath, infoResponse.Endpoints.Stream)
			} else if !tc.expectStream && infoResponse.Endpoints.Stream != "" {
				t.Errorf("info.Endpoints.Stream: expected empty, got %q", infoResponse.Endpoints.Stream)
			}

			if tc.expectSSE && infoResponse.Endpoints.SSE != ssePath {
				t.Errorf("info.Endpoints.SSE: expected %q, got %q", ssePath, infoResponse.Endpoints.SSE)
			} else if !tc.expectSSE && infoResponse.Endpoints.SSE != "" {
				t.Errorf("info.Endpoints.SSE: expected empty, got %q", infoResponse.Endpoints.SSE)
			}
		})
	}
}
