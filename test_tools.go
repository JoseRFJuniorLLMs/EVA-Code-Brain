package main

import (
	"context"
	"fmt"
)

// Test generation tool implementations

var testGenService *TestGenerationService

func initTestGeneration() {
	testGenService = NewTestGenerationService()
}

func executeGenerateTests(ctx context.Context, params map[string]interface{}) (string, error) {
	filePath, ok := params["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'file_path' inválido")
	}

	functionName, _ := params["function_name"].(string)

	// Detect language
	language := detectLanguageFromPath(filePath)
	if language == "unknown" {
		return "", fmt.Errorf("linguagem não suportada para %s", filePath)
	}

	var tests string
	var err error

	if functionName != "" {
		// Generate tests for specific function
		tests, err = testGenService.GenerateTests(ctx, filePath, functionName, language)
		if err != nil {
			return "", fmt.Errorf("falha ao gerar testes: %w", err)
		}
	} else {
		// Generate tests for entire file
		tests, err = testGenService.GenerateTestsForFile(ctx, filePath)
		if err != nil {
			return "", fmt.Errorf("falha ao gerar testes: %w", err)
		}
	}

	// Format output
	output := fmt.Sprintf("🧪 **Testes Gerados para %s**\n\n", filePath)
	if functionName != "" {
		output += fmt.Sprintf("Função: `%s`\n\n", functionName)
	}
	output += "```" + language + "\n"
	output += tests
	output += "\n```\n\n"
	output += "**Próximos passos:**\n"
	output += "1. Revise os testes gerados\n"
	output += "2. Ajuste conforme necessário\n"
	output += "3. Execute os testes\n"

	return output, nil
}

func executeGenerateMocks(ctx context.Context, params map[string]interface{}) (string, error) {
	interfaceName, ok := params["interface_name"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'interface_name' inválido")
	}

	language, ok := params["language"].(string)
	if !ok {
		return "", fmt.Errorf("parâmetro 'language' inválido")
	}

	mockCode, err := testGenService.GenerateMocks(ctx, interfaceName, language)
	if err != nil {
		return "", fmt.Errorf("falha ao gerar mock: %w", err)
	}

	// Format output
	output := fmt.Sprintf("🎭 **Mock Gerado para %s**\n\n", interfaceName)
	output += "```" + language + "\n"
	output += mockCode
	output += "\n```\n\n"
	output += "**Como usar:**\n"
	output += "1. Copie o mock para seu arquivo de testes\n"
	output += "2. Configure o comportamento esperado\n"
	output += "3. Use nos testes unitários\n"

	return output, nil
}
