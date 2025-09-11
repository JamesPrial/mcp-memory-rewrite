package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
)

// setupBenchDB creates a test database with the specified number of entities
func setupBenchDB(b *testing.B, entityCount int) *DB {
	b.Helper()
	
	// Use in-memory database for benchmarks
	db, err := NewDBWithLogger("file::memory:?cache=shared", slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	if err != nil {
		b.Fatal(err)
	}
	
	ctx := context.Background()
	
	// Create test entities
	entities := make([]EntityWithObservations, entityCount)
	for i := 0; i < entityCount; i++ {
		entities[i] = EntityWithObservations{
			Name:       fmt.Sprintf("entity_%d", i),
			EntityType: fmt.Sprintf("type_%d", i%10),
			Observations: []string{
				fmt.Sprintf("observation_1_for_entity_%d", i),
				fmt.Sprintf("observation_2_for_entity_%d", i),
				fmt.Sprintf("test data with searchable content %d", i),
			},
		}
	}
	
	// Batch create entities
	batchSize := 100
	for i := 0; i < len(entities); i += batchSize {
		end := i + batchSize
		if end > len(entities) {
			end = len(entities)
		}
		if _, err := db.CreateEntities(ctx, entities[i:end]); err != nil {
			b.Fatal(err)
		}
	}
	
	// Create some relations
	relations := []RelationDTO{}
	for i := 0; i < entityCount/2; i++ {
		relations = append(relations, RelationDTO{
			From:         fmt.Sprintf("entity_%d", i),
			To:           fmt.Sprintf("entity_%d", (i+1)%entityCount),
			RelationType: "connects_to",
		})
	}
	
	// Batch create relations
	for i := 0; i < len(relations); i += batchSize {
		end := i + batchSize
		if end > len(relations) {
			end = len(relations)
		}
		if _, err := db.CreateRelations(ctx, relations[i:end]); err != nil {
			b.Fatal(err)
		}
	}
	
	return db
}

// BenchmarkReadGraph measures performance of reading the entire graph
func BenchmarkReadGraph(b *testing.B) {
	sizes := []int{10, 100, 1000}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			db := setupBenchDB(b, size)
			defer db.Close()
			
			ctx := context.Background()
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				graph, err := db.ReadGraph(ctx)
				if err != nil {
					b.Fatal(err)
				}
				if len(graph.Entities) != size {
					b.Fatalf("expected %d entities, got %d", size, len(graph.Entities))
				}
			}
		})
	}
}

// BenchmarkSearchNodes measures performance of searching nodes
func BenchmarkSearchNodes(b *testing.B) {
	sizes := []int{100, 1000, 5000}
	queries := []string{"test", "entity", "observation", "content"}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			db := setupBenchDB(b, size)
			defer db.Close()
			
			ctx := context.Background()
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				query := queries[i%len(queries)]
				graph, err := db.SearchNodes(ctx, query)
				if err != nil {
					b.Fatal(err)
				}
				_ = graph // Use the result
			}
		})
	}
}

// BenchmarkSearchNodesFTS measures performance of FTS5 search
func BenchmarkSearchNodesFTS(b *testing.B) {
	sizes := []int{100, 1000, 5000}
	queries := []string{"test", "entity", "observation", "content"}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			db := setupBenchDB(b, size)
			defer db.Close()
			
			ctx := context.Background()
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				query := queries[i%len(queries)]
				graph, err := db.SearchNodesFTS(ctx, query)
				if err != nil {
					// Fallback to regular search if FTS5 not available
					graph, err = db.SearchNodes(ctx, query)
					if err != nil {
						b.Fatal(err)
					}
				}
				_ = graph // Use the result
			}
		})
	}
}

// BenchmarkCreateEntities measures performance of entity creation
func BenchmarkCreateEntities(b *testing.B) {
	batchSizes := []int{1, 10, 100}
	
	for _, batchSize := range batchSizes {
		b.Run(fmt.Sprintf("batch_%d", batchSize), func(b *testing.B) {
			db, err := NewDBWithLogger("file::memory:?cache=shared", slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			
			ctx := context.Background()
			
			// Prepare entities
			entities := make([]EntityWithObservations, batchSize)
			for i := 0; i < batchSize; i++ {
				entities[i] = EntityWithObservations{
					Name:       fmt.Sprintf("entity_%d_%d", b.N, i),
					EntityType: "benchmark_type",
					Observations: []string{
						"observation_1",
						"observation_2",
					},
				}
			}
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				// Update entity names to avoid conflicts
				for j := 0; j < batchSize; j++ {
					entities[j].Name = fmt.Sprintf("entity_%d_%d", i, j)
				}
				
				_, err := db.CreateEntities(ctx, entities)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkOpenNodes measures performance of opening specific nodes
func BenchmarkOpenNodes(b *testing.B) {
	db := setupBenchDB(b, 1000)
	defer db.Close()
	
	ctx := context.Background()
	
	// Prepare node names to open
	nodeCounts := []int{1, 10, 50}
	
	for _, count := range nodeCounts {
		b.Run(fmt.Sprintf("nodes_%d", count), func(b *testing.B) {
			names := make([]string, count)
			for i := 0; i < count; i++ {
				names[i] = fmt.Sprintf("entity_%d", i*10) // Sample every 10th entity
			}
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				graph, err := db.OpenNodes(ctx, names)
				if err != nil {
					b.Fatal(err)
				}
				if len(graph.Entities) != count {
					b.Fatalf("expected %d entities, got %d", count, len(graph.Entities))
				}
			}
		})
	}
}

// Comparison benchmarks between old and new implementations

// BenchmarkReadGraphComparison compares N+1 vs optimized implementation
func BenchmarkReadGraphComparison(b *testing.B) {
	// This would compare the old N+1 implementation vs the new one
	// For now, we only have the optimized version
	b.Run("optimized", func(b *testing.B) {
		db := setupBenchDB(b, 1000)
		defer db.Close()
		
		ctx := context.Background()
		b.ResetTimer()
		
		for i := 0; i < b.N; i++ {
			_, err := db.ReadGraph(ctx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkSearchComparison compares LIKE vs FTS5 search
func BenchmarkSearchComparison(b *testing.B) {
	db := setupBenchDB(b, 1000)
	defer db.Close()
	
	ctx := context.Background()
	query := "test"
	
	b.Run("LIKE_search", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := db.SearchNodes(ctx, query)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
	
	b.Run("FTS5_search", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := db.SearchNodesFTS(ctx, query)
			if err != nil {
				// Fallback to regular search
				_, err = db.SearchNodes(ctx, query)
				if err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}