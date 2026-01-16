package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/generative-ai-go/genai"
)

// Agent represents a specialized AI agent
type Agent struct {
	Name           string
	Specialization string
	SystemPrompt   string
	Tools          []string
	FileFilters    []string // File extensions this agent specializes in
}

// MasterAgent orchestrates specialized agents
type MasterAgent struct {
	Agents map[string]*Agent
}

// AgentResponse represents a response from an agent
type AgentResponse struct {
	AgentName string
	Response  string
	Error     error
}

// NewMasterAgent creates a new master agent with all specialized agents
func NewMasterAgent() *MasterAgent {
	ma := &MasterAgent{
		Agents: make(map[string]*Agent),
	}

	// Frontend Agent
	ma.Agents["frontend"] = &Agent{
		Name:           "Frontend Agent",
		Specialization: "React, Flutter, UI/UX, Mobile Development",
		SystemPrompt: `You are a Frontend specialist for EVA-Code-Brain.
Expertise: React, Flutter, Dart, UI/UX, mobile development, responsive design.

Focus on:
- Component architecture and reusability
- State management (Provider, Riverpod, Redux)
- UI/UX best practices and accessibility
- Mobile-specific patterns (Flutter widgets, navigation)
- Performance optimization (lazy loading, memoization)
- Responsive design and cross-platform compatibility

When analyzing code, prioritize:
- Clean component structure
- Proper state management
- User experience
- Performance considerations`,
		Tools:       []string{"search_code", "get_file", "git_blame", "git_log", "git_diff"},
		FileFilters: []string{".jsx", ".tsx", ".dart", ".vue", ".svelte", ".html", ".css"},
	}

	// Backend Agent
	ma.Agents["backend"] = &Agent{
		Name:           "Backend Agent",
		Specialization: "Go, Python, APIs, Microservices",
		SystemPrompt: `You are a Backend specialist for EVA-Code-Brain.
Expertise: Go, Python, FastAPI, microservices, REST APIs, gRPC.

Focus on:
- API design and RESTful principles
- Service architecture and scalability
- Error handling and validation
- Authentication and authorization
- Performance optimization and caching
- Database interactions and ORM usage

When analyzing code, prioritize:
- Clean architecture
- Error handling
- Security best practices
- Performance and scalability`,
		Tools:       []string{"search_code", "get_file", "query_database", "call_api", "git_blame", "git_log", "git_diff"},
		FileFilters: []string{".go", ".py", ".java", ".js", ".ts"},
	}

	// Database Agent
	ma.Agents["database"] = &Agent{
		Name:           "Database Agent",
		Specialization: "SQL, PostgreSQL, Data Modeling",
		SystemPrompt: `You are a Database specialist for EVA-Code-Brain.
Expertise: PostgreSQL, SQL optimization, data modeling, migrations, indexing.

Focus on:
- Query optimization and performance
- Index design and usage
- Schema design and normalization
- Data integrity and constraints
- Migration strategies
- Performance tuning and EXPLAIN analysis

When analyzing code, prioritize:
- Query efficiency
- Proper indexing
- Data integrity
- Scalability considerations`,
		Tools:       []string{"search_code", "query_database", "analyze_health_trend", "git_log", "git_diff"},
		FileFilters: []string{".sql"},
	}

	// DevOps Agent
	ma.Agents["devops"] = &Agent{
		Name:           "DevOps Agent",
		Specialization: "Docker, CI/CD, Infrastructure",
		SystemPrompt: `You are a DevOps specialist for EVA-Code-Brain.
Expertise: Docker, CI/CD, deployment, infrastructure, monitoring, Kubernetes.

Focus on:
- Containerization and Docker best practices
- CI/CD pipeline design
- Deployment strategies (blue-green, canary)
- Infrastructure as code
- Monitoring and logging
- Security and secrets management

When analyzing code, prioritize:
- Container optimization
- Pipeline efficiency
- Security considerations
- Scalability and reliability`,
		Tools:       []string{"search_code", "get_file", "call_api", "git_log", "git_diff"},
		FileFilters: []string{"Dockerfile", ".yml", ".yaml", ".sh", ".tf"},
	}

	// Health Agent
	ma.Agents["health"] = &Agent{
		Name:           "Health Agent",
		Specialization: "Medical Data, Health Analytics",
		SystemPrompt: `You are a Health Data specialist for EVA-Code-Brain.
Expertise: Medical data analysis, health metrics, risk assessment, statistical analysis.

Focus on:
- Health data interpretation and validation
- Statistical analysis and trend detection
- Risk assessment and medical recommendations
- Data quality and accuracy
- Privacy and HIPAA compliance
- Clinical decision support

When analyzing data, prioritize:
- Medical accuracy
- Patient safety
- Data privacy
- Evidence-based recommendations`,
		Tools:       []string{"analyze_health_trend", "assess_health_risk", "query_database", "search_code"},
		FileFilters: []string{".py", ".sql"},
	}

	return ma
}

// Classify determines which agent(s) should handle a query
func (ma *MasterAgent) Classify(ctx context.Context, query string) ([]string, error) {
	classificationPrompt := fmt.Sprintf(`Analyze this query and classify it into one or more categories.
Return ONLY the category names, comma-separated, nothing else.

Categories:
- frontend: React, Flutter, UI, UX, mobile app, components, styling, navigation
- backend: Go, Python, APIs, services, endpoints, authentication, business logic
- database: SQL, queries, schema, tables, indexes, data modeling, migrations
- devops: Docker, CI/CD, deployment, infrastructure, containers, pipelines
- health: Medical data, health metrics, analytics, risk assessment, vital signs

Query: %s

Categories (comma-separated):`, query)

	// Use the configured LLM to classify
	var classification string
	var err error

	if useOllama && ollamaClient != nil {
		classification, err = ollamaClient.Call(ctx, classificationPrompt)
	} else if useGrok {
		classification, err = callGrok(classificationPrompt)
	} else if geminiModel != nil {
		classification, err = ma.callGeminiForClassification(ctx, classificationPrompt)
	} else {
		return nil, fmt.Errorf("no LLM available for classification")
	}

	if err != nil {
		return nil, fmt.Errorf("classification failed: %w", err)
	}

	// Parse classification
	agents := ma.parseClassification(classification)

	// Default to backend if no clear classification
	if len(agents) == 0 {
		agents = []string{"backend"}
	}

	return agents, nil
}

// callGeminiForClassification uses Gemini for classification
func (ma *MasterAgent) callGeminiForClassification(ctx context.Context, prompt string) (string, error) {
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

// parseClassification extracts agent names from classification response
func (ma *MasterAgent) parseClassification(classification string) []string {
	classification = strings.ToLower(strings.TrimSpace(classification))

	// Remove common prefixes/suffixes
	classification = strings.TrimPrefix(classification, "categories:")
	classification = strings.TrimSuffix(classification, ".")

	// Split by comma
	parts := strings.Split(classification, ",")

	var agents []string
	validAgents := map[string]bool{
		"frontend": true,
		"backend":  true,
		"database": true,
		"devops":   true,
		"health":   true,
	}

	for _, part := range parts {
		agent := strings.TrimSpace(part)
		if validAgents[agent] {
			agents = append(agents, agent)
		}
	}

	return agents
}

// Delegate executes a query with a specific agent
func (ma *MasterAgent) Delegate(ctx context.Context, query string, agentName string, searchResults []SearchResult, history string) (string, error) {
	agent, exists := ma.Agents[agentName]
	if !exists {
		return "", fmt.Errorf("agent '%s' not found", agentName)
	}

	// Build specialized prompt
	specializedPrompt := ma.buildAgentPrompt(agent, query, searchResults, history)

	// Execute with LLM
	var response string
	var err error

	if useOllama && ollamaClient != nil {
		response, err = ollamaClient.Call(ctx, specializedPrompt)
	} else if useGrok {
		response, err = callGrok(specializedPrompt)
	} else if geminiModel != nil {
		response, err = ma.callGeminiForClassification(ctx, specializedPrompt)
	} else {
		return "", fmt.Errorf("no LLM available")
	}

	if err != nil {
		return "", fmt.Errorf("agent execution failed: %w", err)
	}

	return response, nil
}

// buildAgentPrompt constructs a specialized prompt for an agent
func (ma *MasterAgent) buildAgentPrompt(agent *Agent, query string, searchResults []SearchResult, history string) string {
	var prompt strings.Builder

	// Agent's system prompt
	prompt.WriteString(agent.SystemPrompt)
	prompt.WriteString("\n\n")

	// Conversation history
	if history != "" {
		prompt.WriteString("CONVERSATION HISTORY:\n")
		prompt.WriteString(history)
		prompt.WriteString("\n\n")
	}

	// Code context (filtered by agent's specialization)
	if len(searchResults) > 0 {
		prompt.WriteString("RELEVANT CODE:\n\n")
		for _, r := range searchResults {
			// Check if file matches agent's filters
			if ma.matchesAgentFilters(r.FilePath, agent.FileFilters) {
				prompt.WriteString(fmt.Sprintf("--- %s ---\n", r.FilePath))
				prompt.WriteString(r.Content)
				prompt.WriteString("\n\n")
			}
		}
	}

	// User query
	prompt.WriteString("USER QUERY:\n")
	prompt.WriteString(query)
	prompt.WriteString("\n\n")

	// Instructions
	prompt.WriteString("Provide a detailed, specialized response based on your expertise.\n")
	prompt.WriteString("If you need to use tools, mention which tools would be helpful.\n")

	return prompt.String()
}

// matchesAgentFilters checks if a file matches agent's specialization
func (ma *MasterAgent) matchesAgentFilters(filePath string, filters []string) bool {
	if len(filters) == 0 {
		return true // No filters = accept all
	}

	filePath = strings.ToLower(filePath)
	for _, filter := range filters {
		if strings.HasSuffix(filePath, strings.ToLower(filter)) {
			return true
		}
		if strings.Contains(filePath, strings.ToLower(filter)) {
			return true
		}
	}
	return false
}

// Aggregate combines responses from multiple agents
func (ma *MasterAgent) Aggregate(ctx context.Context, responses map[string]string) string {
	if len(responses) == 0 {
		return "No responses from agents."
	}

	// Single agent - return directly
	if len(responses) == 1 {
		for _, response := range responses {
			return response
		}
	}

	// Multiple agents - synthesize
	var aggregated strings.Builder
	aggregated.WriteString("**Multi-Agent Analysis:**\n\n")

	// Add each agent's perspective
	agentOrder := []string{"frontend", "backend", "database", "devops", "health"}
	for _, agentName := range agentOrder {
		if response, exists := responses[agentName]; exists {
			agent := ma.Agents[agentName]
			aggregated.WriteString(fmt.Sprintf("### %s Perspective:\n", agent.Name))
			aggregated.WriteString(response)
			aggregated.WriteString("\n\n---\n\n")
		}
	}

	return aggregated.String()
}

// ExecuteMultiAgent runs the full multi-agent workflow
func (ma *MasterAgent) ExecuteMultiAgent(ctx context.Context, query string, searchResults []SearchResult, history string) (string, error) {
	// 1. Classify query
	agents, err := ma.Classify(ctx, query)
	if err != nil {
		return "", fmt.Errorf("classification failed: %w", err)
	}

	log.Printf("🤖 Query classified to agents: %v", agents)

	// 2. Delegate to each agent
	responses := make(map[string]string)
	for _, agentName := range agents {
		response, err := ma.Delegate(ctx, query, agentName, searchResults, history)
		if err != nil {
			log.Printf("⚠️  Agent '%s' failed: %v", agentName, err)
			continue
		}
		responses[agentName] = response
	}

	// 3. Aggregate responses
	finalResponse := ma.Aggregate(ctx, responses)

	return finalResponse, nil
}
