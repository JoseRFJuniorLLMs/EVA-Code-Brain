package main

import (
	"context"
	"fmt"
)

// Code quality tool implementations

var codeQualityService *CodeQualityService

func initCodeQuality() {
	codeQualityService = NewCodeQualityService()
}

func executeAnalyzeCodeQuality(ctx context.Context, params map[string]interface{}) (string, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'file_path' inválido")
	}

	report, err := codeQualityService.AnalyzeFile(ctx, filePath)
	if err != nil {
		return "", fmt.Errorf("falha na análise: %w", err)
	}

	// Format output
	output := fmt.Sprintf("📊 **Análise de Qualidade: %s**\n\n", filePath)

	// Overall score
	scoreEmoji := "🟢"
	if report.OverallScore < 70 {
		scoreEmoji = "🔴"
	} else if report.OverallScore < 85 {
		scoreEmoji = "🟡"
	}
	output += fmt.Sprintf("**Score Geral:** %s %.1f/100\n\n", scoreEmoji, report.OverallScore)

	// Complexity metrics
	output += "**Métricas de Complexidade:**\n"
	output += fmt.Sprintf("- Linhas de código: %d\n", report.Complexity.LinesOfCode)
	output += fmt.Sprintf("- Linhas de comentário: %d (%.1f%%)\n",
		report.Complexity.CommentLines,
		float64(report.Complexity.CommentLines)/float64(report.Complexity.LinesOfCode)*100)
	output += fmt.Sprintf("- Funções: %d\n", report.Complexity.FunctionCount)
	output += fmt.Sprintf("- Complexidade ciclomática: %d\n", report.Complexity.CyclomaticComplexity)
	output += fmt.Sprintf("- Complexidade média: %.1f\n\n", report.Complexity.AverageComplexity)

	// Linter issues
	if len(report.LinterIssues) > 0 {
		errorCount := 0
		warningCount := 0
		for _, issue := range report.LinterIssues {
			if issue.Severity == "error" {
				errorCount++
			} else if issue.Severity == "warning" {
				warningCount++
			}
		}

		output += fmt.Sprintf("**Problemas do Linter:** %d total\n", len(report.LinterIssues))
		if errorCount > 0 {
			output += fmt.Sprintf("- 🔴 Erros: %d\n", errorCount)
		}
		if warningCount > 0 {
			output += fmt.Sprintf("- 🟡 Avisos: %d\n", warningCount)
		}

		// Show first 5 issues
		output += "\n**Principais Problemas:**\n"
		for i, issue := range report.LinterIssues {
			if i >= 5 {
				output += fmt.Sprintf("... e mais %d problemas\n", len(report.LinterIssues)-5)
				break
			}
			emoji := "🟡"
			if issue.Severity == "error" {
				emoji = "🔴"
			}
			output += fmt.Sprintf("%d. %s Linha %d: %s (%s)\n", i+1, emoji, issue.Line, issue.Message, issue.Rule)
		}
		output += "\n"
	} else {
		output += "✅ **Nenhum problema encontrado pelo linter**\n\n"
	}

	// Code smells
	if len(report.CodeSmells) > 0 {
		output += fmt.Sprintf("**Code Smells:** %d detectados\n", len(report.CodeSmells))
		for i, smell := range report.CodeSmells {
			emoji := "🟡"
			if smell.Severity == "error" {
				emoji = "🔴"
			} else if smell.Severity == "info" {
				emoji = "🔵"
			}
			output += fmt.Sprintf("%d. %s %s: %s\n", i+1, emoji, smell.Type, smell.Description)
		}
		output += "\n"
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		output += "**Recomendações:**\n"
		for i, rec := range report.Recommendations {
			output += fmt.Sprintf("%d. %s\n", i+1, rec)
		}
	}

	return output, nil
}
