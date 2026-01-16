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
	FilePath   string  `json:"file_path"`
	ChunkIndex int     `json:"chunk_index"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"`
}

type ChatRequest struct {
	Question string `json:"question"`
	Model    string `json:"model"` // 'grok', 'ollama', 'gemini', 'auto'
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
	useOllama         bool
	// Grok
	useGrok    bool
	grokApiKey string
	grokModel  string = "grok-4-latest"
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
		if err := rows.Scan(&r.FilePath, &r.ChunkIndex, &r.Content, &r.Similarity); err != nil {
			log.Println("Erro ao escanear linha:", err)
			continue
		}
		results = append(results, r)
	}

	return results, nil
}

func generateAnswer(ctx context.Context, question string, searchResults []SearchResult, model string) (string, error) {
	// Monta contexto com os arquivos encontrados
	context := "CÓDIGO RELEVANTE ENCONTRADO:\n\n"

	for _, r := range searchResults {
		context += fmt.Sprintf("--- %s (chunk %d, similaridade: %.2f) ---\n",
			r.FilePath, r.ChunkIndex, r.Similarity)
		context += r.Content + "\n\n"
	}

	prompt := fmt.Sprintf(`Você é um assistente especializado em análise de código.

%s

PERGUNTA DO USUÁRIO:
%s

Responda de forma técnica e objetiva, citando trechos de código quando relevante. 
Se a informação não estiver no código fornecido, seja honesto sobre isso.`, context, question)

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

	// Busca código relevante
	searchResults, err := searchCodebase(r.Context(), req.Question, 5)
	if err != nil {
		log.Println("Erro na busca:", err)
		respondWithError(w, http.StatusInternalServerError, "Erro ao buscar código: "+err.Error())
		return
	}

	// Gera resposta
	answer, err := generateAnswer(r.Context(), req.Question, searchResults, req.Model)
	if err != nil {
		log.Println("Erro ao gerar resposta:", err)
		respondWithError(w, http.StatusInternalServerError, "Erro ao gerar resposta: "+err.Error())
		return
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
	flag.Parse()

	// Se estiver no modo indexador, roda e sai
	if *indexMode {
		runIndexer(*dirPath)
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
