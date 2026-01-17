package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
	"github.com/tmc/langchaingo/llms/ollama"
	"google.golang.org/api/option"
)

// ============================================
// ESTRUTURAS
// ============================================

type SearchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type SearchResult struct {
	FilePath    string  `json:"file_path"`
	ChunkIndex  int     `json:"chunk_index"`
	Content     string  `json:"content"`
	Similarity  float64 `json:"similarity"`
	Type        string  `json:"type"`         // NEW
	Symbol      string  `json:"symbol"`       // NEW
	StartLine   int     `json:"start_line"`   // NEW
	EndLine     int     `json:"end_line"`     // NEW
	ProjectName string  `json:"project_name"` // NEW
	Score       float64 `json:"score"`        // RRF Score
}

type ChatRequest struct {
	Question  string `json:"question"`
	Model     string `json:"model"` // 'grok', 'ollama', 'gemini', 'auto'
	SessionID string `json:"session_id"`
}

type ChatResponse struct {
	Answer        string         `json:"answer"`
	SourceFiles   []string       `json:"source_files"`
	SearchResults []SearchResult `json:"search_results"`
}

// ============================================
// CONFIGURAÇÃO
// ============================================

var (
	db *sql.DB
	// Google Gemini
	geminiClient *genai.Client
	geminiModel  *genai.GenerativeModel
	// Ollama
	ollamaClient      *ollama.LLM
	ollamaEmbedClient *ollama.LLM
	ollamaToolWrapper *OllamaToolClient
	useOllama         bool
	// Grok
	useGrok    bool
	grokApiKey string
	grokModel  string = "grok-4-latest"
	// Multi-Agent
	useMultiAgent bool
	masterAgent   *MasterAgent
	// Re-ranking
	useReranking     bool
	rerankingService *RerankingService
)

type GrokRequest struct {
	Messages    []GrokMessage `json:"messages"`
	Model       string        `json:"model"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature"`
}

type GrokMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GrokResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func init() {
	var err error

	// Carrega .env
	_ = godotenv.Load()

	// Conecta ao PostgreSQL
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("❌ DATABASE_URL está vazia! Crie um arquivo .env ou configure a variável.")
	}

	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Erro ao conectar ao banco:", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal("Banco inacessível:", err)
	}

	log.Println("✅ Conectado ao PostgreSQL")

	// Inicializa ferramentas
	initTools()
	log.Printf("🔧 %d ferramentas registradas", len(toolRegistry.List()))

	// Inicializa Git service
	ctx := context.Background()
	initGitService(ctx)
	log.Println("📦 Git service inicializado")

	// Inicializa Health Analytics
	initHealthAnalytics()
	log.Println("🏥 Health Analytics inicializado")

	// Inicializa Multi-Agent System
	useMultiAgent, _ = strconv.ParseBool(os.Getenv("USE_MULTI_AGENT"))
	if useMultiAgent {
		masterAgent = NewMasterAgent()
		log.Println("🤖 Multi-Agent System ativado")
	}

	// Inicializa Test Generation
	initTestGeneration()
	log.Println("🧪 Test Generation inicializado")

	// Inicializa Code Quality
	initCodeQuality()
	log.Println("📊 Code Quality Analysis inicializado")

	// Inicializa Re-ranking
	useReranking, _ = strconv.ParseBool(os.Getenv("USE_RERANKING"))
	rerankingService = NewRerankingService(useReranking)
	if useReranking {
		log.Println("🎯 Re-ranking ativado")
	}

	// Inicializa Gemini
	useOllama, _ = strconv.ParseBool(os.Getenv("USE_OLLAMA"))
	useGrok, _ = strconv.ParseBool(os.Getenv("USE_GROK"))
	grokApiKey = os.Getenv("GROK_API_KEY")

	if useGrok {
		log.Println("🚀 Modo GROK ativado (xAI)")
		// Embeddings continuarão usando Gemini ou Ollama
	}

	if useOllama {
		log.Println("🦙 Modo OLLAMA ativado (Local AI)")

		// Cliente de Chat (Llama3)
		chatModel := os.Getenv("OLLAMA_MODEL")
		llm, err := ollama.New(ollama.WithModel(chatModel))
		if err != nil {
			log.Fatal("Erro ao criar cliente Ollama Chat:", err)
		}
		ollamaClient = llm

		// Cliente de Embedding (Nomic)
		embedModel := os.Getenv("OLLAMA_EMBED_MODEL")
		if embedModel == "" {
			embedModel = "nomic-embed-text"
		}

		embedLlm, err := ollama.New(ollama.WithModel(embedModel))
		if err != nil {
			log.Fatal("Erro ao criar cliente Ollama Embed:", err)
		}
		ollamaEmbedClient = embedLlm

		log.Printf("✅ Ollama Inicializado: Chat=%s, Embed=%s", chatModel, embedModel)

		// Wrapper for Tool Support
		ollamaToolWrapper = NewOllamaToolClient(ollamaClient)
	}

	// Inicializa Google Gemini (Sempre necessário para embeddings se Ollama estiver off, ou se Grok estiver on)
	if !useOllama {
		apiKey := os.Getenv("GOOGLE_API_KEY")
		if apiKey == "" {
			log.Println("⚠️ GOOGLE_API_KEY não definida (Embeddings podem falhar)")
		} else {
			ctx := context.Background()
			geminiClient, err = genai.NewClient(ctx, option.WithAPIKey(apiKey))
			if err != nil {
				log.Fatal("Erro ao criar cliente Gemini:", err)
			}
			geminiModel = geminiClient.GenerativeModel("gemini-1.5-flash-latest")
			log.Println("✅ Cliente Gemini inicializado (Embeddings/Chat)")
		}
	}
}

// ============================================
// FUNÇÕES CORE
// ============================================

func callGrok(prompt string) (string, error) {
	reqBody := GrokRequest{
		Messages: []GrokMessage{
			{Role: "system", Content: "You are a helpful coding assistant specialized in Go and web development."},
			{Role: "user", Content: prompt},
		},
		Model:       grokModel,
		Stream:      false,
		Temperature: 0.1,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.x.ai/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+grokApiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("erro API Grok: %s", resp.Status)
	}

	var grokResp GrokResponse
	if err := json.NewDecoder(resp.Body).Decode(&grokResp); err != nil {
		return "", err
	}

	if len(grokResp.Choices) == 0 {
		return "", fmt.Errorf("resposta vazia do Grok")
	}

	return grokResp.Choices[0].Message.Content, nil
}

func getEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Prioridade: Gemini (sempre disponível) > Ollama (opcional)

	// 1. Tenta Gemini primeiro (se disponível)
	if geminiClient != nil {
		em := geminiClient.EmbeddingModel("text-embedding-004")
		res, err := em.EmbedContent(ctx, genai.Text(text))
		if err == nil && res != nil && res.Embedding != nil {
			return res.Embedding.Values, nil
		}
		log.Printf("⚠️  Gemini embedding falhou: %v", err)
	}

	// 2. Fallback para Ollama (se ativado)
	if useOllama && ollamaEmbedClient != nil {
		embeddings, err := ollamaEmbedClient.CreateEmbedding(ctx, []string{text})
		if err != nil {
			return nil, fmt.Errorf("Ollama embedding falhou: %w", err)
		}
		if len(embeddings) == 0 || len(embeddings[0]) == 0 {
			return nil, fmt.Errorf("embedding vazio do Ollama")
		}
		return embeddings[0], nil
	}

	return nil, fmt.Errorf("nenhum serviço de embedding disponível (Gemini e Ollama falharam ou desativados)")
}

func searchCodebase(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	// 1. Gera embedding da query
	queryVector, err := getEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar embedding: %w", err)
	}

	// 2. Busca no banco usando distância de cosseno
	rows, err := db.QueryContext(ctx, `
		SELECT 
			file_path,
			chunk_index,
			content,
			chunk_type,
			symbol_name,
			start_line,
			end_line,
			1 - (embedding <=> $1) AS similarity
		FROM project_codebase
		WHERE 1 - (embedding <=> $1) > 0.5
		ORDER BY embedding <=> $1
		LIMIT $2
	`, pgvector.NewVector(queryVector), limit)

	if err != nil {
		return nil, fmt.Errorf("erro na busca: %w", err)
	}
	defer rows.Close()

	// 3. Coleta resultados
	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		if err := rows.Scan(&r.FilePath, &r.ChunkIndex, &r.Content, &r.Type, &r.Symbol, &r.StartLine, &r.EndLine, &r.Similarity); err != nil {
			log.Println("Erro ao escanear linha:", err)
			continue
		}
		results = append(results, r)
	}

	return results, nil
}

func generateAnswer(ctx context.Context, question string, model string, searchResults []SearchResult, history string) (string, error) {
	// Monta contexto com os arquivos encontrados
	context := "CÓDIGO RELEVANTE ENCONTRADO:\n\n"

	for _, r := range searchResults {
		context += fmt.Sprintf("--- %s (chunk %d, similaridade: %.2f) ---\n",
			r.FilePath, r.ChunkIndex, r.Similarity)
		context += r.Content + "\n\n"
	}

	prompt := fmt.Sprintf(`Você é um assistente especializado em análise de código.

%s

HISTÓRICO DA CONVERSA:
%s

PERGUNTA DO USUÁRIO:
%s

DIRETRIZES CRÍTICAS:
1. Responda de forma técnica e objetiva.
2. IMPORTANTE: Se o usuário pedir para REFATORAR, MODIFICAR ou CORRIGIR um arquivo, você DEVE PRIMEIRO usar a ferramenta "get_file" para ler o conteúdo COMPLETO e ATUAL do arquivo. NÃO confie apenas nos trechos acima (chunks), pois podem estar incompletos ou desatualizados.
3. Se a informação não estiver no código fornecido e você precisar ver o arquivo, CHAME a ferramenta "get_file".
4. MASTER AGENT - CONTEXTO DO SISTEMA EVA:
   - Você tem acesso ao ecossistema EVA completo.
   - TABELAS IMPORTANTES: 'idosos' (perfis), 'sinais_vitais_health' (batimentos, spo2 do relógio), 'atividade' (passos), 'alertas', 'medicamentos', 'historico_ligacoes' (memória de voz).
   - NOTIFICAÇÕES: Para enviar alertas para o celular, use 'call_api' no endpoint 'POST http://localhost:8000/api/idosos/{id}/notify?title=...&body=...'.
   - SAÚDE / RELÓGIO: 
     - Para ver dados: Use 'query_database' na tabela 'sinais_vitais_health' ou 'atividade'.
     - Para FORÇAR Sincronização (Silent Push): Use 'call_api' no endpoint 'POST http://localhost:8000/api/idosos/{id}/sync-device'. (Isso acorda o App Android em background).
     - Para Sync via Nuvem (Backup): Link legado 'POST http://localhost:8080/api/google/fit/sync/{id}'.
5. VISUALIZAÇÃO: Para explicar fluxos ou arquitetura, GERE OBRIGATORIAMENTE um bloco de código Markdown com o identificador 'mermaid'.
Exemplo:
`+"`"+`mermaid
sequenceDiagram
...
`+"`"+`
Suporte a: sequenceDiagram, graph TD, erDiagram, classDiagram.
IMPORTANTE: NÃO gere HTML ou ASCII art. APENAS BLOCOS DE CÓDIGO MERMAID.`, context, history, question)

	// Seleção de Modelo
	log.Printf("🤖 Gerando resposta com modelo: %s", model)

	// 1. GPT-4 (OpenAI) - Específico
	if model == "gpt4" {
		useGPT4, _ := strconv.ParseBool(os.Getenv("USE_GPT4"))
		if !useGPT4 {
			return "", fmt.Errorf("GPT-4 não está ativado no .env (USE_GPT4=false)")
		}
		openaiKey := os.Getenv("OPENAI_API_KEY")
		if openaiKey == "" {
			return "", fmt.Errorf("OpenAI API Key não configurada")
		}
		return callOpenAI(prompt, openaiKey)
	}

	// 2. Claude (Anthropic) - Específico
	if model == "claude" {
		useClaude, _ := strconv.ParseBool(os.Getenv("USE_CLAUDE"))
		if !useClaude {
			return "", fmt.Errorf("Claude não está ativado no .env (USE_CLAUDE=false)")
		}
		anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
		if anthropicKey == "" {
			return "", fmt.Errorf("Anthropic API Key não configurada")
		}
		return callClaude(prompt, anthropicKey)
	}

	// 3. Grok - Específico ou Auto
	if model == "grok" || (model == "auto" && useGrok) || (model == "" && useGrok) {
		if useGrok {
			return callGrok(prompt)
		}
		if model == "grok" {
			return "", fmt.Errorf("Grok não está configurado")
		}
	}

	// 4. Ollama - Específico ou Auto
	if model == "ollama" || (model == "auto" && useOllama) || (model == "" && useOllama) {
		if useOllama {
			return ollamaClient.Call(ctx, prompt)
		}
		if model == "ollama" {
			return "", fmt.Errorf("Ollama não está configurado")
		}
	}

	// 5. Gemini - Específico ou Auto
	if model == "gemini" || (model == "auto" && geminiModel != nil) || (model == "" && geminiModel != nil) {
		if geminiModel != nil {
			resp, err := geminiModel.GenerateContent(ctx, genai.Text(prompt))
			if err != nil {
				return "", err
			}
			if resp == nil || len(resp.Candidates) == 0 {
				return "", fmt.Errorf("resposta vazia do Gemini")
			}
			var answer string
			for _, part := range resp.Candidates[0].Content.Parts {
				if txt, ok := part.(genai.Text); ok {
					answer += string(txt)
				}
			}
			return answer, nil
		}
		if model == "gemini" {
			return "", fmt.Errorf("Gemini não está configurado")
		}
	}

	return "", fmt.Errorf("nenhum modelo disponível para '%s'", model)
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// ============================================
// HANDLERS HTTP
// ============================================

func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Método não permitido")
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	if req.Limit == 0 {
		req.Limit = 5
	}

	results, err := searchCodebase(r.Context(), req.Query, req.Limit)
	if err != nil {
		log.Println("Erro na busca:", err)
		respondWithError(w, http.StatusInternalServerError, "Erro ao buscar: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondWithError(w, http.StatusMethodNotAllowed, "Método não permitido")
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	// Session Management
	if req.SessionID == "" {
		// New Session
		var sessionID string
		err := db.QueryRowContext(r.Context(), "INSERT INTO conversation_sessions (user_id) VALUES ($1) RETURNING id", "default_user").Scan(&sessionID)
		if err != nil {
			log.Printf("Erro ao criar sessão: %v", err)
			// Continue without session (stateless fallback)
		} else {
			req.SessionID = sessionID
		}
	} else {
		// Verify if session exists
		var exists bool
		db.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM conversation_sessions WHERE id = $1)", req.SessionID).Scan(&exists)
		if !exists {
			// If invalid provided, recreate
			db.QueryRowContext(r.Context(), "INSERT INTO conversation_sessions (id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", req.SessionID, "default_user")
		}
	}

	// Initialize Memory Service
	memoryService := NewMemoryService(db)

	// Save User Message with metadata (will be populated after code search)
	userMetadata := MessageMetadata{}
	if req.SessionID != "" {
		_, err := memoryService.SaveMessage(r.Context(), req.SessionID, "user", req.Question, userMetadata)
		if err != nil {
			log.Printf("⚠️  Failed to save user message: %v", err)
		}
	}

	// Build Smart Context Window (summary + relevant + recent)
	history := ""
	if req.SessionID != "" {
		contextWindow, err := memoryService.BuildContextWindow(r.Context(), req.SessionID, req.Question)
		if err != nil {
			log.Printf("⚠️  Failed to build context window: %v", err)
			// Fallback to simple recent messages
			recentMessages, _ := memoryService.GetRecentMessages(r.Context(), req.SessionID, 10)
			for _, msg := range recentMessages {
				history += fmt.Sprintf("%s: %s\n", msg.Role, msg.Content)
			}
		} else {
			history = contextWindow
		}
	}

	// Busca código relevante
	searchResults, err := searchHybrid(r.Context(), req.Question, 5)
	if err != nil {
		log.Println("Erro na busca:", err)
		respondWithError(w, http.StatusInternalServerError, "Erro ao buscar código: "+err.Error())
		return
	}

	// Re-ranking (se ativado)
	if useReranking && rerankingService != nil {
		reranked, err := rerankingService.Rerank(r.Context(), req.Question, searchResults)
		if err != nil {
			log.Printf("⚠️  Re-ranking failed, using original results: %v", err)
		} else {
			searchResults = reranked
			log.Printf("🎯 Re-ranking applied to %d results", len(searchResults))
		}
	}

	// Gera resposta (Multi-Agent ou Standard)
	var answer string
	if useMultiAgent && masterAgent != nil {
		// Multi-Agent System
		answer, err = masterAgent.ExecuteMultiAgent(r.Context(), req.Question, searchResults, history)
		if err != nil {
			log.Printf("⚠️  Multi-Agent failed, falling back to standard: %v", err)
			// Fallback to standard generation
			answer, err = generateAnswer(r.Context(), req.Question, req.Model, searchResults, history)
		}
	} else {
		// Standard generation
		answer, err = generateAnswer(r.Context(), req.Question, req.Model, searchResults, history)
	}

	if err != nil {
		log.Println("Erro ao gerar resposta:", err)
		respondWithError(w, http.StatusInternalServerError, "Erro ao gerar resposta: "+err.Error())
		return
	}

	// Extract metadata from search results and tools used
	assistantMetadata := MessageMetadata{
		FilesMentioned: extractFilesFromResults(searchResults),
	}

	// Save Assistant Message with metadata
	if req.SessionID != "" {
		_, err = memoryService.SaveMessage(r.Context(), req.SessionID, "assistant", answer, assistantMetadata)
		if err != nil {
			log.Printf("⚠️  Failed to save assistant message: %v", err)
		}

		// Trigger async summarization check (non-blocking)
		go func() {
			ctx := context.Background()
			_, _ = memoryService.GetOrCreateSummary(ctx, req.SessionID)
		}()
	}

	// Extrai lista de arquivos únicos
	filesMap := make(map[string]bool)
	for _, r := range searchResults {
		filesMap[r.FilePath] = true
	}

	var sourceFiles []string
	for file := range filesMap {
		sourceFiles = append(sourceFiles, file)
	}

	response := ChatResponse{
		Answer:        answer,
		SourceFiles:   sourceFiles,
		SearchResults: searchResults,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	var stats struct {
		TotalFiles  int `json:"total_files"`
		TotalChunks int `json:"total_chunks"`
		Languages   []struct {
			Language string `json:"language"`
			Count    int    `json:"count"`
		} `json:"languages"`
	}

	// Total de arquivos únicos
	db.QueryRow("SELECT COUNT(DISTINCT file_path) FROM project_codebase").Scan(&stats.TotalFiles)

	// Total de chunks
	db.QueryRow("SELECT COUNT(*) FROM project_codebase").Scan(&stats.TotalChunks)

	// Linguagens
	rows, _ := db.Query(`
		SELECT language, COUNT(*) 
		FROM project_codebase 
		GROUP BY language 
		ORDER BY COUNT(*) DESC
	`)
	defer rows.Close()

	for rows.Next() {
		var lang struct {
			Language string `json:"language"`
			Count    int    `json:"count"`
		}
		rows.Scan(&lang.Language, &lang.Count)
		stats.Languages = append(stats.Languages, lang)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// ============================================
// MAIN
// ============================================

func main() {
	defer db.Close()

	// Flags de linha de comando
	indexMode := flag.Bool("index", false, "Modo indexador: varre diretório e indexa código")
	dirPath := flag.String("dir", ".", "Diretório para indexar (usado com -index)")
	forceMode := flag.Bool("force", false, "Forçar re-indexação ignorando hash check")
	flag.Parse()

	// Se estiver no modo indexador, roda e sai
	if *indexMode {
		runIndexer(*dirPath, *forceMode)
		return
	}

	// CORS middleware
	corsMiddleware := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next(w, r)
		}
	}

	// Rotas
	http.HandleFunc("/search", corsMiddleware(handleSearch))
	http.HandleFunc("/chat", corsMiddleware(handleChat))
	http.HandleFunc("/stats", corsMiddleware(handleStats))
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// Servir arquivos estáticos (Frontend)
	fs := http.FileServer(http.Dir("."))
	http.Handle("/", fs)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Code Brain API rodando na porta %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
