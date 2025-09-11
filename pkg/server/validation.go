package server

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	MaxEntityNameLength      = 255
	MaxEntityTypeLength      = 100
	MaxRelationTypeLength    = 100
	MaxObservationLength     = 5000
	MaxEntitiesPerRequest    = 1000
	MaxObservationsPerEntity = 100
	MaxSearchQueryLength     = 500
)

var (
	// Valid entity name pattern: alphanumeric, spaces, hyphens, underscores, dots
	entityNamePattern = regexp.MustCompile(`^[a-zA-Z0-9\s\-_.]+$`)
	
	// SQL injection patterns to block
	sqlInjectionPatterns = []string{
		"--;",
		"/*",
		"*/",
		"xp_",
		"sp_",
		"exec",
		"execute",
		"select",
		"insert",
		"update",
		"delete",
		"drop",
		"create",
		"alter",
		"union",
		"'--",
		"\"--",
	}
)

// ValidateEntityName validates an entity name
func ValidateEntityName(name string) error {
	if name == "" {
		return fmt.Errorf("entity name cannot be empty")
	}
	
	if !utf8.ValidString(name) {
		return fmt.Errorf("entity name contains invalid UTF-8 characters")
	}
	
	if len(name) > MaxEntityNameLength {
		return fmt.Errorf("entity name exceeds maximum length of %d characters", MaxEntityNameLength)
	}
	
	// Check for SQL injection patterns
	nameLower := strings.ToLower(name)
	for _, pattern := range sqlInjectionPatterns {
		if strings.Contains(nameLower, pattern) {
			return fmt.Errorf("entity name contains invalid pattern: %s", pattern)
		}
	}
	
	// Allow more flexible naming but still prevent control characters
	for _, r := range name {
		if r < 32 || r == 127 { // Control characters
			return fmt.Errorf("entity name contains control characters")
		}
	}
	
	return nil
}

// ValidateEntityType validates an entity type
func ValidateEntityType(entityType string) error {
	if entityType == "" {
		return fmt.Errorf("entity type cannot be empty")
	}
	
	if !utf8.ValidString(entityType) {
		return fmt.Errorf("entity type contains invalid UTF-8 characters")
	}
	
	if len(entityType) > MaxEntityTypeLength {
		return fmt.Errorf("entity type exceeds maximum length of %d characters", MaxEntityTypeLength)
	}
	
	// Check for SQL injection patterns
	typeLower := strings.ToLower(entityType)
	for _, pattern := range sqlInjectionPatterns {
		if strings.Contains(typeLower, pattern) {
			return fmt.Errorf("entity type contains invalid pattern: %s", pattern)
		}
	}
	
	return nil
}

// ValidateRelationType validates a relation type
func ValidateRelationType(relationType string) error {
	if relationType == "" {
		return fmt.Errorf("relation type cannot be empty")
	}
	
	if !utf8.ValidString(relationType) {
		return fmt.Errorf("relation type contains invalid UTF-8 characters")
	}
	
	if len(relationType) > MaxRelationTypeLength {
		return fmt.Errorf("relation type exceeds maximum length of %d characters", MaxRelationTypeLength)
	}
	
	// Check for SQL injection patterns
	typeLower := strings.ToLower(relationType)
	for _, pattern := range sqlInjectionPatterns {
		if strings.Contains(typeLower, pattern) {
			return fmt.Errorf("relation type contains invalid pattern: %s", pattern)
		}
	}
	
	return nil
}

// ValidateObservation validates an observation
func ValidateObservation(observation string) error {
	if observation == "" {
		return fmt.Errorf("observation cannot be empty")
	}
	
	if !utf8.ValidString(observation) {
		return fmt.Errorf("observation contains invalid UTF-8 characters")
	}
	
	if len(observation) > MaxObservationLength {
		return fmt.Errorf("observation exceeds maximum length of %d characters", MaxObservationLength)
	}
	
	return nil
}

// ValidateSearchQuery validates a search query
func ValidateSearchQuery(query string) error {
	// Empty query is allowed - returns all results
	if query == "" {
		return nil
	}
	
	if !utf8.ValidString(query) {
		return fmt.Errorf("search query contains invalid UTF-8 characters")
	}
	
	if len(query) > MaxSearchQueryLength {
		return fmt.Errorf("search query exceeds maximum length of %d characters", MaxSearchQueryLength)
	}
	
	return nil
}

// ValidateCreateEntitiesParams validates parameters for creating entities
func ValidateCreateEntitiesParams(params CreateEntitiesParams) error {
	if len(params.Entities) == 0 {
		return fmt.Errorf("no entities provided")
	}
	
	if len(params.Entities) > MaxEntitiesPerRequest {
		return fmt.Errorf("too many entities in request: %d (max %d)", len(params.Entities), MaxEntitiesPerRequest)
	}
	
	for i, entity := range params.Entities {
		if err := ValidateEntityName(entity.Name); err != nil {
			return fmt.Errorf("entity[%d].name: %w", i, err)
		}
		
		if err := ValidateEntityType(entity.EntityType); err != nil {
			return fmt.Errorf("entity[%d].entityType: %w", i, err)
		}
		
		if len(entity.Observations) > MaxObservationsPerEntity {
			return fmt.Errorf("entity[%d]: too many observations: %d (max %d)", i, len(entity.Observations), MaxObservationsPerEntity)
		}
		
		for j, obs := range entity.Observations {
			if err := ValidateObservation(obs); err != nil {
				return fmt.Errorf("entity[%d].observations[%d]: %w", i, j, err)
			}
		}
	}
	
	return nil
}

// ValidateCreateRelationsParams validates parameters for creating relations
func ValidateCreateRelationsParams(params CreateRelationsParams) error {
	if len(params.Relations) == 0 {
		return fmt.Errorf("no relations provided")
	}
	
	if len(params.Relations) > MaxEntitiesPerRequest {
		return fmt.Errorf("too many relations in request: %d (max %d)", len(params.Relations), MaxEntitiesPerRequest)
	}
	
	for i, rel := range params.Relations {
		if err := ValidateEntityName(rel.From); err != nil {
			return fmt.Errorf("relation[%d].from: %w", i, err)
		}
		
		if err := ValidateEntityName(rel.To); err != nil {
			return fmt.Errorf("relation[%d].to: %w", i, err)
		}
		
		if err := ValidateRelationType(rel.RelationType); err != nil {
			return fmt.Errorf("relation[%d].relationType: %w", i, err)
		}
	}
	
	return nil
}

// ValidateAddObservationsParams validates parameters for adding observations
func ValidateAddObservationsParams(params AddObservationsParams) error {
	if len(params.Observations) == 0 {
		return fmt.Errorf("no observations provided")
	}
	
	for i, obs := range params.Observations {
		if err := ValidateEntityName(obs.EntityName); err != nil {
			return fmt.Errorf("observations[%d].entityName: %w", i, err)
		}
		
		if len(obs.Contents) == 0 {
			return fmt.Errorf("observations[%d]: no contents provided", i)
		}
		
		if len(obs.Contents) > MaxObservationsPerEntity {
			return fmt.Errorf("observations[%d]: too many observations: %d (max %d)", i, len(obs.Contents), MaxObservationsPerEntity)
		}
		
		for j, content := range obs.Contents {
			if err := ValidateObservation(content); err != nil {
				return fmt.Errorf("observations[%d].contents[%d]: %w", i, j, err)
			}
		}
	}
	
	return nil
}

// ValidateDeleteEntitiesParams validates parameters for deleting entities
func ValidateDeleteEntitiesParams(params DeleteEntitiesParams) error {
	if len(params.EntityNames) == 0 {
		return fmt.Errorf("no entity names provided")
	}
	
	if len(params.EntityNames) > MaxEntitiesPerRequest {
		return fmt.Errorf("too many entities to delete: %d (max %d)", len(params.EntityNames), MaxEntitiesPerRequest)
	}
	
	for i, name := range params.EntityNames {
		if err := ValidateEntityName(name); err != nil {
			return fmt.Errorf("entityNames[%d]: %w", i, err)
		}
	}
	
	return nil
}

// ValidateSearchNodesParams validates parameters for searching nodes
func ValidateSearchNodesParams(params SearchNodesParams) error {
	return ValidateSearchQuery(params.Query)
}

// ValidateOpenNodesParams validates parameters for opening nodes
func ValidateOpenNodesParams(params OpenNodesParams) error {
	// Empty list is allowed - returns empty graph
	if len(params.Names) == 0 {
		return nil
	}
	
	if len(params.Names) > MaxEntitiesPerRequest {
		return fmt.Errorf("too many nodes to open: %d (max %d)", len(params.Names), MaxEntitiesPerRequest)
	}
	
	for i, name := range params.Names {
		if err := ValidateEntityName(name); err != nil {
			return fmt.Errorf("names[%d]: %w", i, err)
		}
	}
	
	return nil
}