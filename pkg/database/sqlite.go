package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn       *sql.DB
	logger     *slog.Logger
	ftsEnabled bool // Whether FTS5 is available
}

// NewDBWithLogger creates a new database connection with a logger
func NewDBWithLogger(dbPath string, logger *slog.Logger) (*DB, error) {
	if logger == nil {
		logger = slog.Default()
	}

	// Ensure the parent directory exists
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	logger.Info("opening database connection",
		slog.String("path", dbPath),
	)

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool for SQLite
	conn.SetMaxOpenConns(1) // SQLite only supports one writer at a time
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0) // Connections don't expire

	db := &DB{
		conn:       conn,
		logger:     logger,
		ftsEnabled: false, // Will be set during migration
	}

	// Configure SQLite pragmas for better performance
	if err := db.configurePragmas(); err != nil {
		return nil, fmt.Errorf("failed to configure database: %w", err)
	}

	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	logger.Info("database initialized successfully")
	return db, nil
}

// configurePragmas sets SQLite pragmas for optimal performance
func (db *DB) configurePragmas() error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL",    // Write-Ahead Logging for better concurrency
		"PRAGMA synchronous = NORMAL",  // Good balance of safety and speed
		"PRAGMA cache_size = -64000",   // 64MB cache (negative = KB)
		"PRAGMA foreign_keys = ON",     // Enforce foreign key constraints
		"PRAGMA busy_timeout = 5000",   // 5 second timeout for locks
		"PRAGMA temp_store = MEMORY",   // Use memory for temporary tables
		"PRAGMA mmap_size = 268435456", // 256MB memory-mapped I/O
	}

	for _, pragma := range pragmas {
		db.logger.Debug("executing pragma",
			slog.String("pragma", pragma),
		)
		if _, err := db.conn.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute %s: %w", pragma, err)
		}
	}

	db.logger.Info("SQLite pragmas configured successfully")
	return nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

// IsFTSEnabled returns whether FTS5 is available
func (db *DB) IsFTSEnabled() bool {
	return db.ftsEnabled
}

func (db *DB) migrate() error {
	// Core table creation and indexes
	coreStatements := []string{
		`CREATE TABLE IF NOT EXISTS entities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			entity_type TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entity_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
			UNIQUE(entity_id, content)
		);`,
		`CREATE TABLE IF NOT EXISTS relations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_entity_id INTEGER NOT NULL,
			to_entity_id INTEGER NOT NULL,
			relation_type TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (from_entity_id) REFERENCES entities(id) ON DELETE CASCADE,
			FOREIGN KEY (to_entity_id) REFERENCES entities(id) ON DELETE CASCADE,
			UNIQUE(from_entity_id, to_entity_id, relation_type)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_entities_name ON entities(name);`,
		`CREATE INDEX IF NOT EXISTS idx_entities_type ON entities(entity_type);`,
		`CREATE INDEX IF NOT EXISTS idx_observations_entity ON observations(entity_id);`,
		`CREATE INDEX IF NOT EXISTS idx_observations_content ON observations(content);`, // For text search
		`CREATE INDEX IF NOT EXISTS idx_relations_from ON relations(from_entity_id);`,
		`CREATE INDEX IF NOT EXISTS idx_relations_to ON relations(to_entity_id);`,
		`CREATE INDEX IF NOT EXISTS idx_relations_type ON relations(relation_type);`, // For filtering by relation type
	}

	// Execute core statements
	for _, stmt := range coreStatements {
		if _, err := db.conn.Exec(stmt); err != nil {
			return err
		}
	}

	// Try to create FTS5 tables
	// Use simpler FTS5 tables without external content
	ftsStatements := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS entities_fts USING fts5(
			entity_id UNINDEXED,
			name, 
			entity_type,
			tokenize='porter unicode61'
		);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
			observation_id UNINDEXED,
			entity_id UNINDEXED,
			content,
			tokenize='porter unicode61'
		);`,
	}

	// Try to create FTS5 tables
	ftsCreated := true
	for _, stmt := range ftsStatements {
		if _, err := db.conn.Exec(stmt); err != nil {
			if strings.Contains(err.Error(), "no such module: fts5") {
				db.logger.Warn("FTS5 not available, skipping full-text search setup")
				ftsCreated = false
				break
			}
			return err
		}
	}

	// Only create triggers if FTS5 tables were successfully created
	if ftsCreated {
		db.ftsEnabled = true
		triggerStatements := []string{
			// Entity triggers
			`CREATE TRIGGER IF NOT EXISTS entities_ai AFTER INSERT ON entities BEGIN
				INSERT INTO entities_fts(entity_id, name, entity_type) 
				VALUES (new.id, new.name, new.entity_type);
			END;`,
			`CREATE TRIGGER IF NOT EXISTS entities_ad AFTER DELETE ON entities BEGIN
				DELETE FROM entities_fts WHERE entity_id = old.id;
			END;`,
			`CREATE TRIGGER IF NOT EXISTS entities_au AFTER UPDATE ON entities BEGIN
				DELETE FROM entities_fts WHERE entity_id = old.id;
				INSERT INTO entities_fts(entity_id, name, entity_type) 
				VALUES (new.id, new.name, new.entity_type);
			END;`,

			// Observation triggers
			`CREATE TRIGGER IF NOT EXISTS observations_ai AFTER INSERT ON observations BEGIN
				INSERT INTO observations_fts(observation_id, entity_id, content) 
				VALUES (new.id, new.entity_id, new.content);
			END;`,
			`CREATE TRIGGER IF NOT EXISTS observations_ad AFTER DELETE ON observations BEGIN
				DELETE FROM observations_fts WHERE observation_id = old.id;
			END;`,
			`CREATE TRIGGER IF NOT EXISTS observations_au AFTER UPDATE ON observations BEGIN
				DELETE FROM observations_fts WHERE observation_id = old.id;
				INSERT INTO observations_fts(observation_id, entity_id, content) 
				VALUES (new.id, new.entity_id, new.content);
			END;`,
		}

		for _, stmt := range triggerStatements {
			if _, err := db.conn.Exec(stmt); err != nil {
				return err
			}
		}

		db.logger.Info("FTS5 enabled successfully")
	} else {
		db.logger.Info("FTS5 not available, using standard LIKE search")
	}

	return nil
}

func (db *DB) CreateEntities(ctx context.Context, entities []EntityWithObservations) ([]EntityWithObservations, error) {
	start := time.Now()
	db.logger.Debug("creating entities",
		slog.Int("count", len(entities)),
	)

	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		db.logger.Error("failed to begin transaction",
			slog.String("error", err.Error()),
		)
		return nil, err
	}
	defer tx.Rollback()

	created := []EntityWithObservations{}

	for _, entity := range entities {
		var exists bool
		err := tx.QueryRowContext(ctx, "SELECT 1 FROM entities WHERE name = ?", entity.Name).Scan(&exists)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if exists {
			continue
		}

		result, err := tx.ExecContext(ctx,
			"INSERT INTO entities (name, entity_type) VALUES (?, ?)",
			entity.Name, entity.EntityType,
		)
		if err != nil {
			return nil, err
		}

		entityID, err := result.LastInsertId()
		if err != nil {
			return nil, err
		}

		for _, obs := range entity.Observations {
			_, err := tx.ExecContext(ctx,
				"INSERT INTO observations (entity_id, content) VALUES (?, ?)",
				entityID, obs,
			)
			if err != nil {
				return nil, err
			}
		}

		created = append(created, entity)
	}

	err = tx.Commit()
	if err != nil {
		db.logger.Error("failed to commit transaction",
			slog.String("error", err.Error()),
		)
		return nil, err
	}

	db.logger.Info("entities created successfully",
		slog.Int("requested", len(entities)),
		slog.Int("created", len(created)),
		slog.Duration("duration", time.Since(start)),
	)
	return created, nil
}

func (db *DB) CreateRelations(ctx context.Context, relations []RelationDTO) ([]RelationDTO, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	created := []RelationDTO{}

	for _, rel := range relations {
		var fromID, toID int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM entities WHERE name = ?", rel.From).Scan(&fromID)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, err
		}

		err = tx.QueryRowContext(ctx, "SELECT id FROM entities WHERE name = ?", rel.To).Scan(&toID)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, err
		}

		var exists bool
		err = tx.QueryRowContext(ctx,
			"SELECT 1 FROM relations WHERE from_entity_id = ? AND to_entity_id = ? AND relation_type = ?",
			fromID, toID, rel.RelationType,
		).Scan(&exists)
		if err != nil && err != sql.ErrNoRows {
			return nil, err
		}
		if exists {
			continue
		}

		_, err = tx.ExecContext(ctx,
			"INSERT INTO relations (from_entity_id, to_entity_id, relation_type) VALUES (?, ?, ?)",
			fromID, toID, rel.RelationType,
		)
		if err != nil {
			return nil, err
		}

		created = append(created, rel)
	}

	return created, tx.Commit()
}

func (db *DB) AddObservations(ctx context.Context, observations []ObservationAdditionInput) ([]ObservationAdditionResult, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	results := []ObservationAdditionResult{}

	for _, obs := range observations {
		var entityID int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM entities WHERE name = ?", obs.EntityName).Scan(&entityID)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, fmt.Errorf("entity with name %s not found", obs.EntityName)
			}
			return nil, err
		}

		added := []string{}
		for _, content := range obs.Contents {
			var exists bool
			err := tx.QueryRowContext(ctx,
				"SELECT 1 FROM observations WHERE entity_id = ? AND content = ?",
				entityID, content,
			).Scan(&exists)
			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}
			if exists {
				continue
			}

			_, err = tx.ExecContext(ctx,
				"INSERT INTO observations (entity_id, content) VALUES (?, ?)",
				entityID, content,
			)
			if err != nil {
				return nil, err
			}
			added = append(added, content)
		}

		results = append(results, ObservationAdditionResult{
			EntityName:        obs.EntityName,
			AddedObservations: added,
		})
	}

	return results, tx.Commit()
}

func (db *DB) DeleteEntities(ctx context.Context, entityNames []string) error {
	if len(entityNames) == 0 {
		return nil
	}

	placeholders := make([]string, len(entityNames))
	args := make([]interface{}, len(entityNames))
	for i, name := range entityNames {
		placeholders[i] = "?"
		args[i] = name
	}

	query := fmt.Sprintf("DELETE FROM entities WHERE name IN (%s)", strings.Join(placeholders, ","))
	_, err := db.conn.ExecContext(ctx, query, args...)
	return err
}

func (db *DB) DeleteObservations(ctx context.Context, deletions []ObservationDeletionInput) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, del := range deletions {
		var entityID int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM entities WHERE name = ?", del.EntityName).Scan(&entityID)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}

		for _, obs := range del.Observations {
			_, err := tx.ExecContext(ctx,
				"DELETE FROM observations WHERE entity_id = ? AND content = ?",
				entityID, obs,
			)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (db *DB) DeleteRelations(ctx context.Context, relations []RelationDTO) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, rel := range relations {
		var fromID, toID int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM entities WHERE name = ?", rel.From).Scan(&fromID)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}

		err = tx.QueryRowContext(ctx, "SELECT id FROM entities WHERE name = ?", rel.To).Scan(&toID)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}

		_, err = tx.ExecContext(ctx,
			"DELETE FROM relations WHERE from_entity_id = ? AND to_entity_id = ? AND relation_type = ?",
			fromID, toID, rel.RelationType,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) ReadGraph(ctx context.Context) (*KnowledgeGraph, error) {
	start := time.Now()
	db.logger.Debug("reading entire graph")

	graph := &KnowledgeGraph{
		Entities:  []EntityWithObservations{},
		Relations: []RelationDTO{},
	}

	// Optimized query using GROUP_CONCAT to avoid N+1 problem
	rows, err := db.conn.QueryContext(ctx, `
		SELECT 
			e.id, 
			e.name, 
			e.entity_type,
			COALESCE(GROUP_CONCAT(o.content, '|||'), '') as observations
		FROM entities e
		LEFT JOIN observations o ON e.id = o.entity_id
		GROUP BY e.id, e.name, e.entity_type
		ORDER BY e.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entityMap := make(map[int64]string)
	for rows.Next() {
		var id int64
		var entity EntityWithObservations
		var observationsStr string

		if err := rows.Scan(&id, &entity.Name, &entity.EntityType, &observationsStr); err != nil {
			return nil, err
		}

		entityMap[id] = entity.Name

		// Parse observations from concatenated string
		if observationsStr != "" {
			entity.Observations = strings.Split(observationsStr, "|||")
		} else {
			entity.Observations = []string{}
		}

		graph.Entities = append(graph.Entities, entity)
	}

	// Optimized query with JOINs to get relation names directly
	relRows, err := db.conn.QueryContext(ctx, `
        SELECT 
            e1.name as from_name,
            e2.name as to_name,
            r.relation_type
        FROM relations r
        JOIN entities e1 ON r.from_entity_id = e1.id
        JOIN entities e2 ON r.to_entity_id = e2.id
        ORDER BY e1.name, e2.name, r.relation_type
    `)
	if err != nil {
		return nil, err
	}
	defer relRows.Close()

	for relRows.Next() {
		var rel RelationDTO
		if err := relRows.Scan(&rel.From, &rel.To, &rel.RelationType); err != nil {
			return nil, err
		}
		graph.Relations = append(graph.Relations, rel)
	}

	db.logger.Info("graph read successfully",
		slog.Int("entities", len(graph.Entities)),
		slog.Int("relations", len(graph.Relations)),
		slog.Duration("duration", time.Since(start)),
	)
	return graph, nil
}

func (db *DB) SearchNodes(ctx context.Context, query string) (*KnowledgeGraph, error) {
	graph := &KnowledgeGraph{
		Entities:  []EntityWithObservations{},
		Relations: []RelationDTO{},
	}

	searchPattern := "%" + query + "%"

	// Optimized query using CTE and GROUP_CONCAT to avoid N+1 problem
	rows, err := db.conn.QueryContext(ctx, `
		WITH matched_entities AS (
			SELECT DISTINCT e.id
			FROM entities e
			LEFT JOIN observations o ON e.id = o.entity_id
			WHERE 
				e.name LIKE ? OR
				e.entity_type LIKE ? OR
				o.content LIKE ?
		)
		SELECT 
			e.id,
			e.name,
			e.entity_type,
			COALESCE(GROUP_CONCAT(o.content, '|||'), '') as observations
		FROM entities e
		LEFT JOIN observations o ON e.id = o.entity_id
		WHERE e.id IN (SELECT id FROM matched_entities)
		GROUP BY e.id, e.name, e.entity_type
		ORDER BY e.name
	`, searchPattern, searchPattern, searchPattern)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entityIDs := []int64{}
	entityMap := make(map[int64]string)

	for rows.Next() {
		var id int64
		var entity EntityWithObservations
		var observationsStr string

		if err := rows.Scan(&id, &entity.Name, &entity.EntityType, &observationsStr); err != nil {
			return nil, err
		}

		entityIDs = append(entityIDs, id)
		entityMap[id] = entity.Name

		// Parse observations from concatenated string
		if observationsStr != "" {
			entity.Observations = strings.Split(observationsStr, "|||")
		} else {
			entity.Observations = []string{}
		}

		graph.Entities = append(graph.Entities, entity)
	}

	// Get relations between matched entities with optimized query
	if len(entityIDs) > 0 {
		placeholders := make([]string, len(entityIDs))
		args := make([]interface{}, 0, len(entityIDs)*2)

		for i, id := range entityIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}

		// Duplicate the args for both IN clauses
		args = append(args, args[:len(entityIDs)]...)

		relQuery := fmt.Sprintf(`
			SELECT 
				e1.name as from_name,
				e2.name as to_name,
				r.relation_type
			FROM relations r
			JOIN entities e1 ON r.from_entity_id = e1.id
			JOIN entities e2 ON r.to_entity_id = e2.id
			WHERE r.from_entity_id IN (%s) AND r.to_entity_id IN (%s)
			ORDER BY e1.name, e2.name, r.relation_type
		`, strings.Join(placeholders, ","), strings.Join(placeholders, ","))

		relRows, err := db.conn.QueryContext(ctx, relQuery, args...)
		if err != nil {
			return nil, err
		}
		defer relRows.Close()

		for relRows.Next() {
			var rel RelationDTO
			if err := relRows.Scan(&rel.From, &rel.To, &rel.RelationType); err != nil {
				return nil, err
			}
			graph.Relations = append(graph.Relations, rel)
		}
	}

	return graph, nil
}

func (db *DB) OpenNodes(ctx context.Context, names []string) (*KnowledgeGraph, error) {
	graph := &KnowledgeGraph{
		Entities:  []EntityWithObservations{},
		Relations: []RelationDTO{},
	}

	if len(names) == 0 {
		return graph, nil
	}

	placeholders := make([]string, len(names))
	args := make([]interface{}, len(names))
	for i, name := range names {
		placeholders[i] = "?"
		args[i] = name
	}

	// Optimized query using GROUP_CONCAT to avoid N+1 problem
	query := fmt.Sprintf(`
		SELECT 
			e.id,
			e.name,
			e.entity_type,
			COALESCE(GROUP_CONCAT(o.content, '|||'), '') as observations
		FROM entities e
		LEFT JOIN observations o ON e.id = o.entity_id
		WHERE e.name IN (%s)
		GROUP BY e.id, e.name, e.entity_type
		ORDER BY e.name
	`, strings.Join(placeholders, ","))

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entityIDs := []int64{}
	entityMap := make(map[int64]string)

	for rows.Next() {
		var id int64
		var entity EntityWithObservations
		var observationsStr string

		if err := rows.Scan(&id, &entity.Name, &entity.EntityType, &observationsStr); err != nil {
			return nil, err
		}

		entityIDs = append(entityIDs, id)
		entityMap[id] = entity.Name

		// Parse observations from concatenated string
		if observationsStr != "" {
			entity.Observations = strings.Split(observationsStr, "|||")
		} else {
			entity.Observations = []string{}
		}

		graph.Entities = append(graph.Entities, entity)
	}

	// Get relations between opened nodes with optimized query
	if len(entityIDs) > 0 {
		placeholders := make([]string, len(entityIDs))
		relArgs := make([]interface{}, 0, len(entityIDs)*2)

		for i, id := range entityIDs {
			placeholders[i] = "?"
			relArgs = append(relArgs, id)
		}

		// Duplicate for both IN clauses
		relArgs = append(relArgs, relArgs[:len(entityIDs)]...)

		relQuery := fmt.Sprintf(`
			SELECT 
				e1.name as from_name,
				e2.name as to_name,
				r.relation_type
			FROM relations r
			JOIN entities e1 ON r.from_entity_id = e1.id
			JOIN entities e2 ON r.to_entity_id = e2.id
			WHERE r.from_entity_id IN (%s) AND r.to_entity_id IN (%s)
			ORDER BY e1.name, e2.name, r.relation_type
		`, strings.Join(placeholders, ","), strings.Join(placeholders, ","))

		relRows, err := db.conn.QueryContext(ctx, relQuery, relArgs...)
		if err != nil {
			return nil, err
		}
		defer relRows.Close()

		for relRows.Next() {
			var rel RelationDTO
			if err := relRows.Scan(&rel.From, &rel.To, &rel.RelationType); err != nil {
				return nil, err
			}
			graph.Relations = append(graph.Relations, rel)
		}
	}

	return graph, nil
}
