package server

import (
    "context"
    "encoding/json"
    "testing"

    "github.com/jamesprial/mcp-memory-rewrite/pkg/database"
    "github.com/modelcontextprotocol/go-sdk/mcp"
    "github.com/stretchr/testify/assert"
)

// helper to create a test server backed by shared in-memory sqlite
func newTestServer(t *testing.T) (*Server, *database.DB) {
    db, err := database.NewDB("file::memory:?cache=shared")
    assert.NoError(t, err)
    srv := NewServer(db)
    t.Cleanup(func() { _ = db.Close() })
    return srv, db
}

func jsonText(t *testing.T, res *mcp.CallToolResult) string {
    assert.NotNil(t, res)
    assert.NotEmpty(t, res.Content)
    tc, ok := res.Content[0].(*mcp.TextContent)
    assert.True(t, ok)
    return tc.Text
}

func TestServer_CreateEntities_AndReadGraph(t *testing.T) {
    s, _ := newTestServer(t)

    // create two entities with observations
    res, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{
        {Name: "E1", EntityType: "T1", Observations: []string{"o1", "o2"}},
        {Name: "E2", EntityType: "T2"},
    }})
    assert.NoError(t, err)

    var created []database.EntityWithObservations
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &created))
    assert.Len(t, created, 2)

    // read graph
    res, _, err = s.handleReadGraph(context.Background())
    assert.NoError(t, err)
    var g database.KnowledgeGraph
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
    assert.Len(t, g.Entities, 2)
}

func TestServer_CreateEntities_Table(t *testing.T) {
    cases := []struct{
        name     string
        seed     []database.EntityWithObservations
        input    []database.EntityWithObservations
        wantLen  int
    }{
        {
            name:    "one new",
            seed:    nil,
            input:   []database.EntityWithObservations{{Name: "E1", EntityType: "T1"}},
            wantLen: 1,
        },
        {
            name:    "duplicate no-op",
            seed:    []database.EntityWithObservations{{Name: "E1", EntityType: "T1"}},
            input:   []database.EntityWithObservations{{Name: "E1", EntityType: "T1"}},
            wantLen: 0,
        },
        {
            name:    "multiple with observations",
            seed:    nil,
            input:   []database.EntityWithObservations{{Name: "E1", EntityType: "T1", Observations: []string{"a","b"}}, {Name: "E2", EntityType: "T2"}},
            wantLen: 2,
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            s, _ := newTestServer(t)
            if len(tc.seed) > 0 {
                _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: tc.seed})
                assert.NoError(t, err)
            }
            res, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: tc.input})
            assert.NoError(t, err)
            var created []database.EntityWithObservations
            assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &created))
            assert.Len(t, created, tc.wantLen)
        })
    }
}

func TestServer_AddObservations_MixedAndError(t *testing.T) {
    s, _ := newTestServer(t)

    // seed
    _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{{Name: "E1", EntityType: "T1", Observations: []string{"o1"}}}})
    assert.NoError(t, err)

    // add mixture: existing and duplicates within the same call
    res, _, err := s.handleAddObservations(context.Background(), AddObservationsParams{Observations: []ObservationInput{{
        EntityName: "E1",
        Contents:   []string{"o1", "o2", "o2"},
    }}})
    assert.NoError(t, err)
    var added []struct {
        EntityName        string   `json:"entityName"`
        AddedObservations []string `json:"addedObservations"`
    }
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &added))
    assert.Len(t, added, 1)
    assert.Equal(t, []string{"o2"}, added[0].AddedObservations)

    // error for unknown entity
    _, _, err = s.handleAddObservations(context.Background(), AddObservationsParams{Observations: []ObservationInput{{
        EntityName: "MISSING",
        Contents:   []string{"z"},
    }}})
    assert.Error(t, err)
}

func TestServer_AddObservations_Table(t *testing.T) {
    type obsRes struct{
        EntityName string   `json:"entityName"`
        AddedObservations []string `json:"addedObservations"`
    }

    cases := []struct{
        name      string
        seed      []database.EntityWithObservations
        input     []ObservationInput
        want      []obsRes
        wantErr   bool
    }{
        {
            name: "existing with duplicates yields uniques",
            seed: []database.EntityWithObservations{{Name: "E1", EntityType: "T1", Observations: []string{"o1"}}},
            input: []ObservationInput{{EntityName: "E1", Contents: []string{"o1","o2","o2"}}},
            want: []obsRes{{EntityName: "E1", AddedObservations: []string{"o2"}}},
        },
        {
            name: "unknown entity errors",
            seed: nil,
            input: []ObservationInput{{EntityName: "MISS", Contents: []string{"z"}}},
            wantErr: true,
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            s, _ := newTestServer(t)
            if len(tc.seed) > 0 {
                _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: tc.seed})
                assert.NoError(t, err)
            }
            res, _, err := s.handleAddObservations(context.Background(), AddObservationsParams{Observations: tc.input})
            if tc.wantErr {
                assert.Error(t, err)
                return
            }
            assert.NoError(t, err)
            var got []obsRes
            assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &got))
            assert.Equal(t, tc.want, got)
        })
    }
}

func TestServer_CreateRelations_Edges(t *testing.T) {
    s, _ := newTestServer(t)
    // seed
    _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}}})
    assert.NoError(t, err)

    // self relation allowed
    res, _, err := s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: []database.RelationDTO{{From: "A", To: "A", RelationType: "self"}}})
    assert.NoError(t, err)
    var created []database.RelationDTO
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &created))
    assert.Len(t, created, 1)

    // duplicate no-op
    res, _, err = s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: []database.RelationDTO{{From: "A", To: "A", RelationType: "self"}}})
    assert.NoError(t, err)
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &created))
    assert.Len(t, created, 0)

    // missing endpoint no-op
    res, _, err = s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: []database.RelationDTO{{From: "A", To: "C", RelationType: "rel"}}})
    assert.NoError(t, err)
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &created))
    assert.Len(t, created, 0)
}

func TestServer_CreateRelations_Table(t *testing.T) {
    cases := []struct{
        name     string
        seed     []database.EntityWithObservations
        preRels  []database.RelationDTO
        input    []database.RelationDTO
        wantLen  int
    }{
        {
            name:    "normal relation",
            seed:    []database.EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}},
            input:   []database.RelationDTO{{From: "A", To: "B", RelationType: "rel"}},
            wantLen: 1,
        },
        {
            name:    "duplicate no-op",
            seed:    []database.EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}},
            preRels: []database.RelationDTO{{From: "A", To: "B", RelationType: "rel"}},
            input:   []database.RelationDTO{{From: "A", To: "B", RelationType: "rel"}},
            wantLen: 0,
        },
        {
            name:    "missing endpoint no-op",
            seed:    []database.EntityWithObservations{{Name: "A", EntityType: "T"}},
            input:   []database.RelationDTO{{From: "A", To: "C", RelationType: "rel"}},
            wantLen: 0,
        },
        {
            name:    "self relation",
            seed:    []database.EntityWithObservations{{Name: "A", EntityType: "T"}},
            input:   []database.RelationDTO{{From: "A", To: "A", RelationType: "self"}},
            wantLen: 1,
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            s, _ := newTestServer(t)
            _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: tc.seed})
            assert.NoError(t, err)
            if len(tc.preRels) > 0 {
                _, _, err = s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: tc.preRels})
                assert.NoError(t, err)
            }
            res, _, err := s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: tc.input})
            assert.NoError(t, err)
            var created []database.RelationDTO
            assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &created))
            assert.Len(t, created, tc.wantLen)
        })
    }
}

func TestServer_DeleteEntities_Cascade(t *testing.T) {
    s, _ := newTestServer(t)
    // seed
    _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{
        {Name: "A", EntityType: "T", Observations: []string{"x"}},
        {Name: "B", EntityType: "T"},
    }})
    assert.NoError(t, err)
    _, _, err = s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: []database.RelationDTO{{From: "A", To: "B", RelationType: "rel"}}})
    assert.NoError(t, err)

    // delete A
    res, _, err := s.handleDeleteEntities(context.Background(), DeleteEntitiesParams{EntityNames: []string{"A"}})
    assert.NoError(t, err)
    assert.Contains(t, jsonText(t, res), "successfully")

    // read graph
    res, _, err = s.handleReadGraph(context.Background())
    assert.NoError(t, err)
    var g database.KnowledgeGraph
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
    assert.Len(t, g.Entities, 1)
    assert.Equal(t, "B", g.Entities[0].Name)
    assert.Len(t, g.Relations, 0)
}

func TestServer_DeleteEntities_Table(t *testing.T) {
    cases := []struct{
        name       string
        delete     []string
        wantNames  []string
        wantRelLen int
    }{
        {
            name:       "delete existing cascades",
            delete:     []string{"A"},
            wantNames:  []string{"B"},
            wantRelLen: 0,
        },
        {
            name:       "delete missing is noop",
            delete:     []string{"C"},
            wantNames:  []string{"A","B"},
            wantRelLen: 1,
        },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            s, _ := newTestServer(t)
            // seed
            _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{{Name: "A", EntityType: "T", Observations: []string{"x"}}, {Name: "B", EntityType: "T"}}})
            assert.NoError(t, err)
            _, _, err = s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: []database.RelationDTO{{From: "A", To: "B", RelationType: "rel"}}})
            assert.NoError(t, err)

            _, _, err = s.handleDeleteEntities(context.Background(), DeleteEntitiesParams{EntityNames: tc.delete})
            assert.NoError(t, err)

            res, _, err := s.handleReadGraph(context.Background())
            assert.NoError(t, err)
            var g database.KnowledgeGraph
            assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
            // collect names
            gotNames := make([]string, 0, len(g.Entities))
            for _, e := range g.Entities { gotNames = append(gotNames, e.Name) }
            assert.ElementsMatch(t, tc.wantNames, gotNames)
            assert.Len(t, g.Relations, tc.wantRelLen)
        })
    }
}

func TestServer_DeleteObservations_Scenarios(t *testing.T) {
    s, _ := newTestServer(t)
    // seed
    _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{{Name: "A", EntityType: "T", Observations: []string{"o1", "o2"}}}})
    assert.NoError(t, err)

    // delete existing and a missing one
    res, _, err := s.handleDeleteObservations(context.Background(), DeleteObservationsParams{Deletions: []DeletionInput{{EntityName: "A", Observations: []string{"o1", "nope"}}}})
    assert.NoError(t, err)
    assert.Contains(t, jsonText(t, res), "successfully")

    // unknown entity should be a no-op
    res, _, err = s.handleDeleteObservations(context.Background(), DeleteObservationsParams{Deletions: []DeletionInput{{EntityName: "UNKNOWN", Observations: []string{"x"}}}})
    assert.NoError(t, err)
    assert.Contains(t, jsonText(t, res), "successfully")
}

func TestServer_DeleteObservations_Table(t *testing.T) {
    cases := []struct{
        name      string
        deletions []DeletionInput
        wantObs   []string
    }{
        { name: "delete existing", deletions: []DeletionInput{{EntityName: "A", Observations: []string{"o1"}}}, wantObs: []string{"o2"}},
        { name: "delete unknown observation", deletions: []DeletionInput{{EntityName: "A", Observations: []string{"nope"}}}, wantObs: []string{"o1","o2"}},
        { name: "unknown entity noop", deletions: []DeletionInput{{EntityName: "UNKNOWN", Observations: []string{"x"}}}, wantObs: []string{"o1","o2"}},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            s, _ := newTestServer(t)
            _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{{Name: "A", EntityType: "T", Observations: []string{"o1","o2"}}}})
            assert.NoError(t, err)

            _, _, err = s.handleDeleteObservations(context.Background(), DeleteObservationsParams{Deletions: tc.deletions})
            assert.NoError(t, err)

            res, _, err := s.handleOpenNodes(context.Background(), OpenNodesParams{Names: []string{"A"}})
            assert.NoError(t, err)
            var g database.KnowledgeGraph
            assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
            assert.ElementsMatch(t, tc.wantObs, g.Entities[0].Observations)
        })
    }
}

func TestServer_DeleteRelations_NoopsAndDelete(t *testing.T) {
    s, _ := newTestServer(t)
    // seed
    _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}}})
    assert.NoError(t, err)
    _, _, err = s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: []database.RelationDTO{{From: "A", To: "B", RelationType: "rel"}}})
    assert.NoError(t, err)

    // delete missing relation (no-op)
    res, _, err := s.handleDeleteRelations(context.Background(), DeleteRelationsParams{Relations: []database.RelationDTO{{From: "A", To: "B", RelationType: "other"}}})
    assert.NoError(t, err)
    assert.Contains(t, jsonText(t, res), "successfully")

    // delete existing relation
    res, _, err = s.handleDeleteRelations(context.Background(), DeleteRelationsParams{Relations: []database.RelationDTO{{From: "A", To: "B", RelationType: "rel"}}})
    assert.NoError(t, err)
    assert.Contains(t, jsonText(t, res), "successfully")
}

func TestServer_DeleteRelations_Table(t *testing.T) {
    cases := []struct{
        name      string
        deletions []database.RelationDTO
        wantRel   int
    }{
        { name: "delete missing type noop", deletions: []database.RelationDTO{{From: "A", To: "B", RelationType: "other"}}, wantRel: 1 },
        { name: "delete existing", deletions: []database.RelationDTO{{From: "A", To: "B", RelationType: "rel"}}, wantRel: 0 },
        { name: "missing endpoint noop", deletions: []database.RelationDTO{{From: "A", To: "C", RelationType: "rel"}}, wantRel: 1 },
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            s, _ := newTestServer(t)
            _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{{Name: "A", EntityType: "T"}, {Name: "B", EntityType: "T"}}})
            assert.NoError(t, err)
            _, _, err = s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: []database.RelationDTO{{From: "A", To: "B", RelationType: "rel"}}})
            assert.NoError(t, err)

            _, _, err = s.handleDeleteRelations(context.Background(), DeleteRelationsParams{Relations: tc.deletions})
            assert.NoError(t, err)

            res, _, err := s.handleReadGraph(context.Background())
            assert.NoError(t, err)
            var g database.KnowledgeGraph
            assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
            assert.Len(t, g.Relations, tc.wantRel)
        })
    }
}

func TestServer_SearchNodes_Edges(t *testing.T) {
    s, _ := newTestServer(t)
    _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{
        {Name: "Apple", EntityType: "Fruit", Observations: []string{"Red and tasty"}},
        {Name: "Banana", EntityType: "Fruit", Observations: []string{"Yellow and sweet"}},
    }})
    assert.NoError(t, err)

    // case-insensitive search
    res, _, err := s.handleSearchNodes(context.Background(), SearchNodesParams{Query: "apple"})
    assert.NoError(t, err)
    var g database.KnowledgeGraph
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
    assert.Len(t, g.Entities, 1)
    assert.Equal(t, "Apple", g.Entities[0].Name)

    // empty query returns all
    res, _, err = s.handleSearchNodes(context.Background(), SearchNodesParams{Query: ""})
    assert.NoError(t, err)
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
    assert.GreaterOrEqual(t, len(g.Entities), 2)
}

func TestServer_SearchNodes_Table(t *testing.T) {
    s, _ := newTestServer(t)
    _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{
        {Name: "Apple", EntityType: "Fruit", Observations: []string{"Red and tasty"}},
        {Name: "Banana", EntityType: "Fruit", Observations: []string{"Yellow and sweet"}},
    }})
    assert.NoError(t, err)

    cases := []struct{
        name   string
        query  string
        wantCt int
    }{
        {name: "case-insensitive", query: "apple", wantCt: 1},
        {name: "empty returns all", query: "", wantCt: 2},
        {name: "unmatched", query: "zebra", wantCt: 0},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            res, _, err := s.handleSearchNodes(context.Background(), SearchNodesParams{Query: tc.query})
            assert.NoError(t, err)
            var g database.KnowledgeGraph
            assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
            assert.Len(t, g.Entities, tc.wantCt)
        })
    }
}

func TestServer_OpenNodes_Edges(t *testing.T) {
    s, _ := newTestServer(t)
    _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{{Name: "E1", EntityType: "T"}, {Name: "E2", EntityType: "T"}, {Name: "E3", EntityType: "T"}}})
    assert.NoError(t, err)
    _, _, err = s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: []database.RelationDTO{{From: "E1", To: "E2", RelationType: "rel"}}})
    assert.NoError(t, err)

    // open two with no relation between them
    res, _, err := s.handleOpenNodes(context.Background(), OpenNodesParams{Names: []string{"E1", "E3"}})
    assert.NoError(t, err)
    var g database.KnowledgeGraph
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
    assert.Len(t, g.Entities, 2)
    assert.Len(t, g.Relations, 0)

    // duplicates and unknown filtered
    res, _, err = s.handleOpenNodes(context.Background(), OpenNodesParams{Names: []string{"E1", "E1", "unknown"}})
    assert.NoError(t, err)
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
    assert.Len(t, g.Entities, 1)
    assert.Equal(t, "E1", g.Entities[0].Name)

    // empty input
    res, _, err = s.handleOpenNodes(context.Background(), OpenNodesParams{Names: nil})
    assert.NoError(t, err)
    assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
    assert.Len(t, g.Entities, 0)
    assert.Len(t, g.Relations, 0)
}

func TestServer_OpenNodes_Table(t *testing.T) {
    s, _ := newTestServer(t)
    _, _, err := s.handleCreateEntities(context.Background(), CreateEntitiesParams{Entities: []database.EntityWithObservations{{Name: "E1", EntityType: "T"}, {Name: "E2", EntityType: "T"}, {Name: "E3", EntityType: "T"}}})
    assert.NoError(t, err)
    _, _, err = s.handleCreateRelations(context.Background(), CreateRelationsParams{Relations: []database.RelationDTO{{From: "E1", To: "E2", RelationType: "rel"}}})
    assert.NoError(t, err)

    cases := []struct{
        name     string
        names    []string
        wantEnt  int
        wantRel  int
    }{
        {name: "no relation among opened", names: []string{"E1","E3"}, wantEnt: 2, wantRel: 0},
        {name: "duplicates and unknown filtered", names: []string{"E1","E1","unknown"}, wantEnt: 1, wantRel: 0},
        {name: "empty", names: nil, wantEnt: 0, wantRel: 0},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            res, _, err := s.handleOpenNodes(context.Background(), OpenNodesParams{Names: tc.names})
            assert.NoError(t, err)
            var g database.KnowledgeGraph
            assert.NoError(t, json.Unmarshal([]byte(jsonText(t, res)), &g))
            assert.Len(t, g.Entities, tc.wantEnt)
            assert.Len(t, g.Relations, tc.wantRel)
        })
    }
}

func TestServer_Shutdown_ClosesDB(t *testing.T) {
    s, db := newTestServer(t)
    assert.NoError(t, s.Shutdown(context.Background()))
    // subsequent DB ops should fail since the connection is closed
    _, err := db.ReadGraph(context.Background())
    assert.Error(t, err)
}

func TestServer_RegisterTools_Smoke(t *testing.T) {
    s, _ := newTestServer(t)
    m := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
    // should not panic or error when registering tools
    s.RegisterTools(m)
}
