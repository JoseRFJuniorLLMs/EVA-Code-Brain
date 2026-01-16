package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"

	"github.com/pgvector/pgvector-go"
)

// SearchResult struct is defined in main.go

// searchHybrid performs both Vector and Keyword search and combines them using RRF
func searchHybrid(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	// 1. Vector Search
	vectorResults, err := searchVector(ctx, query, limit*2) // fetch more for reranking
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %v", err)
	}

	// 2. Keyword Search (FTS)
	keywordResults, err := searchKeyword(ctx, query, limit*2)
	if err != nil {
		// Keyword search failures shouldn't block everything, log and proceed
		log.Printf("⚠️ Keyword search failed: %v", err)
		keywordResults = []SearchResult{}
	}

	// 3. RRF Combination
	// k is a constant, typically 60
	const k = 60.0
	scores := make(map[string]float64)
	resultsMap := make(map[string]SearchResult)

	// Process Vector Results
	for rank, r := range vectorResults {
		key := fmt.Sprintf("%s:%s:%d", r.ProjectName, r.FilePath, r.ChunkIndex)
		scores[key] += 1.0 / (k + float64(rank+1))
		resultsMap[key] = r
	}

	// Process Keyword Results
	for rank, r := range keywordResults {
		key := fmt.Sprintf("%s:%s:%d", r.ProjectName, r.FilePath, r.ChunkIndex)
		scores[key] += 1.0 / (k + float64(rank+1))

		// If exists (found by vector too), prefer the one with metadata if available
		if existing, exists := resultsMap[key]; exists {
			// Keep existing but update score? No, score is updated in 'scores' map.
			// Just ensure we have the struct content.
			_ = existing
		} else {
			resultsMap[key] = r
		}
	}

	// Convert back to slice
	var finalResults []SearchResult
	for key, score := range scores {
		r := resultsMap[key]
		r.Score = score
		finalResults = append(finalResults, r)
	}

	// Sort by RRF Score DESC
	sort.Slice(finalResults, func(i, j int) bool {
		return finalResults[i].Score > finalResults[j].Score
	})

	// Return Top N
	if len(finalResults) > limit {
		return finalResults[:limit], nil
	}
	return finalResults, nil
}

// searchVector is the semantic search (existing logic refactored)
func searchVector(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	// Get query embedding
	embedding, err := getEmbedding(ctx, query)
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT 
			file_path, chunk_index, content, 
			chunk_type, symbol_name, start_line, end_line, project_name,
			1 - (embedding <=> $1) AS similarity
		FROM project_codebase
		ORDER BY embedding <=> $1
		LIMIT $2
	`, pgvector.NewVector(embedding), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		// Handle NULLs safely for metadata columns
		var cType, symName, projName sql.NullString
		var sLine, eLine sql.NullInt32

		if err := rows.Scan(&r.FilePath, &r.ChunkIndex, &r.Content,
			&cType, &symName, &sLine, &eLine, &projName,
			&r.Similarity); err != nil {
			log.Println("Error scanning vector result:", err)
			continue
		}

		r.Type = cType.String
		r.Symbol = symName.String
		r.StartLine = int(sLine.Int32)
		r.EndLine = int(eLine.Int32)
		r.ProjectName = projName.String

		results = append(results, r)
	}
	return results, nil
}

// searchKeyword uses Postgres Full Text Search
func searchKeyword(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	// Convert query to tsquery compatible format
	// websearch_to_tsquery handles natural language better ("foo or bar")

	rows, err := db.QueryContext(ctx, `
		SELECT 
			file_path, chunk_index, content, 
			chunk_type, symbol_name, start_line, end_line, project_name,
			ts_rank(content_fts, websearch_to_tsquery('english', $1)) as rank
		FROM project_codebase
		WHERE content_fts @@ websearch_to_tsquery('english', $1)
		ORDER BY rank DESC
		LIMIT $2
	`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var rank float64
		// Handle NULLs
		var cType, symName, projName sql.NullString
		var sLine, eLine sql.NullInt32

		if err := rows.Scan(&r.FilePath, &r.ChunkIndex, &r.Content,
			&cType, &symName, &sLine, &eLine, &projName,
			&rank); err != nil {
			log.Println("Error scanning keyword result:", err)
			continue
		}

		r.Type = cType.String
		r.Symbol = symName.String
		r.StartLine = int(sLine.Int32)
		r.EndLine = int(eLine.Int32)
		r.ProjectName = projName.String
		r.Similarity = rank // Use rank as similarity proxy

		results = append(results, r)
	}
	return results, nil
}
