package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
)

// Git tool implementations

var gitService *GitService

func initGitService(ctx context.Context) {
	gitService = NewGitService()
	if err := gitService.LoadProjectRoots(ctx); err != nil {
		log.Printf("⚠️  Failed to load Git project roots: %v", err)
	}
}

func executeGitBlame(ctx context.Context, params map[string]interface{}) (string, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'file_path' inválido")
	}

	startLine := 1
	if sl, ok := params["start_line"].(float64); ok {
		startLine = int(sl)
	}

	endLine := 50
	if el, ok := params["end_line"].(float64); ok {
		endLine = int(el)
	}

	// Resolve full path from indexed files
	fullPath, err := resolveFilePath(ctx, filePath)
	if err != nil {
		return "", fmt.Errorf("arquivo não encontrado: %v", err)
	}

	results, err := gitService.Blame(fullPath, startLine, endLine)
	if err != nil {
		return "", fmt.Errorf("git blame falhou: %v", err)
	}

	output := fmt.Sprintf("📝 Git Blame: %s (linhas %d-%d)\n\n", filePath, startLine, endLine)
	for _, r := range results {
		output += fmt.Sprintf("Linha %d | %s | %s | %s\n",
			r.Line, r.Hash[:8], r.Author, r.Date.Format("2006-01-02"))
		output += fmt.Sprintf("  Commit: %s\n", r.CommitMsg)
		output += fmt.Sprintf("  Código: %s\n\n", r.LineContent)
	}

	return output, nil
}

func executeGitLog(ctx context.Context, params map[string]interface{}) (string, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'file_path' inválido")
	}

	limit := 10
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	fullPath, err := resolveFilePath(ctx, filePath)
	if err != nil {
		return "", fmt.Errorf("arquivo não encontrado: %v", err)
	}

	commits, err := gitService.Log(fullPath, limit)
	if err != nil {
		return "", fmt.Errorf("git log falhou: %v", err)
	}

	output := fmt.Sprintf("📜 Histórico de Commits: %s\n\n", filePath)
	for i, c := range commits {
		output += fmt.Sprintf("%d. [%s] %s\n", i+1, c.Hash[:8], c.Message)
		output += fmt.Sprintf("   Autor: %s | Data: %s\n\n", c.Author, c.Date.Format("2006-01-02 15:04"))
	}

	if len(commits) == 0 {
		return "Nenhum commit encontrado para este arquivo.", nil
	}

	return output, nil
}

func executeGitDiff(ctx context.Context, params map[string]interface{}) (string, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'file_path' inválido")
	}

	ref1 := "HEAD~1"
	if r1, ok := params["ref1"].(string); ok {
		ref1 = r1
	}

	ref2 := "HEAD"
	if r2, ok := params["ref2"].(string); ok {
		ref2 = r2
	}

	fullPath, err := resolveFilePath(ctx, filePath)
	if err != nil {
		return "", fmt.Errorf("arquivo não encontrado: %v", err)
	}

	diff, err := gitService.Diff(fullPath, ref1, ref2)
	if err != nil {
		return "", fmt.Errorf("git diff falhou: %v", err)
	}

	if diff == "" {
		return fmt.Sprintf("Nenhuma diferença entre %s e %s para %s", ref1, ref2, filePath), nil
	}

	output := fmt.Sprintf("🔀 Git Diff: %s (%s..%s)\n\n```diff\n%s\n```", filePath, ref1, ref2, diff)
	return output, nil
}

// resolveFilePath finds the full path of a file from the indexed database
func resolveFilePath(ctx context.Context, filePath string) (string, error) {
	// First try: exact match in database
	var rootPath string
	err := db.QueryRowContext(ctx, `
		SELECT pm.root_path 
		FROM project_codebase pc
		JOIN project_metadata pm ON pc.project_name = pm.project_name
		WHERE pc.file_path = $1
		LIMIT 1`, filePath).Scan(&rootPath)

	if err == nil {
		return filepath.Join(rootPath, filePath), nil
	}

	// Second try: partial match (user might have given relative path)
	err = db.QueryRowContext(ctx, `
		SELECT pm.root_path, pc.file_path
		FROM project_codebase pc
		JOIN project_metadata pm ON pc.project_name = pm.project_name
		WHERE pc.file_path LIKE $1
		LIMIT 1`, "%"+filePath).Scan(&rootPath, &filePath)

	if err == nil {
		return filepath.Join(rootPath, filePath), nil
	}

	return "", fmt.Errorf("arquivo '%s' não encontrado no índice", filePath)
}
