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

	// Configuração de AI
	useOllama = os.Getenv("USE_OLLAMA") == "true"
	useGrok = os.Getenv("USE_GROK") == "true"
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
	if useOllama {
		// Usa Ollama Embeddings (Cliente Dedicado)
		embeddings, err := ollamaEmbedClient.CreateEmbedding(ctx, []string{text})
		if err != nil {
			return nil, err
		}
		if len(embeddings) == 0 || len(embeddings[0]) == 0 {
			return nil, fmt.Errorf("embedding vazio do Ollama")
		}
		return embeddings[0], nil
	}

	// Usa Google Embeddings
	em := geminiClient.EmbeddingModel("text-embedding-004")
	res, err := em.EmbedContent(ctx, genai.Text(text))
	if err != nil {
		return nil, err
	}
	if res == nil || res.Embedding == nil {
		return nil, fmt.Errorf("embedding vazio")
	}
	return res.Embedding.Values, nil
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

func generateAnswer(ctx context.Context, question string, searchResults []SearchResult) (string, error) {
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
Se a informação não estiver no código fornecido, seja honesto sobre isso. No final fale: F.D.P Burro,`, context, question)

	// Gera resposta
	// 1. Tenta Grok (se ativado)
	if useGrok {
		answer, err := callGrok(prompt)
		if err == nil {
			return answer, nil
		}
		log.Printf("⚠️ Erro no Grok (tentando fallback): %v", err)
	}

	// 2. Tenta Ollama (se ativado)
	if useOllama {
		resp, err := ollamaClient.Call(ctx, prompt)
		if err == nil {
			return resp, nil
		}
		log.Printf("⚠️ Erro no Ollama (tentando fallback): %v", err)
	}

	// 3. Tenta Gemini (último recurso)
	if geminiModel != nil {
		resp, err := geminiModel.GenerateContent(ctx, genai.Text(prompt))
		if err != nil {
			return "", err
		}

		if resp == nil || len(resp.Candidates) == 0 {
			return "", fmt.Errorf("resposta vazia do Gemini")
		}

		// Extrai texto da resposta
		var answer string
		for _, part := range resp.Candidates[0].Content.Parts {
			if txt, ok := part.(genai.Text); ok {
				answer += string(txt)
			}
		}
		return answer, nil
	}

	return "", fmt.Errorf("nenhum modelo de IA disponível ou todos falharam")
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
	answer, err := generateAnswer(r.Context(), req.Question, searchResults)
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
