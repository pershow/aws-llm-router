package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type ChatCompletionRequest struct {
	Model             string          `json:"model"`
	Messages          []ChatMessage   `json:"messages"`
	Temperature       *float64        `json:"temperature,omitempty"`
	TopP              *float64        `json:"top_p,omitempty"`
	MaxTokens         *int            `json:"max_tokens,omitempty"`
	Stream            bool            `json:"stream,omitempty"`
	User              string          `json:"user,omitempty"`
	Tools             []Tool          `json:"tools,omitempty"`
	ToolChoice        json.RawMessage `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	// Cursor 可能发送的额外字段 - 需要兼容（忽略不报错）
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	Stop             json.RawMessage `json:"stop,omitempty"`
	N                *int            `json:"n,omitempty"`
	LogitBias        json.RawMessage `json:"logit_bias,omitempty"`
	Logprobs         *bool           `json:"logprobs,omitempty"`
	TopLogprobs      *int            `json:"top_logprobs,omitempty"`
	Seed             *int            `json:"seed,omitempty"`
	ResponseFormat   json.RawMessage `json:"response_format,omitempty"`
	StreamOptions    json.RawMessage `json:"stream_options,omitempty"`
}

type ChatMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Name       string          `json:"name,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
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
	Role      string              `json:"role,omitempty"`
	Content   string              `json:"content,omitempty"`
	ToolCalls []ChatChunkToolCall `json:"tool_calls,omitempty"`
}

type ChatChunkToolCall struct {
	Index    int               `json:"index"`
	ID       string            `json:"id,omitempty"`
	Type     string            `json:"type,omitempty"`
	Function *ToolCallFunction `json:"function,omitempty"`
}

type Tool struct {
	Type     string        `json:"type"`
	Function *ToolFunction `json:"function,omitempty"`
	// Cursor/Responses API 格式 - 工具定义直接在顶层
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// GetFunction 返回工具的函数定义，支持两种格式
func (t *Tool) GetFunction() *ToolFunction {
	// 优先使用嵌套的 function 字段
	if t.Function != nil {
		return t.Function
	}
	// 如果没有嵌套的 function，检查顶层字段
	if t.Name != "" {
		return &ToolFunction{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
			Strict:      t.Strict,
		}
	}
	return nil
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
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
		if len(message.ToolCalls) > 0 {
			toolBlob, marshalErr := json.Marshal(message.ToolCalls)
			if marshalErr == nil {
				if strings.TrimSpace(text) == "" {
					text = "tool_calls=" + string(toolBlob)
				} else {
					text += " tool_calls=" + string(toolBlob)
				}
			}
		}
		if strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			toolCallID := strings.TrimSpace(message.ToolCallID)
			if toolCallID != "" {
				if strings.TrimSpace(text) == "" {
					text = "tool_call_id=" + toolCallID
				} else {
					text = fmt.Sprintf("tool_call_id=%s %s", toolCallID, text)
				}
			}
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

// RenderRequestForLog 渲染完整的请求信息，包括 messages、tools 和 tool_choice
func RenderRequestForLog(request ChatCompletionRequest, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}

	var builder strings.Builder

	// 1. 渲染 tools 信息
	if len(request.Tools) > 0 {
		builder.WriteString(fmt.Sprintf("[Tools: %d 个]\n", len(request.Tools)))
		for i, tool := range request.Tools {
			if tool.Function != nil {
				builder.WriteString(fmt.Sprintf("  %d. %s", i+1, tool.Function.Name))
				if tool.Function.Description != "" {
					desc := tool.Function.Description
					if len(desc) > 50 {
						desc = desc[:50] + "..."
					}
					builder.WriteString(fmt.Sprintf(" - %s", desc))
				}
				builder.WriteString("\n")
			}
			if builder.Len() >= maxChars/3 {
				builder.WriteString(fmt.Sprintf("  ... 还有 %d 个工具\n", len(request.Tools)-i-1))
				break
			}
		}
	}

	// 2. 渲染 tool_choice
	if len(request.ToolChoice) > 0 {
		builder.WriteString(fmt.Sprintf("[tool_choice: %s]\n", string(request.ToolChoice)))
	}

	// 3. 渲染 messages
	builder.WriteString("\n[Messages]\n")
	messagesContent := RenderMessagesForLog(request.Messages, maxChars-builder.Len())
	builder.WriteString(messagesContent)

	output := builder.String()
	if len(output) <= maxChars {
		return output
	}
	return output[:maxChars]
}
