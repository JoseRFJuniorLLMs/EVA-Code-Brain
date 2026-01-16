package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// ============================================
// TOOL FRAMEWORK
// ============================================

// Tool representa uma ferramenta que o LLM pode chamar
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
	Execute     func(ctx context.Context, params map[string]interface{}) (string, error)
}

// ToolRegistry gerencia todas as ferramentas disponíveis
type ToolRegistry struct {
	tools map[string]*Tool
}

// NewToolRegistry cria um novo registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*Tool),
	}
}

// Register adiciona uma ferramenta ao registry
func (r *ToolRegistry) Register(tool *Tool) {
	r.tools[tool.Name] = tool
}

// Get retorna uma ferramenta pelo nome
func (r *ToolRegistry) Get(name string) (*Tool, bool) {
	tool, exists := r.tools[name]
	return tool, exists
}

// List retorna todas as ferramentas disponíveis
func (r *ToolRegistry) List() []*Tool {
	tools := make([]*Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

// ToOpenAIFormat converte ferramentas para formato OpenAI function calling
func (r *ToolRegistry) ToOpenAIFormat() []map[string]interface{} {
	functions := make([]map[string]interface{}, 0, len(r.tools))
	for _, tool := range r.tools {
		functions = append(functions, map[string]interface{}{
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.Parameters,
		})
	}
	return functions
}

// Execute executa uma ferramenta
func (r *ToolRegistry) Execute(ctx context.Context, name string, params map[string]interface{}) (string, error) {
	tool, exists := r.Get(name)
	if !exists {
		return "", fmt.Errorf("ferramenta '%s' não encontrada", name)
	}
	return tool.Execute(ctx, params)
}

// ============================================
// TOOL DEFINITIONS
// ============================================

var toolRegistry *ToolRegistry

func initTools() {
	toolRegistry = NewToolRegistry()

	// Tool 1: search_code
	toolRegistry.Register(&Tool{
		Name:        "search_code",
		Description: "Busca semântica no código indexado. Use para encontrar funções, classes, ou trechos de código relevantes.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "Consulta de busca (ex: 'função de autenticação', 'criar tabela usuarios')",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Número máximo de resultados (padrão: 5)",
					"default":     5,
				},
			},
			"required": []string{"query"},
		},
		Execute: executeSearchCode,
	})

	// Tool 2: get_file
	toolRegistry.Register(&Tool{
		Name:        "get_file",
		Description: "Retorna o conteúdo completo de um arquivo específico.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file_path": map[string]interface{}{
					"type":        "string",
					"description": "Caminho relativo do arquivo (ex: 'main.go', 'auth/login.py')",
				},
				"project_name": map[string]interface{}{
					"type":        "string",
					"description": "Opcional: Nome do projeto (ex: 'EVA-Back', 'EVA-Code-Brain') caso haja arquivos com nomes iguais.",
				},
			},
			"required": []string{"file_path"},
		},
		Execute: executeGetFile,
	})

	// Tool 3: list_files
	toolRegistry.Register(&Tool{
		Name:        "list_files",
		Description: "Lista arquivos indexados, opcionalmente filtrados por padrão.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Padrão de busca (ex: '*.go', 'auth*', vazio para todos)",
					"default":     "",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Número máximo de arquivos (padrão: 50)",
					"default":     50,
				},
			},
		},
		Execute: executeListFiles,
	})

	// Tool 4: find_references
	toolRegistry.Register(&Tool{
		Name:        "find_references",
		Description: "Encontra onde um símbolo (função, variável, classe) é usado no código.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"symbol": map[string]interface{}{
					"type":        "string",
					"description": "Nome do símbolo a buscar (ex: 'getUserByID', 'User')",
				},
			},
			"required": []string{"symbol"},
		},
		Execute: executeFindReferences,
	})

	// Tool 5: analyze_project
	toolRegistry.Register(&Tool{
		Name:        "analyze_project",
		Description: "Retorna estatísticas sobre o projeto (total de arquivos, linguagens, tamanho).",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
		Execute: executeAnalyzeProject,
	})
}

// ============================================
// TOOL IMPLEMENTATIONS
// ============================================

func executeSearchCode(ctx context.Context, params map[string]interface{}) (string, error) {
	query, ok := params["query"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'query' inválido")
	}

	limit := 5
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	results, err := searchHybrid(ctx, query, limit)
	if err != nil {
		return "", err
	}

	// Formata resultados
	output := fmt.Sprintf("Encontrados %d resultados:\n\n", len(results))
	for i, r := range results {
		meta := ""
		if r.Type != "" {
			meta = fmt.Sprintf(" [%s: %s]", r.Type, r.Symbol)
		}
		proj := ""
		if r.ProjectName != "" {
			proj = fmt.Sprintf("[%s] ", r.ProjectName)
		}
		output += fmt.Sprintf("%d. %s%s%s (chunk %d, linhas %d-%d, similaridade: %.3f)\n```%s\n%s\n```\n\n",
			i+1, proj, r.FilePath, meta, r.ChunkIndex, r.StartLine, r.EndLine, r.Score, detectLanguage(r.FilePath), r.Content)
	}

	return output, nil
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".sql":
		return "sql"
	default:
		return ""
	}
}

func executeGetFile(ctx context.Context, params map[string]interface{}) (string, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'file_path' inválido")
	}
	projectName, _ := params["project_name"].(string)

	query := `
		SELECT content, chunk_index
		FROM project_codebase
		WHERE file_path = $1
	`
	args := []interface{}{filePath}
	if projectName != "" {
		query += " AND project_name = $2"
		args = append(args, projectName)
	}
	query += " ORDER BY chunk_index"

	// Busca todos os chunks do arquivo
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var chunks []string
	for rows.Next() {
		var content string
		var chunkIndex int
		if err := rows.Scan(&content, &chunkIndex); err != nil {
			continue
		}
		chunks = append(chunks, content)
	}

	if len(chunks) == 0 {
		return "", fmt.Errorf("arquivo '%s' não encontrado", filePath)
	}

	// Junta chunks
	fullContent := ""
	for _, chunk := range chunks {
		fullContent += chunk + "\n"
	}

	return fmt.Sprintf("Arquivo: %s\n\n```\n%s\n```", filePath, fullContent), nil
}

func executeListFiles(ctx context.Context, params map[string]interface{}) (string, error) {
	pattern := ""
	if p, ok := params["pattern"].(string); ok {
		pattern = p
	}

	limit := 50
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	query := `
		SELECT DISTINCT file_path, language, file_size
		FROM project_codebase
	`
	args := []interface{}{}

	if pattern != "" {
		query += " WHERE file_path LIKE $1"
		args = append(args, "%"+pattern+"%")
	}

	query += " ORDER BY file_path LIMIT $" + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	output := "Arquivos encontrados:\n\n"
	count := 0
	for rows.Next() {
		var filePath, language string
		var fileSize int
		if err := rows.Scan(&filePath, &language, &fileSize); err != nil {
			continue
		}
		count++
		output += fmt.Sprintf("%d. %s (%s, %d bytes)\n", count, filePath, language, fileSize)
	}

	if count == 0 {
		return "Nenhum arquivo encontrado.", nil
	}

	return output, nil
}

func executeFindReferences(ctx context.Context, params map[string]interface{}) (string, error) {
	symbol, ok := params["symbol"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'symbol' inválido")
	}

	// Busca por chunks que contêm o símbolo
	rows, err := db.QueryContext(ctx, `
		SELECT file_path, chunk_index, content
		FROM project_codebase
		WHERE content ILIKE $1
		ORDER BY file_path, chunk_index
		LIMIT 20
	`, "%"+symbol+"%")
	if err != nil {
		return "", err
	}
	defer rows.Close()

	output := fmt.Sprintf("Referências para '%s':\n\n", symbol)
	count := 0
	for rows.Next() {
		var filePath, content string
		var chunkIndex int
		if err := rows.Scan(&filePath, &chunkIndex, &content); err != nil {
			continue
		}
		count++
		output += fmt.Sprintf("%d. %s (chunk %d)\n```\n%s\n```\n\n", count, filePath, chunkIndex, content)
	}

	if count == 0 {
		return fmt.Sprintf("Nenhuma referência encontrada para '%s'.", symbol), nil
	}

	return output, nil
}

func executeAnalyzeProject(ctx context.Context, params map[string]interface{}) (string, error) {
	// Estatísticas gerais
	var totalFiles, totalChunks int
	var totalSize int64

	err := db.QueryRowContext(ctx, `
		SELECT 
			COUNT(DISTINCT file_path) as files,
			COUNT(*) as chunks,
			SUM(file_size) as total_size
		FROM project_codebase
	`).Scan(&totalFiles, &totalChunks, &totalSize)
	if err != nil {
		return "", err
	}

	// Por linguagem
	rows, err := db.QueryContext(ctx, `
		SELECT language, COUNT(DISTINCT file_path) as count
		FROM project_codebase
		WHERE language IS NOT NULL
		GROUP BY language
		ORDER BY count DESC
	`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	output := "📊 Estatísticas do Projeto:\n\n"
	output += fmt.Sprintf("Total de arquivos: %d\n", totalFiles)
	output += fmt.Sprintf("Total de chunks: %d\n", totalChunks)
	output += fmt.Sprintf("Tamanho total: %.2f MB\n\n", float64(totalSize)/(1024*1024))
	output += "Linguagens:\n"

	for rows.Next() {
		var language string
		var count int
		if err := rows.Scan(&language, &count); err != nil {
			continue
		}
		output += fmt.Sprintf("  - %s: %d arquivos\n", language, count)
	}

	return output, nil
}

// ============================================
// TOOL EXECUTION HELPERS
// ============================================

// ToolCall representa uma chamada de ferramenta do LLM
type ToolCall struct {
	ID   string                 `json:"id"`
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"arguments"`
}

// ExecuteToolCall executa uma chamada de ferramenta e retorna o resultado
func ExecuteToolCall(ctx context.Context, call ToolCall) (string, error) {
	// Parse arguments se vier como string JSON
	var args map[string]interface{}
	if argsStr, ok := call.Args["arguments"].(string); ok {
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			args = call.Args
		}
	} else {
		args = call.Args
	}

	return toolRegistry.Execute(ctx, call.Name, args)
}
