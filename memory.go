package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/pgvector/pgvector-go"
)

// MemoryService handles advanced conversation memory operations
type MemoryService struct {
	db *sql.DB
}

// MessageMetadata stores structured context about a message
type MessageMetadata struct {
	FilesMentioned  []string `json:"files_mentioned,omitempty"`
	TablesQueried   []string `json:"tables_queried,omitempty"`
	ToolsUsed       []string `json:"tools_used,omitempty"`
	CodeSymbols     []string `json:"code_symbols,omitempty"`
	ProjectsIndexed []string `json:"projects_indexed,omitempty"`
}

// ConversationMessage represents a message with full context
type ConversationMessage struct {
	ID        int
	SessionID string
	Role      string
	Content   string
	Metadata  MessageMetadata
	CreatedAt string
}

// ConversationSummary represents a summary of message range
type ConversationSummary struct {
	ID                int
	SessionID         string
	Summary           string
	MessageRangeStart int
	MessageRangeEnd   int
	CreatedAt         string
}

// NewMemoryService creates a new memory service instance
func NewMemoryService(database *sql.DB) *MemoryService {
	return &MemoryService{db: database}
}

// SaveMessage saves a message with embedding and metadata
func (ms *MemoryService) SaveMessage(ctx context.Context, sessionID, role, content string, metadata MessageMetadata) (int, error) {
	// Generate embedding for the message
	embedding, err := getEmbedding(ctx, content)
	if err != nil {
		log.Printf("⚠️  Warning: Failed to generate embedding for message: %v", err)
		// Continue without embedding - better to save message than fail completely
		embedding = nil
	}

	// Convert metadata to JSON
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	// Insert message with embedding and metadata
	var messageID int
	query := `
		INSERT INTO conversation_messages (session_id, role, content, embedding, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	if embedding != nil {
		err = ms.db.QueryRowContext(ctx, query, sessionID, role, content, pgvector.NewVector(embedding), metadataJSON).Scan(&messageID)
	} else {
		err = ms.db.QueryRowContext(ctx, query, sessionID, role, content, nil, metadataJSON).Scan(&messageID)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to save message: %w", err)
	}

	return messageID, nil
}

// GetRecentMessages retrieves the last N messages from a session
func (ms *MemoryService) GetRecentMessages(ctx context.Context, sessionID string, limit int) ([]ConversationMessage, error) {
	query := `
		SELECT id, session_id, role, content, COALESCE(metadata::text, '{}'), created_at
		FROM conversation_messages
		WHERE session_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := ms.db.QueryContext(ctx, query, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recent messages: %w", err)
	}
	defer rows.Close()

	var messages []ConversationMessage
	for rows.Next() {
		var msg ConversationMessage
		var metadataJSON string
		err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &metadataJSON, &msg.CreatedAt)
		if err != nil {
			log.Printf("⚠️  Error scanning message: %v", err)
			continue
		}

		// Parse metadata
		json.Unmarshal([]byte(metadataJSON), &msg.Metadata)
		messages = append(messages, msg)
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// SearchSimilarMessages performs semantic search on conversation history
func (ms *MemoryService) SearchSimilarMessages(ctx context.Context, sessionID, query string, limit int) ([]ConversationMessage, error) {
	// Generate embedding for search query
	queryVector, err := getEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	sqlQuery := `
		SELECT id, session_id, role, content, COALESCE(metadata::text, '{}'), created_at,
		       1 - (embedding <=> $1) AS similarity
		FROM conversation_messages
		WHERE session_id = $2 
		  AND embedding IS NOT NULL
		  AND 1 - (embedding <=> $1) > 0.7
		ORDER BY embedding <=> $1
		LIMIT $3`

	rows, err := ms.db.QueryContext(ctx, sqlQuery, pgvector.NewVector(queryVector), sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar messages: %w", err)
	}
	defer rows.Close()

	var messages []ConversationMessage
	for rows.Next() {
		var msg ConversationMessage
		var metadataJSON string
		var similarity float64
		err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &metadataJSON, &msg.CreatedAt, &similarity)
		if err != nil {
			log.Printf("⚠️  Error scanning similar message: %v", err)
			continue
		}

		json.Unmarshal([]byte(metadataJSON), &msg.Metadata)
		messages = append(messages, msg)
	}

	return messages, nil
}

// GetOrCreateSummary retrieves existing summary or creates one if needed
func (ms *MemoryService) GetOrCreateSummary(ctx context.Context, sessionID string) (*ConversationSummary, error) {
	// Check if summary exists
	var summary ConversationSummary
	err := ms.db.QueryRowContext(ctx, `
		SELECT id, session_id, summary, message_range_start, message_range_end, created_at
		FROM conversation_summaries
		WHERE session_id = $1
		ORDER BY created_at DESC
		LIMIT 1`, sessionID).Scan(
		&summary.ID, &summary.SessionID, &summary.Summary,
		&summary.MessageRangeStart, &summary.MessageRangeEnd, &summary.CreatedAt)

	if err == nil {
		// Summary exists, check if we need a new one
		var messageCount int
		ms.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM conversation_messages 
			WHERE session_id = $1 AND id > $2`, sessionID, summary.MessageRangeEnd).Scan(&messageCount)

		if messageCount < 30 {
			return &summary, nil // Existing summary is still valid
		}
	}

	// Need to create new summary
	return ms.createSummary(ctx, sessionID)
}

// createSummary generates a new summary for the conversation
func (ms *MemoryService) createSummary(ctx context.Context, sessionID string) (*ConversationSummary, error) {
	// Get all messages for summarization
	messages, err := ms.GetRecentMessages(ctx, sessionID, 100) // Get up to 100 messages
	if err != nil || len(messages) < 10 {
		return nil, fmt.Errorf("not enough messages to summarize")
	}

	// Build conversation text
	var conversationText strings.Builder
	for _, msg := range messages {
		conversationText.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}

	// Generate summary using LLM
	summaryPrompt := fmt.Sprintf(`Summarize this conversation in 3-4 sentences, focusing on:
1. Main topics discussed
2. Key decisions or solutions
3. Important context for future reference

Conversation:
%s

Summary:`, conversationText.String())

	summaryText, err := ms.generateSummaryWithLLM(ctx, summaryPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	// Save summary to database
	var summary ConversationSummary
	err = ms.db.QueryRowContext(ctx, `
		INSERT INTO conversation_summaries (session_id, summary, message_range_start, message_range_end)
		VALUES ($1, $2, $3, $4)
		RETURNING id, session_id, summary, message_range_start, message_range_end, created_at`,
		sessionID, summaryText, messages[0].ID, messages[len(messages)-1].ID).Scan(
		&summary.ID, &summary.SessionID, &summary.Summary,
		&summary.MessageRangeStart, &summary.MessageRangeEnd, &summary.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to save summary: %w", err)
	}

	log.Printf("📝 Created conversation summary for session %s (messages %d-%d)", sessionID, summary.MessageRangeStart, summary.MessageRangeEnd)
	return &summary, nil
}

// generateSummaryWithLLM uses the configured LLM to generate a summary
func (ms *MemoryService) generateSummaryWithLLM(ctx context.Context, prompt string) (string, error) {
	// Use the same model selection logic as main chat
	if useOllama && ollamaClient != nil {
		return ollamaClient.Call(ctx, prompt)
	}

	if useGrok && grokApiKey != "" {
		return callGrok(prompt)
	}

	if geminiModel != nil {
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

	return "", fmt.Errorf("no LLM available for summarization")
}

// BuildContextWindow assembles smart context from recent messages, summary, and semantic search
func (ms *MemoryService) BuildContextWindow(ctx context.Context, sessionID, currentQuery string) (string, error) {
	var contextParts []string

	// 1. Get conversation summary if exists
	summary, err := ms.GetOrCreateSummary(ctx, sessionID)
	if err == nil && summary != nil {
		contextParts = append(contextParts, fmt.Sprintf("CONVERSATION SUMMARY:\n%s\n", summary.Summary))
	}

	// 2. Search for semantically similar messages (if query references past)
	if containsPastReference(currentQuery) {
		similarMessages, err := ms.SearchSimilarMessages(ctx, sessionID, currentQuery, 3)
		if err == nil && len(similarMessages) > 0 {
			contextParts = append(contextParts, "RELEVANT PAST DISCUSSION:")
			for _, msg := range similarMessages {
				contextParts = append(contextParts, fmt.Sprintf("[%s] %s: %s", msg.CreatedAt[:10], msg.Role, truncate(msg.Content, 200)))
			}
			contextParts = append(contextParts, "")
		}
	}

	// 3. Get recent messages
	recentMessages, err := ms.GetRecentMessages(ctx, sessionID, 15)
	if err == nil && len(recentMessages) > 0 {
		contextParts = append(contextParts, "RECENT MESSAGES:")
		for _, msg := range recentMessages {
			contextParts = append(contextParts, fmt.Sprintf("%s: %s", msg.Role, msg.Content))
		}
	}

	return strings.Join(contextParts, "\n"), nil
}

// Helper functions

func containsPastReference(query string) bool {
	pastKeywords := []string{
		"yesterday", "last week", "before", "earlier", "previously",
		"ontem", "semana passada", "antes", "anteriormente",
		"remember", "recall", "lembrar", "lembra",
	}

	lowerQuery := strings.ToLower(query)
	for _, keyword := range pastKeywords {
		if strings.Contains(lowerQuery, keyword) {
			return true
		}
	}
	return false
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
