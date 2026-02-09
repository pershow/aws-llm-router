package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	User        string        `json:"user,omitempty"`
}

type ChatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   Usage                  `json:"usage"`
}

type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ChatCompletionChunk struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []ChatChunkChoice   `json:"choices"`
	Usage   *Usage              `json:"usage,omitempty"`
	Error   *OpenAIErrorPayload `json:"error,omitempty"`
}

type ChatChunkChoice struct {
	Index        int             `json:"index"`
	Delta        ChatChunkDelta  `json:"delta"`
	FinishReason *string         `json:"finish_reason"`
	Logprobs     json.RawMessage `json:"logprobs,omitempty"`
}

type ChatChunkDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type ModelsResponse struct {
	Object string      `json:"object"`
	Data   []ModelInfo `json:"data"`
}

type ModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ErrorResponse struct {
	Error OpenAIErrorPayload `json:"error"`
}

type OpenAIErrorPayload struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

type contentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func ValidateChatRequest(request ChatCompletionRequest) error {
	if len(request.Messages) == 0 {
		return errors.New("messages cannot be empty")
	}
	return nil
}

func DecodeContentAsText(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", nil
	}

	if strings.HasPrefix(trimmed, "\"") {
		var content string
		if err := json.Unmarshal(raw, &content); err != nil {
			return "", fmt.Errorf("invalid string content: %w", err)
		}
		return content, nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var parts []contentPart
		if err := json.Unmarshal(raw, &parts); err != nil {
			return "", fmt.Errorf("invalid array content: %w", err)
		}
		var builder strings.Builder
		for _, part := range parts {
			if part.Type == "" || part.Type == "text" {
				builder.WriteString(part.Text)
			}
		}
		return builder.String(), nil
	}

	if strings.HasPrefix(trimmed, "{") {
		var part contentPart
		if err := json.Unmarshal(raw, &part); err != nil {
			return "", fmt.Errorf("invalid object content: %w", err)
		}
		if part.Type == "" || part.Type == "text" {
			return part.Text, nil
		}
		return "", nil
	}

	return "", errors.New("unsupported content format")
}

func RenderMessagesForLog(messages []ChatMessage, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}

	var builder strings.Builder
	for _, message := range messages {
		text, err := DecodeContentAsText(message.Content)
		if err != nil {
			text = "<unparseable-content>"
		}
		line := strings.TrimSpace(fmt.Sprintf("%s: %s", message.Role, text))
		if line == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(line)
		if builder.Len() >= maxChars {
			break
		}
	}

	output := builder.String()
	if len(output) <= maxChars {
		return output
	}
	return output[:maxChars]
}
