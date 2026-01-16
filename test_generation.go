package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/google/generative-ai-go/genai"
)

// TestGenerationService handles automatic test generation
type TestGenerationService struct{}

// NewTestGenerationService creates a new test generation service
func NewTestGenerationService() *TestGenerationService {
	return &TestGenerationService{}
}

// GenerateTests generates unit tests for a given function/class
func (tgs *TestGenerationService) GenerateTests(ctx context.Context, filePath, functionName, language string) (string, error) {
	// Read the source code
	sourceCode, err := tgs.readSourceCode(ctx, filePath, functionName)
	if err != nil {
		return "", err
	}

	// Generate test code using LLM
	testCode, err := tgs.generateTestCode(ctx, sourceCode, functionName, language)
	if err != nil {
		return "", err
	}

	return testCode, nil
}

// readSourceCode retrieves the source code for a function
func (tgs *TestGenerationService) readSourceCode(ctx context.Context, filePath, functionName string) (string, error) {
	// Query database for the function code
	query := `
		SELECT content
		FROM project_codebase
		WHERE file_path = $1
		  AND (symbol_name = $2 OR content ILIKE $3)
		ORDER BY chunk_index
		LIMIT 5`

	rows, err := db.QueryContext(ctx, query, filePath, functionName, "%"+functionName+"%")
	if err != nil {
		return "", fmt.Errorf("failed to read source code: %w", err)
	}
	defer rows.Close()

	var sourceCode strings.Builder
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			continue
		}
		sourceCode.WriteString(content)
		sourceCode.WriteString("\n")
	}

	if sourceCode.Len() == 0 {
		return "", fmt.Errorf("function '%s' not found in '%s'", functionName, filePath)
	}

	return sourceCode.String(), nil
}

// generateTestCode uses LLM to generate test code
func (tgs *TestGenerationService) generateTestCode(ctx context.Context, sourceCode, functionName, language string) (string, error) {
	prompt := tgs.buildTestPrompt(sourceCode, functionName, language)

	// Use configured LLM
	var testCode string
	var err error

	if useOllama && ollamaClient != nil {
		testCode, err = ollamaClient.Call(ctx, prompt)
	} else if useGrok {
		testCode, err = callGrok(prompt)
	} else if geminiModel != nil {
		testCode, err = tgs.callGemini(ctx, prompt)
	} else {
		return "", fmt.Errorf("no LLM available for test generation")
	}

	if err != nil {
		return "", fmt.Errorf("test generation failed: %w", err)
	}

	return testCode, nil
}

// callGemini uses Gemini for test generation
func (tgs *TestGenerationService) callGemini(ctx context.Context, prompt string) (string, error) {
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

// buildTestPrompt constructs the prompt for test generation
func (tgs *TestGenerationService) buildTestPrompt(sourceCode, functionName, language string) string {
	var prompt strings.Builder

	prompt.WriteString(fmt.Sprintf("Generate comprehensive unit tests for this %s function.\n\n", language))
	prompt.WriteString("SOURCE CODE:\n")
	prompt.WriteString("```" + language + "\n")
	prompt.WriteString(sourceCode)
	prompt.WriteString("\n```\n\n")

	prompt.WriteString("REQUIREMENTS:\n")
	prompt.WriteString("1. Include tests for:\n")
	prompt.WriteString("   - Happy path (normal execution)\n")
	prompt.WriteString("   - Edge cases (boundary conditions)\n")
	prompt.WriteString("   - Error handling (invalid inputs)\n")
	prompt.WriteString("   - Null/nil handling\n")
	prompt.WriteString("2. Use appropriate testing framework:\n")

	switch language {
	case "go":
		prompt.WriteString("   - Use Go's testing package\n")
		prompt.WriteString("   - Use table-driven tests where appropriate\n")
		prompt.WriteString("   - Include t.Run() for subtests\n")
		prompt.WriteString("   - Mock external dependencies if needed\n")
	case "python":
		prompt.WriteString("   - Use pytest\n")
		prompt.WriteString("   - Use @pytest.mark.parametrize for multiple cases\n")
		prompt.WriteString("   - Mock external dependencies with unittest.mock\n")
		prompt.WriteString("   - Include fixtures if needed\n")
	case "javascript", "typescript":
		prompt.WriteString("   - Use Jest or Vitest\n")
		prompt.WriteString("   - Use describe/it blocks\n")
		prompt.WriteString("   - Mock dependencies with jest.mock()\n")
		prompt.WriteString("   - Include setup/teardown if needed\n")
	case "dart":
		prompt.WriteString("   - Use Flutter test package\n")
		prompt.WriteString("   - Use test() and group()\n")
		prompt.WriteString("   - Mock with mockito if needed\n")
	}

	prompt.WriteString("3. Follow best practices:\n")
	prompt.WriteString("   - Clear test names describing what is tested\n")
	prompt.WriteString("   - Arrange-Act-Assert pattern\n")
	prompt.WriteString("   - One assertion per test (when possible)\n")
	prompt.WriteString("   - Independent tests (no shared state)\n\n")

	prompt.WriteString("Generate ONLY the test code, properly formatted and ready to use.\n")
	prompt.WriteString("Include necessary imports and setup.\n")

	return prompt.String()
}

// GenerateTestsForFile generates tests for an entire file
func (tgs *TestGenerationService) GenerateTestsForFile(ctx context.Context, filePath string) (string, error) {
	// Detect language
	language := detectLanguageFromPath(filePath)

	// Get all functions/classes in the file
	query := `
		SELECT DISTINCT symbol_name, chunk_type
		FROM project_codebase
		WHERE file_path = $1
		  AND symbol_name IS NOT NULL
		  AND chunk_type IN ('function', 'class', 'method')
		ORDER BY symbol_name`

	rows, err := db.QueryContext(ctx, query, filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get symbols: %w", err)
	}
	defer rows.Close()

	var allTests strings.Builder
	allTests.WriteString(fmt.Sprintf("// Generated tests for %s\n\n", filePath))

	// Add imports based on language
	allTests.WriteString(tgs.getTestImports(language))
	allTests.WriteString("\n\n")

	count := 0
	for rows.Next() {
		var symbolName, chunkType string
		if err := rows.Scan(&symbolName, &chunkType); err != nil {
			continue
		}

		// Generate tests for this symbol
		tests, err := tgs.GenerateTests(ctx, filePath, symbolName, language)
		if err != nil {
			log.Printf("⚠️  Failed to generate tests for %s: %v", symbolName, err)
			continue
		}

		allTests.WriteString(tests)
		allTests.WriteString("\n\n")
		count++
	}

	if count == 0 {
		return "", fmt.Errorf("no testable symbols found in %s", filePath)
	}

	return allTests.String(), nil
}

// getTestImports returns standard imports for test files
func (tgs *TestGenerationService) getTestImports(language string) string {
	switch language {
	case "go":
		return `import (
	"testing"
)`
	case "python":
		return `import pytest
from unittest.mock import Mock, patch`
	case "javascript":
		return `import { describe, it, expect, jest } from '@jest/globals';`
	case "typescript":
		return `import { describe, it, expect, jest } from '@jest/globals';`
	case "dart":
		return `import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/mockito.dart';`
	default:
		return ""
	}
}

// detectLanguageFromPath detects programming language from file path
func detectLanguageFromPath(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".dart":
		return "dart"
	case ".java":
		return "java"
	default:
		return "unknown"
	}
}

// GenerateMocks generates mock objects for dependencies
func (tgs *TestGenerationService) GenerateMocks(ctx context.Context, interfaceName, language string) (string, error) {
	prompt := fmt.Sprintf(`Generate a mock implementation for this interface in %s.

Interface: %s

Requirements:
- Create a mock struct/class
- Implement all interface methods
- Add tracking for method calls
- Include assertion helpers
- Follow %s mocking best practices

Generate ONLY the mock code, ready to use.`, language, interfaceName, language)

	var mockCode string
	var err error

	if useOllama && ollamaClient != nil {
		mockCode, err = ollamaClient.Call(ctx, prompt)
	} else if useGrok {
		mockCode, err = callGrok(prompt)
	} else if geminiModel != nil {
		mockCode, err = tgs.callGemini(ctx, prompt)
	} else {
		return "", fmt.Errorf("no LLM available")
	}

	if err != nil {
		return "", fmt.Errorf("mock generation failed: %w", err)
	}

	return mockCode, nil
}
