package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jamesprial/mcp-memory-rewrite/internal/logging"
	"github.com/jamesprial/mcp-memory-rewrite/pkg/database"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	db     *database.DB
	logger *slog.Logger
}

type CreateEntitiesParams struct {
	Entities []database.EntityWithObservations `json:"entities" jsonschema:"description:Array of entities to create"`
}

type CreateRelationsParams struct {
	Relations []database.RelationDTO `json:"relations" jsonschema:"description:Array of relations to create"`
}

type AddObservationsParams struct {
	Observations []ObservationInput `json:"observations" jsonschema:"description:Array of observations to add"`
}

type ObservationInput struct {
	EntityName string   `json:"entityName" jsonschema:"description:Name of the entity"`
	Contents   []string `json:"contents" jsonschema:"description:Array of observations to add"`
}

type DeleteEntitiesParams struct {
	EntityNames []string `json:"entityNames" jsonschema:"description:Array of entity names to delete"`
}

type DeleteObservationsParams struct {
	Deletions []DeletionInput `json:"deletions" jsonschema:"description:Array of deletions to perform"`
}

type DeletionInput struct {
	EntityName   string   `json:"entityName" jsonschema:"description:Name of the entity"`
	Observations []string `json:"observations" jsonschema:"description:Array of observations to delete"`
}

type DeleteRelationsParams struct {
	Relations []database.RelationDTO `json:"relations" jsonschema:"description:Array of relations to delete"`
}

type SearchNodesParams struct {
	Query string `json:"query" jsonschema:"description:Search query to match against entity names types and observations"`
}

type OpenNodesParams struct {
	Names []string `json:"names" jsonschema:"description:Array of entity names to retrieve"`
}

// NewServerWithLogger creates a new MCP memory server with a logger
func NewServerWithLogger(db *database.DB, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		db:     db,
		logger: logger,
	}
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.db.Close()
}

// RegisterTools registers all MCP tools with the server
func (s *Server) RegisterTools(mcpServer *mcp.Server) {
	mcp.AddTool(mcpServer,
		&mcp.Tool{
			Name:        "create_entities",
			Description: "Create multiple new entities in the knowledge graph",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, params CreateEntitiesParams) (*mcp.CallToolResult, any, error) {
			return s.handleCreateEntities(ctx, params)
		},
	)

	mcp.AddTool(mcpServer,
		&mcp.Tool{
			Name:        "create_relations",
			Description: "Create multiple new relations between entities in the knowledge graph. Relations should be in active voice",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, params CreateRelationsParams) (*mcp.CallToolResult, any, error) {
			return s.handleCreateRelations(ctx, params)
		},
	)

	mcp.AddTool(mcpServer,
		&mcp.Tool{
			Name:        "add_observations",
			Description: "Add new observations to existing entities in the knowledge graph",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, params AddObservationsParams) (*mcp.CallToolResult, any, error) {
			return s.handleAddObservations(ctx, params)
		},
	)

	mcp.AddTool(mcpServer,
		&mcp.Tool{
			Name:        "delete_entities",
			Description: "Delete multiple entities and their associated relations from the knowledge graph",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, params DeleteEntitiesParams) (*mcp.CallToolResult, any, error) {
			return s.handleDeleteEntities(ctx, params)
		},
	)

	mcp.AddTool(mcpServer,
		&mcp.Tool{
			Name:        "delete_observations",
			Description: "Delete specific observations from entities in the knowledge graph",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, params DeleteObservationsParams) (*mcp.CallToolResult, any, error) {
			return s.handleDeleteObservations(ctx, params)
		},
	)

	mcp.AddTool(mcpServer,
		&mcp.Tool{
			Name:        "delete_relations",
			Description: "Delete multiple relations from the knowledge graph",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, params DeleteRelationsParams) (*mcp.CallToolResult, any, error) {
			return s.handleDeleteRelations(ctx, params)
		},
	)

	mcp.AddTool(mcpServer,
		&mcp.Tool{
			Name:        "read_graph",
			Description: "Read the entire knowledge graph",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
			return s.handleReadGraph(ctx)
		},
	)

	mcp.AddTool(mcpServer,
		&mcp.Tool{
			Name:        "search_nodes",
			Description: "Search for nodes in the knowledge graph based on a query",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, params SearchNodesParams) (*mcp.CallToolResult, any, error) {
			return s.handleSearchNodes(ctx, params)
		},
	)

	mcp.AddTool(mcpServer,
		&mcp.Tool{
			Name:        "open_nodes",
			Description: "Open specific nodes in the knowledge graph by their names",
		},
		func(ctx context.Context, req *mcp.CallToolRequest, params OpenNodesParams) (*mcp.CallToolResult, any, error) {
			return s.handleOpenNodes(ctx, params)
		},
	)
}

func (s *Server) handleCreateEntities(ctx context.Context, params CreateEntitiesParams) (*mcp.CallToolResult, any, error) {
	logger := logging.LoggerWithContext(ctx, s.logger)
	start := time.Now()

	logger.Info("handling create_entities request",
		slog.Int("entity_count", len(params.Entities)),
	)

	// Validate input parameters
	if err := ValidateCreateEntitiesParams(params); err != nil {
		logger.Warn("invalid create_entities parameters",
			slog.String("error", err.Error()),
		)
		return nil, nil, fmt.Errorf("validation error: %w", err)
	}

	created, err := s.db.CreateEntities(ctx, params.Entities)
	if err != nil {
		logger.Error("failed to create entities",
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(start)),
		)
		return nil, nil, fmt.Errorf("failed to create entities: %w", err)
	}

	logger.Info("entities created successfully",
		slog.Int("created", len(created)),
		slog.Duration("duration", time.Since(start)),
	)

	jsonData, _ := json.MarshalIndent(created, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
	}, nil, nil
}

func (s *Server) handleCreateRelations(ctx context.Context, params CreateRelationsParams) (*mcp.CallToolResult, any, error) {
	logger := logging.LoggerWithContext(ctx, s.logger)

	// Validate input parameters
	if err := ValidateCreateRelationsParams(params); err != nil {
		logger.Warn("invalid create_relations parameters",
			slog.String("error", err.Error()),
		)
		return nil, nil, fmt.Errorf("validation error: %w", err)
	}

	created, err := s.db.CreateRelations(ctx, params.Relations)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create relations: %w", err)
	}

	jsonData, _ := json.MarshalIndent(created, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
	}, nil, nil
}

func (s *Server) handleAddObservations(ctx context.Context, params AddObservationsParams) (*mcp.CallToolResult, any, error) {
	logger := logging.LoggerWithContext(ctx, s.logger)

	// Validate input parameters
	if err := ValidateAddObservationsParams(params); err != nil {
		logger.Warn("invalid add_observations parameters",
			slog.String("error", err.Error()),
		)
		return nil, nil, fmt.Errorf("validation error: %w", err)
	}

	// Convert to the format expected by the database (named type)
	dbParams := make([]database.ObservationAdditionInput, len(params.Observations))
	for i, obs := range params.Observations {
		dbParams[i] = database.ObservationAdditionInput{EntityName: obs.EntityName, Contents: obs.Contents}
	}

	results, err := s.db.AddObservations(ctx, dbParams)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add observations: %w", err)
	}

	jsonData, _ := json.MarshalIndent(results, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
	}, nil, nil
}

func (s *Server) handleDeleteEntities(ctx context.Context, params DeleteEntitiesParams) (*mcp.CallToolResult, any, error) {
	if err := s.db.DeleteEntities(ctx, params.EntityNames); err != nil {
		return nil, nil, fmt.Errorf("failed to delete entities: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Entities deleted successfully"},
		},
	}, nil, nil
}

func (s *Server) handleDeleteObservations(ctx context.Context, params DeleteObservationsParams) (*mcp.CallToolResult, any, error) {
	// Convert to the format expected by the database (named type)
	dbParams := make([]database.ObservationDeletionInput, len(params.Deletions))
	for i, del := range params.Deletions {
		dbParams[i] = database.ObservationDeletionInput{EntityName: del.EntityName, Observations: del.Observations}
	}

	if err := s.db.DeleteObservations(ctx, dbParams); err != nil {
		return nil, nil, fmt.Errorf("failed to delete observations: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Observations deleted successfully"},
		},
	}, nil, nil
}

func (s *Server) handleDeleteRelations(ctx context.Context, params DeleteRelationsParams) (*mcp.CallToolResult, any, error) {
	if err := s.db.DeleteRelations(ctx, params.Relations); err != nil {
		return nil, nil, fmt.Errorf("failed to delete relations: %w", err)
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Relations deleted successfully"},
		},
	}, nil, nil
}

func (s *Server) handleReadGraph(ctx context.Context) (*mcp.CallToolResult, any, error) {
	graph, err := s.db.ReadGraph(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read graph: %w", err)
	}

	jsonData, _ := json.MarshalIndent(graph, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
	}, nil, nil
}

func (s *Server) handleSearchNodes(ctx context.Context, params SearchNodesParams) (*mcp.CallToolResult, any, error) {
	logger := logging.LoggerWithContext(ctx, s.logger)
	start := time.Now()

	logger.Info("handling search_nodes request",
		slog.String("query", params.Query),
	)

	// Validate input parameters
	if err := ValidateSearchNodesParams(params); err != nil {
		logger.Warn("invalid search_nodes parameters",
			slog.String("error", err.Error()),
		)
		return nil, nil, fmt.Errorf("validation error: %w", err)
	}

	// Try FTS5 search if available, otherwise use LIKE search
	var graph *database.KnowledgeGraph
	var err error

	if s.db.IsFTSEnabled() {
		graph, err = s.db.SearchNodesFTS(ctx, params.Query)
		if err != nil {
			logger.Debug("FTS5 search failed, falling back to LIKE search",
				slog.String("error", err.Error()),
			)
			// Fallback to regular LIKE-based search
			graph, err = s.db.SearchNodes(ctx, params.Query)
		}
	} else {
		// FTS not available, use LIKE search
		graph, err = s.db.SearchNodes(ctx, params.Query)
	}

	if err != nil {
		logger.Error("failed to search nodes",
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(start)),
		)
		return nil, nil, fmt.Errorf("failed to search nodes: %w", err)
	}

	logger.Info("search completed successfully",
		slog.Int("entities_found", len(graph.Entities)),
		slog.Int("relations_found", len(graph.Relations)),
		slog.Duration("duration", time.Since(start)),
	)

	jsonData, _ := json.MarshalIndent(graph, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
	}, nil, nil
}

func (s *Server) handleOpenNodes(ctx context.Context, params OpenNodesParams) (*mcp.CallToolResult, any, error) {
	logger := logging.LoggerWithContext(ctx, s.logger)

	// Validate input parameters
	if err := ValidateOpenNodesParams(params); err != nil {
		logger.Warn("invalid open_nodes parameters",
			slog.String("error", err.Error()),
		)
		return nil, nil, fmt.Errorf("validation error: %w", err)
	}

	graph, err := s.db.OpenNodes(ctx, params.Names)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open nodes: %w", err)
	}

	jsonData, _ := json.MarshalIndent(graph, "", "  ")
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(jsonData)},
		},
	}, nil, nil
}
