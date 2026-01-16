package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/google/generative-ai-go/genai"
)

// RerankingService handles result re-ranking with cross-encoder
type RerankingService struct {
	useReranking bool
}

// RerankResult represents a re-ranked search result
type RerankResult struct {
	SearchResult
	RerankScore float64
}

// NewRerankingService creates a new re-ranking service
func NewRerankingService(enabled bool) *RerankingService {
	return &RerankingService{
		useReranking: enabled,
	}
}

// Rerank re-scores search results using cross-encoder
func (rs *RerankingService) Rerank(ctx context.Context, query string, results []SearchResult) ([]SearchResult, error) {
	if !rs.useReranking || len(results) == 0 {
		return results, nil
	}

	// Create rerank results with scores
	rerankResults := make([]RerankResult, len(results))
	for i, result := range results {
		rerankResults[i] = RerankResult{
			SearchResult: result,
			RerankScore:  result.Score, // Initialize with original score
		}
	}

	// Calculate cross-encoder scores
	for i := range rerankResults {
		score, err := rs.calculateCrossEncoderScore(ctx, query, rerankResults[i].Content)
		if err != nil {
			log.Printf("⚠️  Re-ranking failed for result %d: %v", i, err)
			continue
		}
		rerankResults[i].RerankScore = score
	}

	// Sort by rerank score
	sort.Slice(rerankResults, func(i, j int) bool {
		return rerankResults[i].RerankScore > rerankResults[j].RerankScore
	})

	// Convert back to SearchResult
	rerankedResults := make([]SearchResult, len(rerankResults))
	for i, rr := range rerankResults {
		rerankedResults[i] = rr.SearchResult
		rerankedResults[i].Score = rr.RerankScore // Update score
	}

	return rerankedResults, nil
}

// calculateCrossEncoderScore calculates relevance score using LLM as cross-encoder
func (rs *RerankingService) calculateCrossEncoderScore(ctx context.Context, query, document string) (float64, error) {
	// Use LLM to score query-document relevance
	prompt := fmt.Sprintf(`Rate the relevance of this document to the query on a scale of 0.0 to 1.0.
Return ONLY a number between 0.0 and 1.0, nothing else.

Query: %s

Document:
%s

Relevance score (0.0-1.0):`, query, truncateForReranking(document))

	var response string
	var err error

	// Use fastest available model for re-ranking
	if useOllama && ollamaClient != nil {
		response, err = ollamaClient.Call(ctx, prompt)
	} else if geminiModel != nil {
		response, err = rs.callGemini(ctx, prompt)
	} else {
		return 0.5, fmt.Errorf("no LLM available for re-ranking")
	}

	if err != nil {
		return 0.5, err
	}

	// Parse score from response
	var score float64
	_, err = fmt.Sscanf(response, "%f", &score)
	if err != nil {
		// Fallback: try to extract number from text
		score = extractScoreFromText(response)
	}

	// Ensure score is in valid range
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score, nil
}

// callGemini uses Gemini for scoring
func (rs *RerankingService) callGemini(ctx context.Context, prompt string) (string, error) {
	resp, err := geminiModel.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}
	if resp == nil || len(resp.Candidates) == 0 {
		return "", fmt.Errorf("empty response from Gemini")
	}

	var answer string
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			answer += string(txt)
		}
	}
	return answer, nil
}

// truncateForReranking limits document size for re-ranking
func truncateForReranking(text string) string {
	maxLen := 500 // Limit to 500 chars for faster re-ranking
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// extractScoreFromText attempts to extract a score from text response
func extractScoreFromText(text string) float64 {
	// Try common patterns
	text = strings.TrimSpace(text)
	
	// Remove common prefixes
	text = strings.TrimPrefix(text, "Score:")
	text = strings.TrimPrefix(text, "Relevance:")
	text = strings.TrimSpace(text)
	
	// Try to parse first number
	var score float64
	_, err := fmt.Sscanf(text, "%f", &score)
	if err == nil && score >= 0 && score <= 1 {
		return score
	}
	
	// Default to middle score if parsing fails
	return 0.5
}

// RerankWithThreshold re-ranks and filters by minimum score
func (rs *RerankingService) RerankWithThreshold(ctx context.Context, query string, results []SearchResult, minScore float64) ([]SearchResult, error) {
	reranked, err := rs.Rerank(ctx, query, results)
	if err != nil {
		return results, err
	}

	// Filter by threshold
	var filtered []SearchResult
	for _, result := range reranked {
		if result.Score >= minScore {
			filtered = append(filtered, result)
		}
	}

	return filtered, nil
}

// BatchRerank re-ranks multiple queries efficiently
func (rs *RerankingService) BatchRerank(ctx context.Context, queries []string, resultSets [][]SearchResult) ([][]SearchResult, error) {
	if len(queries) != len(resultSets) {
		return nil, fmt.Errorf("queries and result sets must have same length")
	}

	rerankedSets := make([][]SearchResult, len(queries))
	for i, query := range queries {
		reranked, err := rs.Rerank(ctx, query, resultSets[i])
		if err != nil {
			log.Printf("⚠️  Batch re-ranking failed for query %d: %v", i, err)
			rerankedSets[i] = resultSets[i] // Use original on error
			continue
		}
		rerankedSets[i] = reranked
	}

	return rerankedSets, nil
}

// GetRerankingStats returns statistics about re-ranking performance
func (rs *RerankingService) GetRerankingStats(original, reranked []SearchResult) map[string]interface{} {
	stats := make(map[string]interface{})
	
	if len(original) == 0 || len(reranked) == 0 {
		return stats
	}

	// Calculate score improvement
	var originalAvg, rerankedAvg float64
	for i := 0; i < len(original) && i < len(reranked); i++ {
		originalAvg += original[i].Score
		rerankedAvg += reranked[i].Score
	}
	originalAvg /= float64(len(original))
	rerankedAvg /= float64(len(reranked))

	stats["original_avg_score"] = originalAvg
	stats["reranked_avg_score"] = rerankedAvg
	stats["improvement"] = rerankedAvg - originalAvg
	stats["improvement_pct"] = (rerankedAvg - originalAvg) / originalAvg * 100

	// Calculate rank changes
	rankChanges := 0
	for i, orig := range original {
		for j, rerank := range reranked {
			if orig.FilePath == rerank.FilePath && orig.ChunkIndex == rerank.ChunkIndex {
				if i != j {
					rankChanges++
				}
				break
			}
		}
	}
	stats["rank_changes"] = rankChanges
	stats["rank_change_pct"] = float64(rankChanges) / float64(len(original)) * 100

	return stats
}

// RerankTopK re-ranks only top K results for efficiency
func (rs *RerankingService) RerankTopK(ctx context.Context, query string, results []SearchResult, topK int) ([]SearchResult, error) {
	if len(results) <= topK {
		return rs.Rerank(ctx, query, results)
	}

	// Re-rank only top K
	topResults := results[:topK]
	reranked, err := rs.Rerank(ctx, query, topResults)
	if err != nil {
		return results, err
	}

	// Append remaining results
	remaining := results[topK:]
	reranked = append(reranked, remaining...)

	return reranked, nil
}
