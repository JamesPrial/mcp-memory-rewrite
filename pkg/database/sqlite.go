package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

func NewDB(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	statements := []string{
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
		`CREATE INDEX IF NOT EXISTS idx_relations_from ON relations(from_entity_id);`,
		`CREATE INDEX IF NOT EXISTS idx_relations_to ON relations(to_entity_id);`,
		`PRAGMA foreign_keys = ON;`,
	}

	for _, stmt := range statements {
		if _, err := db.conn.Exec(stmt); err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) CreateEntities(ctx context.Context, entities []EntityWithObservations) ([]EntityWithObservations, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
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

	return created, tx.Commit()
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

func (db *DB) AddObservations(ctx context.Context, observations []struct {
	EntityName string   `json:"entityName"`
	Contents   []string `json:"contents"`
}) ([]struct {
	EntityName         string   `json:"entityName"`
	AddedObservations []string `json:"addedObservations"`
}, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	results := []struct {
		EntityName         string   `json:"entityName"`
		AddedObservations []string `json:"addedObservations"`
	}{}

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

		results = append(results, struct {
			EntityName         string   `json:"entityName"`
			AddedObservations []string `json:"addedObservations"`
		}{
			EntityName:         obs.EntityName,
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

func (db *DB) DeleteObservations(ctx context.Context, deletions []struct {
	EntityName   string   `json:"entityName"`
	Observations []string `json:"observations"`
}) error {
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
	graph := &KnowledgeGraph{
		Entities:  []EntityWithObservations{},
		Relations: []RelationDTO{},
	}

	rows, err := db.conn.QueryContext(ctx, `
		SELECT e.id, e.name, e.entity_type 
		FROM entities e 
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
		if err := rows.Scan(&id, &entity.Name, &entity.EntityType); err != nil {
			return nil, err
		}
		entityMap[id] = entity.Name

		obsRows, err := db.conn.QueryContext(ctx, "SELECT content FROM observations WHERE entity_id = ?", id)
		if err != nil {
			return nil, err
		}
		
		observations := []string{}
		for obsRows.Next() {
			var content string
			if err := obsRows.Scan(&content); err != nil {
				obsRows.Close()
				return nil, err
			}
			observations = append(observations, content)
		}
		obsRows.Close()
		
		entity.Observations = observations
		graph.Entities = append(graph.Entities, entity)
	}

	relRows, err := db.conn.QueryContext(ctx, `
		SELECT from_entity_id, to_entity_id, relation_type 
		FROM relations
	`)
	if err != nil {
		return nil, err
	}
	defer relRows.Close()

	for relRows.Next() {
		var fromID, toID int64
		var relationType string
		if err := relRows.Scan(&fromID, &toID, &relationType); err != nil {
			return nil, err
		}

		if fromName, ok := entityMap[fromID]; ok {
			if toName, ok := entityMap[toID]; ok {
				graph.Relations = append(graph.Relations, RelationDTO{
					From:         fromName,
					To:           toName,
					RelationType: relationType,
				})
			}
		}
	}

	return graph, nil
}

func (db *DB) SearchNodes(ctx context.Context, query string) (*KnowledgeGraph, error) {
	graph := &KnowledgeGraph{
		Entities:  []EntityWithObservations{},
		Relations: []RelationDTO{},
	}

	matchedEntityIDs := make(map[int64]bool)

	rows, err := db.conn.QueryContext(ctx, `
		SELECT DISTINCT e.id, e.name, e.entity_type
		FROM entities e
		LEFT JOIN observations o ON e.id = o.entity_id
		WHERE 
			e.name LIKE ? OR
			e.entity_type LIKE ? OR
			o.content LIKE ?
	`, "%"+query+"%", "%"+query+"%", "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entityMap := make(map[int64]string)
	for rows.Next() {
		var id int64
		var entity EntityWithObservations
		if err := rows.Scan(&id, &entity.Name, &entity.EntityType); err != nil {
			return nil, err
		}
		
		if matchedEntityIDs[id] {
			continue
		}
		matchedEntityIDs[id] = true
		entityMap[id] = entity.Name

		obsRows, err := db.conn.QueryContext(ctx, "SELECT content FROM observations WHERE entity_id = ?", id)
		if err != nil {
			return nil, err
		}
		
		observations := []string{}
		for obsRows.Next() {
			var content string
			if err := obsRows.Scan(&content); err != nil {
				obsRows.Close()
				return nil, err
			}
			observations = append(observations, content)
		}
		obsRows.Close()
		
		entity.Observations = observations
		graph.Entities = append(graph.Entities, entity)
	}

	if len(matchedEntityIDs) > 0 {
		placeholders := make([]string, 0)
		args := make([]interface{}, 0)
		for id := range matchedEntityIDs {
			placeholders = append(placeholders, "?")
			args = append(args, id)
		}

		relQuery := fmt.Sprintf(`
			SELECT from_entity_id, to_entity_id, relation_type 
			FROM relations 
			WHERE from_entity_id IN (%s) AND to_entity_id IN (%s)
		`, strings.Join(placeholders, ","), strings.Join(placeholders, ","))
		
		args = append(args, args...)
		relRows, err := db.conn.QueryContext(ctx, relQuery, args...)
		if err != nil {
			return nil, err
		}
		defer relRows.Close()

		for relRows.Next() {
			var fromID, toID int64
			var relationType string
			if err := relRows.Scan(&fromID, &toID, &relationType); err != nil {
				return nil, err
			}

			if fromName, ok := entityMap[fromID]; ok {
				if toName, ok := entityMap[toID]; ok {
					graph.Relations = append(graph.Relations, RelationDTO{
						From:         fromName,
						To:           toName,
						RelationType: relationType,
					})
				}
			}
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

	query := fmt.Sprintf(`
		SELECT id, name, entity_type 
		FROM entities 
		WHERE name IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entityMap := make(map[int64]string)
	entityIDs := []int64{}
	
	for rows.Next() {
		var id int64
		var entity EntityWithObservations
		if err := rows.Scan(&id, &entity.Name, &entity.EntityType); err != nil {
			return nil, err
		}
		entityMap[id] = entity.Name
		entityIDs = append(entityIDs, id)

		obsRows, err := db.conn.QueryContext(ctx, "SELECT content FROM observations WHERE entity_id = ?", id)
		if err != nil {
			return nil, err
		}
		
		observations := []string{}
		for obsRows.Next() {
			var content string
			if err := obsRows.Scan(&content); err != nil {
				obsRows.Close()
				return nil, err
			}
			observations = append(observations, content)
		}
		obsRows.Close()
		
		entity.Observations = observations
		graph.Entities = append(graph.Entities, entity)
	}

	if len(entityIDs) > 0 {
		placeholders := make([]string, len(entityIDs))
		args := make([]interface{}, 0)
		for i, id := range entityIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}

		relQuery := fmt.Sprintf(`
			SELECT from_entity_id, to_entity_id, relation_type 
			FROM relations 
			WHERE from_entity_id IN (%s) AND to_entity_id IN (%s)
		`, strings.Join(placeholders, ","), strings.Join(placeholders, ","))
		
		args = append(args, args[:len(entityIDs)]...)
		relRows, err := db.conn.QueryContext(ctx, relQuery, args...)
		if err != nil {
			return nil, err
		}
		defer relRows.Close()

		for relRows.Next() {
			var fromID, toID int64
			var relationType string
			if err := relRows.Scan(&fromID, &toID, &relationType); err != nil {
				return nil, err
			}

			if fromName, ok := entityMap[fromID]; ok {
				if toName, ok := entityMap[toID]; ok {
					graph.Relations = append(graph.Relations, RelationDTO{
						From:         fromName,
						To:           toName,
						RelationType: relationType,
					})
				}
			}
		}
	}

	return graph, nil
}
