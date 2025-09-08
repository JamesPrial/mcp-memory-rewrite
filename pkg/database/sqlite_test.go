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

    additions := []ObservationAdditionInput{{EntityName: "E1", Contents: []string{"obs2", "obs3"}}}

	added, err := db.AddObservations(context.Background(), additions)
	assert.NoError(t, err)
	assert.Len(t, added, 1)
	assert.Len(t, added[0].AddedObservations, 2)

	// Test adding duplicate observations
	added, err = db.AddObservations(context.Background(), additions)
	assert.NoError(t, err)
	assert.Len(t, added[0].AddedObservations, 0, "Should not add duplicate observations")

	// Test adding to non-existent entity
    additions = []ObservationAdditionInput{{EntityName: "NON_EXISTENT", Contents: []string{"obs4"}}}
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

    deletions := []ObservationDeletionInput{{EntityName: "E1", Observations: []string{"obs1", "obs3"}}}
	
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

func TestDB_CreateEntities_Table(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    cases := []struct{
        name    string
        input   []EntityWithObservations
        wantLen int
    }{
        {name: "empty input nil", input: nil, wantLen: 0},
        {name: "empty input slice", input: []EntityWithObservations{}, wantLen: 0},
        {name: "one entity", input: []EntityWithObservations{{Name: "E1", EntityType: "T"}}, wantLen: 1},
        {name: "duplicates in second call", input: []EntityWithObservations{{Name: "E1", EntityType: "T"}}, wantLen: 0},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            created, err := db.CreateEntities(context.Background(), tc.input)
            assert.NoError(t, err)
            assert.Len(t, created, tc.wantLen)
        })
    }
}

func TestDB_CreateRelations_Table(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}})
    assert.NoError(t, err)

    cases := []struct{
        name    string
        input   []RelationDTO
        wantLen int
    }{
        {name: "normal", input: []RelationDTO{{From: "A", To: "B", RelationType: "rel"}}, wantLen: 1},
        {name: "duplicate", input: []RelationDTO{{From: "A", To: "B", RelationType: "rel"}}, wantLen: 0},
        {name: "missing endpoint", input: []RelationDTO{{From: "A", To: "C", RelationType: "rel"}}, wantLen: 0},
        {name: "self relation", input: []RelationDTO{{From: "A", To: "A", RelationType: "self"}}, wantLen: 1},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            created, err := db.CreateRelations(context.Background(), tc.input)
            assert.NoError(t, err)
            assert.Len(t, created, tc.wantLen)
        })
    }
}

func TestDB_AddObservations_Table(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "E1", EntityType: "T", Observations: []string{"o1"}}})
    assert.NoError(t, err)

    type in struct{ entity string; contents []string }
    cases := []struct{
        name    string
        input   []in
        want    map[string][]string
        wantErr bool
    }{
        {name: "add uniques", input: []in{{entity: "E1", contents: []string{"o2","o3"}}}, want: map[string][]string{"E1": {"o2","o3"}}},
        {name: "duplicates within call", input: []in{{entity: "E1", contents: []string{"o3","o3"}}}, want: map[string][]string{"E1": {}}},
        {name: "missing entity", input: []in{{entity: "MISS", contents: []string{"x"}}}, wantErr: true},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            // build argument using named type
            arg := make([]ObservationAdditionInput, len(tc.input))
            for i, v := range tc.input {
                arg[i] = ObservationAdditionInput{EntityName: v.entity, Contents: v.contents}
            }
            got, err := db.AddObservations(context.Background(), arg)
            if tc.wantErr { assert.Error(t, err); return }
            assert.NoError(t, err)
            // map results for comparison
            m := make(map[string][]string)
            for _, r := range got { m[r.EntityName] = r.AddedObservations }
            assert.Equal(t, tc.want, m)
        })
    }
}

func TestDB_DeleteEntities_Table(t *testing.T) {
    cases := []struct{
        name    string
        delete  []string
        wantEnt []string
        wantRel int
    }{
        {name: "delete A cascades", delete: []string{"A"}, wantEnt: []string{"B"}, wantRel: 0},
        {name: "delete missing noop", delete: []string{"C"}, wantEnt: []string{"A","B"}, wantRel: 1},
        {name: "delete none", delete: nil, wantEnt: []string{"A","B"}, wantRel: 1},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            db := setupTestDB(t)
            defer db.Close()
            _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T", Observations: []string{"x"}}, {Name: "B", EntityType: "T"}})
            assert.NoError(t, err)
            _, err = db.CreateRelations(context.Background(), []RelationDTO{{From: "A", To: "B", RelationType: "rel"}})
            assert.NoError(t, err)

            err = db.DeleteEntities(context.Background(), tc.delete)
            assert.NoError(t, err)
            g, err := db.ReadGraph(context.Background())
            assert.NoError(t, err)
            names := make([]string, 0, len(g.Entities))
            for _, e := range g.Entities { names = append(names, e.Name) }
            assert.ElementsMatch(t, tc.wantEnt, names)
            assert.Len(t, g.Relations, tc.wantRel)
        })
    }
}

func TestDB_DeleteObservations_Table(t *testing.T) {
    type del struct{ entity string; obs []string }
    cases := []struct{
        name    string
        del     []del
        wantObs []string
    }{
        {name: "delete existing", del: []del{{entity: "A", obs: []string{"o1"}}}, wantObs: []string{"o2"}},
        {name: "delete unknown observation", del: []del{{entity: "A", obs: []string{"nope"}}}, wantObs: []string{"o1","o2"}},
        {name: "unknown entity noop", del: []del{{entity: "MISSING", obs: []string{"x"}}}, wantObs: []string{"o1","o2"}},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            db := setupTestDB(t)
            defer db.Close()
            _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T", Observations: []string{"o1","o2"}}})
            assert.NoError(t, err)
            // build arg using named type
            arg := make([]ObservationDeletionInput, len(tc.del))
            for i, v := range tc.del {
                arg[i] = ObservationDeletionInput{EntityName: v.entity, Observations: v.obs}
            }
            err = db.DeleteObservations(context.Background(), arg)
            assert.NoError(t, err)
            g, err := db.OpenNodes(context.Background(), []string{"A"})
            assert.NoError(t, err)
            assert.ElementsMatch(t, tc.wantObs, g.Entities[0].Observations)
        })
    }
}

func TestDB_DeleteRelations_Table(t *testing.T) {
    cases := []struct{
        name  string
        del   []RelationDTO
        wantR int
    }{
        {name: "delete missing type", del: []RelationDTO{{From: "A", To: "B", RelationType: "other"}}, wantR: 1},
        {name: "delete existing", del: []RelationDTO{{From: "A", To: "B", RelationType: "rel"}}, wantR: 0},
        {name: "delete missing entity", del: []RelationDTO{{From: "A", To: "C", RelationType: "rel"}}, wantR: 1},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            db := setupTestDB(t)
            defer db.Close()
            _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}})
            assert.NoError(t, err)
            _, err = db.CreateRelations(context.Background(), []RelationDTO{{From: "A", To: "B", RelationType: "rel"}})
            assert.NoError(t, err)

            err = db.DeleteRelations(context.Background(), tc.del)
            assert.NoError(t, err)
            g, err := db.ReadGraph(context.Background())
            assert.NoError(t, err)
            assert.Len(t, g.Relations, tc.wantR)
        })
    }
}

func TestDB_SearchNodes_Table(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "Apple", EntityType: "Fruit", Observations: []string{"Red and tasty"}}, {Name: "Banana", EntityType: "Fruit", Observations: []string{"Yellow and sweet"}}})
    assert.NoError(t, err)

    cases := []struct{
        name string
        q    string
        want int
    }{
        {name: "by name ci", q: "apple", want: 1},
        {name: "by type", q: "Fruit", want: 2},
        {name: "by obs", q: "tasty", want: 1},
        {name: "none", q: "zebra", want: 0},
        {name: "empty all", q: "", want: 2},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            g, err := db.SearchNodes(context.Background(), tc.q)
            assert.NoError(t, err)
            assert.Len(t, g.Entities, tc.want)
        })
    }
}

func TestDB_OpenNodes_Table(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()
    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "E1", EntityType: "T"}, {Name: "E2", EntityType: "T"}, {Name: "E3", EntityType: "T"}})
    assert.NoError(t, err)
    _, err = db.CreateRelations(context.Background(), []RelationDTO{{From: "E1", To: "E2", RelationType: "rel"}})
    assert.NoError(t, err)

    cases := []struct{
        name string
        in   []string
        wantE int
        wantR int
    }{
        {name: "two no relation", in: []string{"E1","E3"}, wantE: 2, wantR: 0},
        {name: "dup and unknown filtered", in: []string{"E1","E1","unknown"}, wantE: 1, wantR: 0},
        {name: "empty", in: nil, wantE: 0, wantR: 0},
        {name: "with relation", in: []string{"E1","E2"}, wantE: 2, wantR: 1},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            g, err := db.OpenNodes(context.Background(), tc.in)
            assert.NoError(t, err)
            assert.Len(t, g.Entities, tc.wantE)
            assert.Len(t, g.Relations, tc.wantR)
        })
    }
}

func TestCreateEntities_EmptyInput(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    created, err := db.CreateEntities(context.Background(), nil)
    assert.NoError(t, err)
    assert.Len(t, created, 0)

    created, err = db.CreateEntities(context.Background(), []EntityWithObservations{})
    assert.NoError(t, err)
    assert.Len(t, created, 0)
}

func TestCreateRelations_SelfRelationAllowed(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "NodeA", EntityType: "Type"}})
    assert.NoError(t, err)

    created, err := db.CreateRelations(context.Background(), []RelationDTO{{From: "NodeA", To: "NodeA", RelationType: "self"}})
    assert.NoError(t, err)
    assert.Len(t, created, 1)

    g, err := db.ReadGraph(context.Background())
    assert.NoError(t, err)
    assert.Len(t, g.Relations, 1)
    assert.Equal(t, "NodeA", g.Relations[0].From)
    assert.Equal(t, "NodeA", g.Relations[0].To)
}

func TestDeleteEntities_CascadesToObservationsAndRelations(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{
        {Name: "A", EntityType: "T", Observations: []string{"o1", "o2"}},
        {Name: "B", EntityType: "T"},
    })
    assert.NoError(t, err)

    _, err = db.CreateRelations(context.Background(), []RelationDTO{{From: "A", To: "B", RelationType: "rel"}})
    assert.NoError(t, err)

    // Delete A and ensure its observations and the relation are gone
    err = db.DeleteEntities(context.Background(), []string{"A"})
    assert.NoError(t, err)

    g, err := db.ReadGraph(context.Background())
    assert.NoError(t, err)
    assert.Len(t, g.Entities, 1)
    assert.Equal(t, "B", g.Entities[0].Name)
    assert.Len(t, g.Relations, 0)
}

func TestDeleteObservations_NonexistentIsNoop(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T", Observations: []string{"x"}}})
    assert.NoError(t, err)

    err = db.DeleteObservations(context.Background(), []ObservationDeletionInput{{EntityName: "A", Observations: []string{"does-not-exist"}}})
    assert.NoError(t, err)

    g, err := db.ReadGraph(context.Background())
    assert.NoError(t, err)
    assert.Len(t, g.Entities, 1)
    assert.Equal(t, []string{"x"}, g.Entities[0].Observations)
}

func TestDeleteRelations_MissingIsNoop(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}})
    assert.NoError(t, err)

    // delete a relation that doesn't exist
    err = db.DeleteRelations(context.Background(), []RelationDTO{{From: "A", To: "B", RelationType: "missing"}})
    assert.NoError(t, err)
}

func TestSearchNodes_EmptyQueryReturnsAll(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}})
    assert.NoError(t, err)

    _, err = db.CreateRelations(context.Background(), []RelationDTO{{From: "A", To: "B", RelationType: "rel"}})
    assert.NoError(t, err)

    gAll, err := db.ReadGraph(context.Background())
    assert.NoError(t, err)

    gSearch, err := db.SearchNodes(context.Background(), "")
    assert.NoError(t, err)
    assert.Len(t, gSearch.Entities, len(gAll.Entities))
    assert.Len(t, gSearch.Relations, len(gAll.Relations))
}

func TestSearchNodes_CaseInsensitivity(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "Apple", EntityType: "Fruit", Observations: []string{"Tasty"}}})
    assert.NoError(t, err)

    g, err := db.SearchNodes(context.Background(), "apple")
    assert.NoError(t, err)
    assert.Len(t, g.Entities, 1)
    assert.Equal(t, "Apple", g.Entities[0].Name)
}

func TestOpenNodes_EmptyInput(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T"}})
    assert.NoError(t, err)

    g, err := db.OpenNodes(context.Background(), nil)
    assert.NoError(t, err)
    assert.Len(t, g.Entities, 0)
    assert.Len(t, g.Relations, 0)
}

func TestOpenNodes_UnknownAndDuplicateNames(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}})
    assert.NoError(t, err)

    g, err := db.OpenNodes(context.Background(), []string{"A", "A", "C"})
    assert.NoError(t, err)
    assert.Len(t, g.Entities, 1)
    assert.Equal(t, "A", g.Entities[0].Name)
}

func TestAddObservations_DuplicateWithinSingleCall(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    _, err := db.CreateEntities(context.Background(), []EntityWithObservations{{Name: "A", EntityType: "T"}})
    assert.NoError(t, err)

    added, err := db.AddObservations(context.Background(), []ObservationAdditionInput{{EntityName: "A", Contents: []string{"dup", "dup"}}})
    assert.NoError(t, err)
    assert.Len(t, added, 1)
    assert.Equal(t, []string{"dup"}, added[0].AddedObservations)

    // Verify persisted once
    g, err := db.ReadGraph(context.Background())
    assert.NoError(t, err)
    assert.Equal(t, []string{"dup"}, g.Entities[0].Observations)
}
