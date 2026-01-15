package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pgvector/pgvector-go"
	"github.com/tmc/langchaingo/llms/ollama"
)

// Configuração do Indexador
const (
	ChunkSize    = 2000
	ChunkOverlap = 200
)

var ignoreDirs = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true, "dist": true, "build": true,
	"venv": true, ".idea": true, ".vscode": true, "vendor": true, "target": true, "bin": true,
	"obj": true, "out": true, ".next": true, ".nuxt": true, "coverage": true,
}

var extensions = map[string]bool{
	".py": true, ".go": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".java": true, ".c": true, ".cpp": true, ".cs": true, ".php": true, ".rb": true,
	".rs": true, ".swift": true, ".kt": true, ".sql": true, ".html": true, ".css": true,
	".vue": true, ".svelte": true, ".scala": true, ".r": true, ".m": true, ".sh": true,
}

func chunks(text string, size, overlap int) []string {
	var results []string
	runes := []rune(text)
	l := len(runes)
	start := 0

	for start < l {
		end := start + size
		if end > l {
			end = l
		}

		// Ajuste para não quebrar no meio da linha se possível
		if end < l {
			// Procura a última quebra de linha no chunk
			slice := runes[start:end]
			lastNewline := -1
			for i := len(slice) - 1; i >= 0; i-- {
				if slice[i] == '\n' {
					lastNewline = i
					break
				}
			}
			// Se encontrou uma quebra de linha razoável (após 50% do chunk)
			if lastNewline > size/2 {
				end = start + lastNewline + 1 // Inclui o \n
			}
		}

		chunk := string(runes[start:end])
		results = append(results, chunk)

		if end == l {
			break
		}
		start = end - overlap
	}
	return results
}

func getFileHash(content string) string {
	hash := md5.Sum([]byte(content))
	return hex.EncodeToString(hash[:])
}

func getEmbedding(ctx context.Context, text string) ([]float32, error) {
	if indexerUseOllama {
		embeddings, err := indexerOllamaEmbedClient.CreateEmbedding(ctx, []string{text})
		if err != nil {
			return nil, err
		}
		if len(embeddings) == 0 || len(embeddings[0]) == 0 {
			return nil, fmt.Errorf("embedding vazio")
		}
		return embeddings[0], nil
	}
	// Fallback or error if no embedding client is configured
	return nil, fmt.Errorf("nenhum cliente de embedding configurado")
}

func processFile(ctx context.Context, path string, rootDir string) (int, error) {
	contentBytes, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	content := string(contentBytes)

	// Hash e checks
	fileHash := getFileHash(content)
	fileSize := len(contentBytes)
	relPath, _ := filepath.Rel(rootDir, path)
	ext := filepath.Ext(path)

	// Verifica se mudou
	var existingID int
	err = db.QueryRowContext(ctx,
		"SELECT id FROM project_codebase WHERE file_path = $1 AND last_modified_hash = $2 LIMIT 1",
		relPath, fileHash).Scan(&existingID)

	if err == nil {
		fmt.Printf("⏭️  Pulando (não mudou): %s\n", relPath)
		return 0, nil
	}

	// Remove antigo
	_, err = db.ExecContext(ctx, "DELETE FROM project_codebase WHERE file_path = $1", relPath)
	if err != nil {
		return 0, fmt.Errorf("erro ao limpar antigos: %v", err)
	}

	// Chunka e insere
	textChunks := chunks(content, ChunkSize, ChunkOverlap)
	if len(textChunks) == 0 {
		return 0, nil
	}

	fmt.Printf("📄 Processando: %s (%d chunks)\n", relPath, len(textChunks))

	inserted := 0
	for i, chunkText := range textChunks {
		embedding, err := getEmbedding(ctx, chunkText)
		if err != nil {
			log.Printf("⚠️  Erro no embedding chunk %d de %s: %v", i, relPath, err)
			continue
		}

		_, err = db.ExecContext(ctx, `
			INSERT INTO project_codebase 
			(file_path, chunk_index, content, embedding, last_modified_hash, file_size, language)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			relPath, i, chunkText, pgvector.NewVector(embedding), fileHash, fileSize, ext,
		)

		if err != nil {
			log.Printf("⚠️  Erro ao inserir chunk %d: %v", i, err)
		} else {
			inserted++
		}
	}

	return inserted, nil
}

func runIndexer(rootDir string) {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("🧠 CODE BRAIN - Indexador (Go Edition)")
	fmt.Println(strings.Repeat("=", 60))

	absRoot, _ := filepath.Abs(rootDir)
	projectName := filepath.Base(absRoot)
	fmt.Printf("📁 Projeto: %s\n", projectName)
	fmt.Printf("📂 Diretório: %s\n\n", absRoot)

	ctx := context.Background()

	// Initialize embedding client
	if os.Getenv("USE_OLLAMA_EMBED") == "true" {
		indexerUseOllama = true
	}

	if indexerUseOllama {
		fmt.Println("🦙 Modo OLLAMA ativado")

		// Chat client (opcional no indexer, mas mantemos)
		llm, err := ollama.New(ollama.WithModel(os.Getenv("OLLAMA_MODEL")))
		if err != nil {
			log.Fatal("Erro Ollama Chat:", err)
		}
		indexerOllamaClient = llm

		// Embed client (CRÍTICO)
		embedModel := os.Getenv("OLLAMA_EMBED_MODEL")
		if embedModel == "" {
			embedModel = "nomic-embed-text"
		}

		embedLlm, err := ollama.New(ollama.WithModel(embedModel))
		if err != nil {
			log.Fatal("Erro Ollama Embed:", err)
		}
		indexerOllamaEmbedClient = embedLlm

		fmt.Printf("✅ Config: Chat=%s, Embed=%s\n", os.Getenv("OLLAMA_MODEL"), embedModel)
	} else {
		// TODO: Initialize Gemini client here if not using Ollama
		fmt.Println("✨ Modo GEMINI ativado (ainda não implementado para embedding)")
		log.Fatal("Erro: Gemini embedding não implementado. Use USE_OLLAMA_EMBED=true")
	}

	var files []string

	fmt.Println("🔍 Varrendo diretório...")
	filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if ignoreDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if extensions[filepath.Ext(path)] {
			files = append(files, path)
		}
		return nil
	})

	fmt.Printf("📊 Encontrados %d arquivos de código\n\n", len(files))

	startTime := time.Now()
	totalChunks := 0
	filesProcessed := 0

	for _, file := range files {
		chunksCount, err := processFile(ctx, file, rootDir)
		if err != nil {
			log.Printf("Erro em %s: %v", file, err)
		} else if chunksCount > 0 {
			totalChunks += chunksCount
			filesProcessed++
		}
	}

	// Update Metadata
	_, err := db.ExecContext(ctx, `
		INSERT INTO project_metadata (project_name, root_path, total_files, total_chunks, last_indexed)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (project_name) 
		DO UPDATE SET total_files=$3, total_chunks=$4, last_indexed=NOW()
	`, projectName, absRoot, filesProcessed, totalChunks)

	if err != nil {
		log.Printf("Erro ao atualizar metadados: %v", err)
	}

	elapsed := time.Since(startTime)
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ INDEXAÇÃO CONCLUÍDA!")
	fmt.Printf("📊 Processados: %d/%d arquivos\n", filesProcessed, len(files))
	fmt.Printf("📦 Chunks gerados: %d\n", totalChunks)
	fmt.Printf("⏱️  Tempo: %s\n", elapsed)
	fmt.Println(strings.Repeat("=", 60))
}
