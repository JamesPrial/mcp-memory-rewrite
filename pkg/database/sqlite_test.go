package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setupTestDB(t *testing.T) *DB {
	// Use a shared in-memory SQLite database for testing to ensure the schema is not lost between connections.
	db, err := NewDB("file::memory:?cache=shared")
	assert.NoError(t, err)
	assert.NotNil(t, db)
	return db
}

func TestDBCreation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
}

func TestCreateEntities(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entities := []EntityWithObservations{
		{Name: "E1", EntityType: "T1", Observations: []string{"obs1", "obs2"}},
		{Name: "E2", EntityType: "T2", Observations: []string{"obs3"}},
	}

	created, err := db.CreateEntities(context.Background(), entities)
	assert.NoError(t, err)
	assert.Len(t, created, 2)
	assert.Equal(t, "E1", created[0].Name)

	// Test creating duplicate entities - should not create them and not error
	created, err = db.CreateEntities(context.Background(), entities)
	assert.NoError(t, err)
	assert.Len(t, created, 0, "Should not create duplicate entities")

	graph, err := db.ReadGraph(context.Background())
	assert.NoError(t, err)
	assert.Len(t, graph.Entities, 2)
}

func TestCreateRelations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entities := []EntityWithObservations{
		{Name: "E1", EntityType: "T1"},
		{Name: "E2", EntityType: "T2"},
	}
	_, err := db.CreateEntities(context.Background(), entities)
	assert.NoError(t, err)

	relations := []RelationDTO{
		{From: "E1", To: "E2", RelationType: "connects_to"},
	}

	created, err := db.CreateRelations(context.Background(), relations)
	assert.NoError(t, err)
	assert.Len(t, created, 1)
	assert.Equal(t, "connects_to", created[0].RelationType)

	// Test creating duplicate relations
	created, err = db.CreateRelations(context.Background(), relations)
	assert.NoError(t, err)
	assert.Len(t, created, 0, "Should not create duplicate relations")

	// Test relation to non-existent entity
	relations = []RelationDTO{
		{From: "E1", To: "NON_EXISTENT", RelationType: "connects_to"},
	}
	created, err = db.CreateRelations(context.Background(), relations)
	assert.NoError(t, err)
	assert.Len(t, created, 0, "Should not create relation to non-existent entity")

	graph, err := db.ReadGraph(context.Background())
	assert.NoError(t, err)
	assert.Len(t, graph.Relations, 1)
}

func TestAddObservations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entities := []EntityWithObservations{
		{Name: "E1", EntityType: "T1", Observations: []string{"obs1"}},
	}
	_, err := db.CreateEntities(context.Background(), entities)
	assert.NoError(t, err)

	additions := []struct {
		EntityName string   `json:"entityName"`
		Contents   []string `json:"contents"`
	}{
		{EntityName: "E1", Contents: []string{"obs2", "obs3"}},
	}

	added, err := db.AddObservations(context.Background(), additions)
	assert.NoError(t, err)
	assert.Len(t, added, 1)
	assert.Len(t, added[0].AddedObservations, 2)

	// Test adding duplicate observations
	added, err = db.AddObservations(context.Background(), additions)
	assert.NoError(t, err)
	assert.Len(t, added[0].AddedObservations, 0, "Should not add duplicate observations")

	// Test adding to non-existent entity
	additions = []struct {
		EntityName string   `json:"entityName"`
		Contents   []string `json:"contents"`
	}{
		{EntityName: "NON_EXISTENT", Contents: []string{"obs4"}},
	}
	_, err = db.AddObservations(context.Background(), additions)
	assert.Error(t, err, "Should error when adding to non-existent entity")

	graph, err := db.ReadGraph(context.Background())
	assert.NoError(t, err)
	assert.Len(t, graph.Entities[0].Observations, 3)
}

func TestDeleteEntities(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entities := []EntityWithObservations{
		{Name: "E1", EntityType: "T1"},
		{Name: "E2", EntityType: "T2"},
	}
	_, err := db.CreateEntities(context.Background(), entities)
	assert.NoError(t, err)

	err = db.DeleteEntities(context.Background(), []string{"E1"})
	assert.NoError(t, err)

	graph, err := db.ReadGraph(context.Background())
	assert.NoError(t, err)
	assert.Len(t, graph.Entities, 1)
	assert.Equal(t, "E2", graph.Entities[0].Name)
}

func TestDeleteObservations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	
	entities := []EntityWithObservations{
		{Name: "E1", EntityType: "T1", Observations: []string{"obs1", "obs2", "obs3"}},
	}
	_, err := db.CreateEntities(context.Background(), entities)
	assert.NoError(t, err)

	deletions := []struct {
		EntityName   string   `json:"entityName"`
		Observations []string `json:"observations"`
	}{
		{EntityName: "E1", Observations: []string{"obs1", "obs3"}},
	}
	
	err = db.DeleteObservations(context.Background(), deletions)
	assert.NoError(t, err)

	graph, err := db.ReadGraph(context.Background())
	assert.NoError(t, err)
	assert.Len(t, graph.Entities[0].Observations, 1)
	assert.Equal(t, "obs2", graph.Entities[0].Observations[0])
}

func TestDeleteRelations(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entities := []EntityWithObservations{
		{Name: "E1", EntityType: "T1"},
		{Name: "E2", EntityType: "T2"},
	}
	_, err := db.CreateEntities(context.Background(), entities)
	assert.NoError(t, err)

	relations := []RelationDTO{
		{From: "E1", To: "E2", RelationType: "connects_to"},
	}
	_, err = db.CreateRelations(context.Background(), relations)
	assert.NoError(t, err)

	err = db.DeleteRelations(context.Background(), relations)
	assert.NoError(t, err)

	graph, err := db.ReadGraph(context.Background())
	assert.NoError(t, err)
	assert.Len(t, graph.Relations, 0)
}

func TestSearchNodes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	
	entities := []EntityWithObservations{
		{Name: "Apple", EntityType: "Fruit", Observations: []string{"Red and tasty"}},
		{Name: "Banana", EntityType: "Fruit", Observations: []string{"Yellow and sweet"}},
		{Name: "Carrot", EntityType: "Vegetable", Observations: []string{"Orange and crunchy"}},
	}
	_, err := db.CreateEntities(context.Background(), entities)
	assert.NoError(t, err)

	// Search by name
	graph, err := db.SearchNodes(context.Background(), "Apple")
	assert.NoError(t, err)
	assert.Len(t, graph.Entities, 1)
	assert.Equal(t, "Apple", graph.Entities[0].Name)

	// Search by type
	graph, err = db.SearchNodes(context.Background(), "Fruit")
	assert.NoError(t, err)
	assert.Len(t, graph.Entities, 2)

	// Search by observation content
	graph, err = db.SearchNodes(context.Background(), "tasty")
	assert.NoError(t, err)
	assert.Len(t, graph.Entities, 1)
	assert.Equal(t, "Apple", graph.Entities[0].Name)

	// Search with no results
	graph, err = db.SearchNodes(context.Background(), "Zebra")
	assert.NoError(t, err)
	assert.Len(t, graph.Entities, 0)
}

func TestOpenNodes(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	entities := []EntityWithObservations{
		{Name: "E1", EntityType: "T1"},
		{Name: "E2", EntityType: "T2"},
		{Name: "E3", EntityType: "T3"},
	}
	_, err := db.CreateEntities(context.Background(), entities)
	assert.NoError(t, err)

	relations := []RelationDTO{
		{From: "E1", To: "E2", RelationType: "connects_to"},
	}
	_, err = db.CreateRelations(context.Background(), relations)
	assert.NoError(t, err)

	// Open specific nodes
	graph, err := db.OpenNodes(context.Background(), []string{"E1", "E3"})
	assert.NoError(t, err)
	assert.Len(t, graph.Entities, 2)
	assert.Len(t, graph.Relations, 0, "Should only return relations between the opened nodes")

	// Open nodes with relations between them
	graph, err = db.OpenNodes(context.Background(), []string{"E1", "E2"})
	assert.NoError(t, err)
	assert.Len(t, graph.Entities, 2)
	assert.Len(t, graph.Relations, 1)
}

