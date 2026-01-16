package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type OpenAIRequest struct {
	Model      string                   `json:"model"`
	Messages   []OpenAIMessage          `json:"messages"`
	Tools      []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice string                   `json:"tool_choice,omitempty"`
}

type OpenAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type OpenAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type OpenAIResponse struct {
	Choices []struct {
		Message      OpenAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
}

func callOpenAI(prompt string, apiKey string) (string, error) {
	return callOpenAIWithTools(context.Background(), prompt, apiKey)
}

func callOpenAIWithTools(ctx context.Context, prompt string, apiKey string) (string, error) {
	messages := []OpenAIMessage{
		{Role: "user", Content: prompt},
	}

	// Prepara tools no formato OpenAI
	tools := make([]map[string]interface{}, 0)
	for _, tool := range toolRegistry.List() {
		tools = append(tools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.Name,
				"description": tool.Description,
				"parameters":  tool.Parameters,
			},
		})
	}

	maxIterations := 5 // Previne loops infinitos
	for i := 0; i < maxIterations; i++ {
		reqBody := OpenAIRequest{
			Model:      "gpt-4-turbo-preview",
			Messages:   messages,
			Tools:      tools,
			ToolChoice: "auto",
		}

		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return "", err
		}

		req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			return "", err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return "", fmt.Errorf("erro OpenAI (%d): %s", resp.StatusCode, string(body))
		}

		var openaiResp OpenAIResponse
		if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
			return "", err
		}

		if len(openaiResp.Choices) == 0 {
			return "", fmt.Errorf("resposta vazia do OpenAI")
		}

		choice := openaiResp.Choices[0]
		messages = append(messages, choice.Message)

		// Se não tem tool calls, retorna a resposta final
		if len(choice.Message.ToolCalls) == 0 {
			return choice.Message.Content, nil
		}

		// Executa tool calls
		log.Printf("🔧 GPT-4 solicitou %d tool calls", len(choice.Message.ToolCalls))
		for _, toolCall := range choice.Message.ToolCalls {
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				log.Printf("❌ Erro ao parsear argumentos: %v", err)
				continue
			}

			log.Printf("  → %s(%v)", toolCall.Function.Name, args)
			result, err := toolRegistry.Execute(ctx, toolCall.Function.Name, args)
			if err != nil {
				result = fmt.Sprintf("Erro ao executar ferramenta: %v", err)
				log.Printf("  ❌ %v", err)
			} else {
				log.Printf("  ✅ Executado com sucesso")
			}

			// Adiciona resultado como mensagem
			messages = append(messages, OpenAIMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			})
		}

		// Continua o loop para obter resposta final
	}

	return "", fmt.Errorf("máximo de iterações atingido")
}
