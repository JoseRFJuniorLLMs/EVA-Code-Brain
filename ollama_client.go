package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"

	"github.com/tmc/langchaingo/llms/ollama"
)

// OllamaToolClient wraps the standard client to add tool support
type OllamaToolClient struct {
	client *ollama.LLM
}

func NewOllamaToolClient(client *ollama.LLM) *OllamaToolClient {
	return &OllamaToolClient{client: client}
}

// CallWithTools executes a ReAct loop:
// 1. Sends prompt
// 2. Checks for [TOOL_CALL: ...]
// 3. Executes tool
// 4. Sends output back
// 5. Repeat until final answer
func (oc *OllamaToolClient) CallWithTools(ctx context.Context, prompt string, tools map[string]Tool) (string, error) {
	maxTurns := 5
	currentPrompt := prompt

	// Append instructions for tool usage
	toolInstructions := "\n\nCRITICAL INSTRUCTION: If you need to access external data (DB, files, search), you MUST request a tool execution.\n" +
		"Format your request EXACTLY like this:\n" +
		"[TOOL_CALL: tool_name {\"param\": \"value\"}]\n\n" +
		"Available Tools:\n"

	for name, tool := range tools {
		toolInstructions += fmt.Sprintf("- %s: %s\n", name, tool.Description)
	}
	toolInstructions += "\nDo not guess. Use tools to find facts."

	currentPrompt += toolInstructions

	for i := 0; i < maxTurns; i++ {
		log.Printf("🤖 Ollama Turn %d", i+1)

		response, err := oc.client.Call(ctx, currentPrompt)
		if err != nil {
			return "", err
		}

		// Check for tool call
		toolCallPattern := regexp.MustCompile(`\[TOOL_CALL:\s*([a-zA-Z0-9_]+)\s*({.*?})\]`)
		matches := toolCallPattern.FindStringSubmatch(response)

		if len(matches) < 3 {
			// No tool call found - return final response
			return response, nil
		}

		// Tool detected
		toolName := matches[1]
		paramsJSON := matches[2]

		log.Printf("🛠️  Ollama requested tool: %s", toolName)

		// Parse params
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
			log.Printf("⚠️ Invalid JSON from Ollama: %v", err)
			currentPrompt += fmt.Sprintf("\n\nSYSTEM: Invalid JSON format for tool %s. Please retry with valid JSON.", toolName)
			continue
		}

		// Execute Tool
		toolOutput := "Tool not found"
		if tool, exists := tools[toolName]; exists {
			output, err := tool.Execute(ctx, params)
			if err != nil {
				toolOutput = fmt.Sprintf("Error: %v", err)
			} else {
				toolOutput = output
			}
		}

		log.Printf("Results: %s...", truncate(toolOutput, 50))

		// Append result to prompt for next turn
		currentPrompt += fmt.Sprintf("\n%s\n\nSYSTEM: Tool '%s' output:\n%s\n\nNow provide the final answer based on this output.", response, toolName, toolOutput)
	}

	return "Error: Max turns reached without final answer.", nil
}
