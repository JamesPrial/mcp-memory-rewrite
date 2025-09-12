package database

import (
	"context"
	"fmt"
	"strings"
)

// SearchNodesFTS performs full-text search using FTS5 tables for better performance
func (db *DB) SearchNodesFTS(ctx context.Context, query string) (*KnowledgeGraph, error) {
	graph := &KnowledgeGraph{
		Entities:  []EntityWithObservations{},
		Relations: []RelationDTO{},
	}

	// Escape special FTS5 characters in the query
	ftsQuery := escapeFTS5(query)
	
	// Use FTS5 MATCH for efficient full-text search
	// This query finds entities that match in either their name/type or observations
	rows, err := db.conn.QueryContext(ctx, `
		WITH matched_entities AS (
			-- Match entities by name or type
			SELECT DISTINCT entity_id as id
			FROM entities_fts 
			WHERE entities_fts MATCH ?
			UNION
			-- Match entities by their observations
			SELECT DISTINCT entity_id as id
			FROM observations_fts 
			WHERE observations_fts MATCH ?
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
	`, ftsQuery, ftsQuery)
	
	if err != nil {
		// Fallback to LIKE search if FTS5 is not available or query fails
		return db.SearchNodes(ctx, query)
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
		args := make([]any, 0, len(entityIDs)*2)
		
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

// SearchNodesRanked performs FTS5 search with relevance ranking
func (db *DB) SearchNodesRanked(ctx context.Context, query string) (*KnowledgeGraph, error) {
	graph := &KnowledgeGraph{
		Entities:  []EntityWithObservations{},
		Relations: []RelationDTO{},
	}

	// Escape special FTS5 characters
	ftsQuery := escapeFTS5(query)
	
	// Search with ranking - entities matching in name/type rank higher than observation matches
	rows, err := db.conn.QueryContext(ctx, `
		WITH ranked_matches AS (
			-- Direct entity matches (higher rank)
			SELECT e.id, 1.0 as rank
			FROM entities e
			WHERE e.id IN (
				SELECT rowid FROM entities_fts 
				WHERE entities_fts MATCH ?
				ORDER BY rank
			)
			UNION ALL
			-- Observation matches (lower rank) 
			SELECT e.id, 0.5 as rank
			FROM entities e
			JOIN observations o ON e.id = o.entity_id
			WHERE o.id IN (
				SELECT rowid FROM observations_fts 
				WHERE observations_fts MATCH ?
				ORDER BY rank
			)
		),
		matched_entities AS (
			SELECT id, MAX(rank) as max_rank
			FROM ranked_matches
			GROUP BY id
		)
		SELECT 
			e.id,
			e.name,
			e.entity_type,
			COALESCE(GROUP_CONCAT(o.content, '|||'), '') as observations,
			m.max_rank
		FROM entities e
		LEFT JOIN observations o ON e.id = o.entity_id
		JOIN matched_entities m ON e.id = m.id
		GROUP BY e.id, e.name, e.entity_type, m.max_rank
		ORDER BY m.max_rank DESC, e.name
	`, ftsQuery, ftsQuery)
	
	if err != nil {
		// Fallback to regular search
		return db.SearchNodesFTS(ctx, query)
	}
	defer rows.Close()

	entityIDs := []int64{}
	entityMap := make(map[int64]string)
	
	for rows.Next() {
		var id int64
		var entity EntityWithObservations
		var observationsStr string
		var rank float64
		
		if err := rows.Scan(&id, &entity.Name, &entity.EntityType, &observationsStr, &rank); err != nil {
			return nil, err
		}
		
		entityIDs = append(entityIDs, id)
		entityMap[id] = entity.Name
		
		// Parse observations
		if observationsStr != "" {
			entity.Observations = strings.Split(observationsStr, "|||")
		} else {
			entity.Observations = []string{}
		}
		
		graph.Entities = append(graph.Entities, entity)
	}

	// Get relations (same as before)
	if len(entityIDs) > 0 {
		placeholders := make([]string, len(entityIDs))
		args := make([]any, 0, len(entityIDs)*2)
		
		for i, id := range entityIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		
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

// escapeFTS5 escapes special characters in FTS5 queries
func escapeFTS5(query string) string {
	// FTS5 special characters that need escaping
	specialChars := []string{"\"", "*", "-", "+", "OR", "AND", "NOT"}
	
	escaped := query
	for _, char := range specialChars {
		escaped = strings.ReplaceAll(escaped, char, "\""+char+"\"")
	}
	
	// Wrap the entire query in quotes for phrase matching
	// This ensures we search for the exact terms
	return "\"" + escaped + "\""
}

// RebuildFTSIndex rebuilds the FTS index (useful after bulk imports)
func (db *DB) RebuildFTSIndex(ctx context.Context) error {
	statements := []string{
		// Rebuild entities FTS
		`DELETE FROM entities_fts`,
		`INSERT INTO entities_fts(rowid, name, entity_type) 
		 SELECT id, name, entity_type FROM entities`,
		
		// Rebuild observations FTS
		`DELETE FROM observations_fts`,
		`INSERT INTO observations_fts(rowid, content) 
		 SELECT id, content FROM observations`,
		
		// Optimize the FTS tables
		`INSERT INTO entities_fts(entities_fts) VALUES('optimize')`,
		`INSERT INTO observations_fts(observations_fts) VALUES('optimize')`,
	}
	
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to rebuild FTS index: %w", err)
		}
	}
	
	return tx.Commit()
}