package database

import (
	"time"
)

type Entity struct {
	ID         int64     `json:"id" db:"id"`
	Name       string    `json:"name" db:"name"`
	EntityType string    `json:"entityType" db:"entity_type"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt  time.Time `json:"updatedAt" db:"updated_at"`
}

type Observation struct {
	ID        int64     `json:"id" db:"id"`
	EntityID  int64     `json:"entityId" db:"entity_id"`
	Content   string    `json:"content" db:"content"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

type Relation struct {
	ID           int64     `json:"id" db:"id"`
	FromEntityID int64     `json:"fromEntityId" db:"from_entity_id"`
	ToEntityID   int64     `json:"toEntityId" db:"to_entity_id"`
	RelationType string    `json:"relationType" db:"relation_type"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
}

type EntityWithObservations struct {
	Name         string   `json:"name"`
	EntityType   string   `json:"entityType"`
	Observations []string `json:"observations"`
}

type RelationDTO struct {
	From         string `json:"from"`
	To           string `json:"to"`
	RelationType string `json:"relationType"`
}

type KnowledgeGraph struct {
    Entities  []EntityWithObservations `json:"entities"`
    Relations []RelationDTO            `json:"relations"`
}

// Named types to replace anonymous structs in DB APIs for ergonomics
type ObservationAdditionInput struct {
    EntityName string   `json:"entityName"`
    Contents   []string `json:"contents"`
}

type ObservationAdditionResult struct {
    EntityName        string   `json:"entityName"`
    AddedObservations []string `json:"addedObservations"`
}

type ObservationDeletionInput struct {
    EntityName   string   `json:"entityName"`
    Observations []string `json:"observations"`
}
