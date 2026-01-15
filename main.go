package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/lib/pq"
	"github.com/pgvector/pgvector-go"
	"google.golang.org/genai"
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
	db           *sql.DB
	geminiClient *genai.Client
	geminiModel  *genai.GenerativeModel
)

func init() {
	var err error

	// Conecta ao PostgreSQL
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgresql://user:password@localhost:5432/codebrain?sslmode=disable"
	}

	db, err = sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Erro ao conectar ao banco:", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal("Banco inacessível:", err)
	}

	log.Println("✅ Conectado ao PostgreSQL")

	// Inicializa Google Gemini
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		log.Fatal("❌ GOOGLE_API_KEY não definida")
	}

	ctx := context.Background()
	geminiClient, err = genai.NewClient(ctx, apiKey)
	if err != nil {
		log.Fatal("Erro ao criar cliente Gemini:", err)
	}

	geminiModel = geminiClient.GenerativeModel("gemini-2.0-flash-exp")
	log.Println("✅ Cliente Gemini inicializado")
}

// ============================================
// FUNÇÕES CORE
// ============================================

func getEmbedding(ctx context.Context, text string) ([]float32, error) {
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
Se a informação não estiver no código fornecido, seja honesto sobre isso.`, context, question)

	// Gera resposta com Gemini
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

// ============================================
// HANDLERS HTTP
// ============================================

func handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	if req.Limit == 0 {
		req.Limit = 5
	}

	results, err := searchCodebase(r.Context(), req.Query, req.Limit)
	if err != nil {
		log.Println("Erro na busca:", err)
		http.Error(w, "Erro ao buscar", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método não permitido", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "JSON inválido", http.StatusBadRequest)
		return
	}

	// Busca código relevante
	searchResults, err := searchCodebase(r.Context(), req.Question, 5)
	if err != nil {
		log.Println("Erro na busca:", err)
		http.Error(w, "Erro ao buscar código", http.StatusInternalServerError)
		return
	}

	// Gera resposta
	answer, err := generateAnswer(r.Context(), req.Question, searchResults)
	if err != nil {
		log.Println("Erro ao gerar resposta:", err)
		http.Error(w, "Erro ao gerar resposta", http.StatusInternalServerError)
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Code Brain API rodando na porta %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
