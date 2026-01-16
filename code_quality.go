package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// CodeQualityService handles code quality analysis
type CodeQualityService struct{}

// QualityReport represents a code quality analysis report
type QualityReport struct {
	FilePath        string
	Language        string
	LinterIssues    []LinterIssue
	Complexity      ComplexityMetrics
	CodeSmells      []CodeSmell
	Duplicates      []DuplicateCode
	OverallScore    float64
	Recommendations []string
}

// LinterIssue represents a linter warning/error
type LinterIssue struct {
	Line     int
	Column   int
	Severity string // "error", "warning", "info"
	Rule     string
	Message  string
}

// ComplexityMetrics represents code complexity measurements
type ComplexityMetrics struct {
	CyclomaticComplexity int
	LinesOfCode          int
	CommentLines         int
	FunctionCount        int
	AverageComplexity    float64
}

// CodeSmell represents a potential code quality issue
type CodeSmell struct {
	Type        string
	Location    string
	Description string
	Severity    string
}

// DuplicateCode represents duplicated code blocks
type DuplicateCode struct {
	File1      string
	File2      string
	Lines      int
	Similarity float64
}

// NewCodeQualityService creates a new code quality service
func NewCodeQualityService() *CodeQualityService {
	return &CodeQualityService{}
}

// AnalyzeFile performs comprehensive quality analysis on a file
func (cqs *CodeQualityService) AnalyzeFile(ctx context.Context, filePath string) (*QualityReport, error) {
	language := detectLanguageFromPath(filePath)
	if language == "unknown" {
		return nil, fmt.Errorf("unsupported file type: %s", filePath)
	}

	report := &QualityReport{
		FilePath:        filePath,
		Language:        language,
		LinterIssues:    []LinterIssue{},
		CodeSmells:      []CodeSmell{},
		Duplicates:      []DuplicateCode{},
		Recommendations: []string{},
	}

	// Run linter
	issues, err := cqs.runLinter(filePath, language)
	if err != nil {
		log.Printf("⚠️  Linter failed: %v", err)
	} else {
		report.LinterIssues = issues
	}

	// Calculate complexity
	complexity, err := cqs.calculateComplexity(ctx, filePath, language)
	if err != nil {
		log.Printf("⚠️  Complexity analysis failed: %v", err)
	} else {
		report.Complexity = complexity
	}

	// Detect code smells
	smells := cqs.detectCodeSmells(ctx, filePath, language, complexity)
	report.CodeSmells = smells

	// Calculate overall score
	report.OverallScore = cqs.calculateScore(report)

	// Generate recommendations
	report.Recommendations = cqs.generateRecommendations(report)

	return report, nil
}

// runLinter executes appropriate linter for the language
func (cqs *CodeQualityService) runLinter(filePath, language string) ([]LinterIssue, error) {
	var cmd *exec.Cmd
	var issues []LinterIssue

	switch language {
	case "go":
		// golangci-lint run --out-format json file.go
		cmd = exec.Command("golangci-lint", "run", "--out-format", "json", filePath)
	case "python":
		// pylint --output-format=json file.py
		cmd = exec.Command("pylint", "--output-format=json", filePath)
	case "javascript", "typescript":
		// eslint --format json file.js
		cmd = exec.Command("eslint", "--format", "json", filePath)
	default:
		return issues, fmt.Errorf("no linter configured for %s", language)
	}

	output, err := cmd.Output()
	if err != nil {
		// Linters often return non-zero exit code when issues are found
		// Check if we got output anyway
		if len(output) == 0 {
			return issues, fmt.Errorf("linter execution failed: %w", err)
		}
	}

	// Parse linter output
	issues = cqs.parseLinterOutput(output, language)
	return issues, nil
}

// parseLinterOutput parses linter JSON output
func (cqs *CodeQualityService) parseLinterOutput(output []byte, language string) []LinterIssue {
	var issues []LinterIssue

	switch language {
	case "go":
		// golangci-lint format
		var result struct {
			Issues []struct {
				Pos struct {
					Line   int `json:"line"`
					Column int `json:"column"`
				} `json:"pos"`
				Severity   string `json:"severity"`
				FromLinter string `json:"fromLinter"`
				Text       string `json:"text"`
			} `json:"Issues"`
		}
		if err := json.Unmarshal(output, &result); err == nil {
			for _, issue := range result.Issues {
				issues = append(issues, LinterIssue{
					Line:     issue.Pos.Line,
					Column:   issue.Pos.Column,
					Severity: issue.Severity,
					Rule:     issue.FromLinter,
					Message:  issue.Text,
				})
			}
		}

	case "python":
		// pylint format
		var result []struct {
			Line    int    `json:"line"`
			Column  int    `json:"column"`
			Type    string `json:"type"`
			Symbol  string `json:"symbol"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(output, &result); err == nil {
			for _, issue := range result {
				severity := "warning"
				if issue.Type == "error" || issue.Type == "fatal" {
					severity = "error"
				}
				issues = append(issues, LinterIssue{
					Line:     issue.Line,
					Column:   issue.Column,
					Severity: severity,
					Rule:     issue.Symbol,
					Message:  issue.Message,
				})
			}
		}

	case "javascript", "typescript":
		// eslint format
		var result []struct {
			Messages []struct {
				Line     int    `json:"line"`
				Column   int    `json:"column"`
				Severity int    `json:"severity"`
				RuleId   string `json:"ruleId"`
				Message  string `json:"message"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(output, &result); err == nil {
			for _, file := range result {
				for _, msg := range file.Messages {
					severity := "warning"
					if msg.Severity == 2 {
						severity = "error"
					}
					issues = append(issues, LinterIssue{
						Line:     msg.Line,
						Column:   msg.Column,
						Severity: severity,
						Rule:     msg.RuleId,
						Message:  msg.Message,
					})
				}
			}
		}
	}

	return issues
}

// calculateComplexity calculates code complexity metrics
func (cqs *CodeQualityService) calculateComplexity(ctx context.Context, filePath, language string) (ComplexityMetrics, error) {
	metrics := ComplexityMetrics{}

	// Get file content from database
	query := `
		SELECT content, chunk_type
		FROM project_codebase
		WHERE file_path = $1
		ORDER BY chunk_index`

	rows, err := db.QueryContext(ctx, query, filePath)
	if err != nil {
		return metrics, err
	}
	defer rows.Close()

	var totalLines int
	var commentLines int
	var functionCount int
	var totalComplexity int

	for rows.Next() {
		var content, chunkType string
		if err := rows.Scan(&content, &chunkType); err != nil {
			continue
		}

		lines := strings.Split(content, "\n")
		totalLines += len(lines)

		// Count comments
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "/*") {
				commentLines++
			}
		}

		// Count functions and estimate complexity
		if chunkType == "function" || chunkType == "method" {
			functionCount++
			complexity := cqs.estimateComplexity(content, language)
			totalComplexity += complexity
		}
	}

	metrics.LinesOfCode = totalLines
	metrics.CommentLines = commentLines
	metrics.FunctionCount = functionCount
	metrics.CyclomaticComplexity = totalComplexity

	if functionCount > 0 {
		metrics.AverageComplexity = float64(totalComplexity) / float64(functionCount)
	}

	return metrics, nil
}

// estimateComplexity estimates cyclomatic complexity
func (cqs *CodeQualityService) estimateComplexity(code, language string) int {
	complexity := 1 // Base complexity

	// Count decision points
	keywords := []string{"if", "else", "for", "while", "case", "catch", "&&", "||", "?"}

	code = strings.ToLower(code)
	for _, keyword := range keywords {
		complexity += strings.Count(code, keyword)
	}

	return complexity
}

// detectCodeSmells identifies potential code quality issues
func (cqs *CodeQualityService) detectCodeSmells(ctx context.Context, filePath, language string, complexity ComplexityMetrics) []CodeSmell {
	var smells []CodeSmell

	// High complexity
	if complexity.AverageComplexity > 10 {
		smells = append(smells, CodeSmell{
			Type:        "High Complexity",
			Location:    filePath,
			Description: fmt.Sprintf("Average complexity %.1f exceeds recommended threshold of 10", complexity.AverageComplexity),
			Severity:    "warning",
		})
	}

	// Low comment ratio
	if complexity.LinesOfCode > 0 {
		commentRatio := float64(complexity.CommentLines) / float64(complexity.LinesOfCode)
		if commentRatio < 0.1 && complexity.LinesOfCode > 50 {
			smells = append(smells, CodeSmell{
				Type:        "Low Documentation",
				Location:    filePath,
				Description: fmt.Sprintf("Comment ratio %.1f%% is below recommended 10%%", commentRatio*100),
				Severity:    "info",
			})
		}
	}

	// Large file
	if complexity.LinesOfCode > 500 {
		smells = append(smells, CodeSmell{
			Type:        "Large File",
			Location:    filePath,
			Description: fmt.Sprintf("File has %d lines, consider splitting into smaller modules", complexity.LinesOfCode),
			Severity:    "warning",
		})
	}

	// Too many functions
	if complexity.FunctionCount > 20 {
		smells = append(smells, CodeSmell{
			Type:        "Too Many Functions",
			Location:    filePath,
			Description: fmt.Sprintf("File has %d functions, consider refactoring", complexity.FunctionCount),
			Severity:    "info",
		})
	}

	return smells
}

// calculateScore calculates overall quality score (0-100)
func (cqs *CodeQualityService) calculateScore(report *QualityReport) float64 {
	score := 100.0

	// Deduct for linter issues
	for _, issue := range report.LinterIssues {
		if issue.Severity == "error" {
			score -= 5
		} else if issue.Severity == "warning" {
			score -= 2
		} else {
			score -= 0.5
		}
	}

	// Deduct for code smells
	for _, smell := range report.CodeSmells {
		if smell.Severity == "error" {
			score -= 10
		} else if smell.Severity == "warning" {
			score -= 5
		} else {
			score -= 2
		}
	}

	// Ensure score is between 0 and 100
	if score < 0 {
		score = 0
	}

	return score
}

// generateRecommendations provides actionable improvement suggestions
func (cqs *CodeQualityService) generateRecommendations(report *QualityReport) []string {
	var recommendations []string

	// Based on linter issues
	errorCount := 0
	warningCount := 0
	for _, issue := range report.LinterIssues {
		if issue.Severity == "error" {
			errorCount++
		} else if issue.Severity == "warning" {
			warningCount++
		}
	}

	if errorCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("Fix %d linter errors immediately", errorCount))
	}
	if warningCount > 5 {
		recommendations = append(recommendations, fmt.Sprintf("Address %d linter warnings to improve code quality", warningCount))
	}

	// Based on complexity
	if report.Complexity.AverageComplexity > 10 {
		recommendations = append(recommendations, "Refactor complex functions to reduce cyclomatic complexity")
	}

	// Based on code smells
	for _, smell := range report.CodeSmells {
		switch smell.Type {
		case "High Complexity":
			recommendations = append(recommendations, "Break down complex functions into smaller, testable units")
		case "Low Documentation":
			recommendations = append(recommendations, "Add comments and documentation to improve maintainability")
		case "Large File":
			recommendations = append(recommendations, "Split large file into smaller, focused modules")
		}
	}

	// General recommendations
	if report.OverallScore < 70 {
		recommendations = append(recommendations, "Consider a comprehensive code review and refactoring session")
	}

	return recommendations
}

// AnalyzeProject analyzes quality across entire project
func (cqs *CodeQualityService) AnalyzeProject(ctx context.Context, projectName string) (map[string]*QualityReport, error) {
	// Get all files in project
	query := `
		SELECT DISTINCT file_path
		FROM project_codebase
		WHERE project_name = $1
		ORDER BY file_path`

	rows, err := db.QueryContext(ctx, query, projectName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	reports := make(map[string]*QualityReport)
	for rows.Next() {
		var filePath string
		if err := rows.Scan(&filePath); err != nil {
			continue
		}

		// Analyze each file
		report, err := cqs.AnalyzeFile(ctx, filePath)
		if err != nil {
			log.Printf("⚠️  Failed to analyze %s: %v", filePath, err)
			continue
		}

		reports[filePath] = report
	}

	return reports, nil
}
