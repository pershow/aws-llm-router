package openai

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ResponsesCreateRequest struct {
	Model             string          `json:"model"`
	Input             json.RawMessage `json:"input"`
	Tools             []ResponsesTool `json:"tools,omitempty"`
	ToolChoice        json.RawMessage `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool           `json:"parallel_tool_calls,omitempty"`
	Temperature       *float64        `json:"temperature,omitempty"`
	TopP              *float64        `json:"top_p,omitempty"`
	MaxOutputTokens   *int            `json:"max_output_tokens,omitempty"`
	Stream            bool            `json:"stream,omitempty"`
	User              string          `json:"user,omitempty"`
	Instructions      string          `json:"instructions,omitempty"`
}

type ResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
	Function    *ToolFunction   `json:"function,omitempty"`
	ServerLabel string          `json:"server_label,omitempty"`
}

type ResponsesCreateResponse struct {
	ID                string                `json:"id"`
	Object            string                `json:"object"`
	CreatedAt         int64                 `json:"created_at"`
	Status            string                `json:"status"`
	Model             string                `json:"model"`
	Output            []ResponsesOutputItem `json:"output"`
	Usage             ResponsesUsage        `json:"usage"`
	ParallelToolCalls bool                  `json:"parallel_tool_calls"`
	ToolChoice        json.RawMessage       `json:"tool_choice,omitempty"`
	OutputText        string                `json:"output_text,omitempty"`
	Error             any                   `json:"error"`
	IncompleteDetails any                   `json:"incomplete_details"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type ResponsesOutputItem struct {
	ID        string                   `json:"id,omitempty"`
	Type      string                   `json:"type"`
	Status    string                   `json:"status,omitempty"`
	Role      string                   `json:"role,omitempty"`
	Content   []ResponsesOutputContent `json:"content,omitempty"`
	CallID    string                   `json:"call_id,omitempty"`
	Name      string                   `json:"name,omitempty"`
	Arguments string                   `json:"arguments,omitempty"`
}

type ResponsesOutputContent struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Annotations []any  `json:"annotations,omitempty"`
}

type ResponsesFunctionCallState struct {
	OutputIndex int
	ItemID      string
	CallID      string
	Name        string
	Arguments   string
}

func ValidateResponsesCreateRequest(request ResponsesCreateRequest) error {
	if strings.TrimSpace(request.Model) == "" {
		return errors.New("model is required")
	}
	if strings.TrimSpace(string(request.Input)) == "" || strings.TrimSpace(string(request.Input)) == "null" {
		return errors.New("input is required")
	}
	return nil
}

func ResponsesRequestToChat(request ResponsesCreateRequest) (ChatCompletionRequest, error) {
	messages, err := ParseResponsesInputMessages(request.Input, request.Instructions)
	if err != nil {
		return ChatCompletionRequest{}, err
	}
	tools, err := normalizeResponsesTools(request.Tools)
	if err != nil {
		return ChatCompletionRequest{}, err
	}

	return ChatCompletionRequest{
		Model:             strings.TrimSpace(request.Model),
		Messages:          messages,
		Temperature:       request.Temperature,
		TopP:              request.TopP,
		MaxTokens:         request.MaxOutputTokens,
		Stream:            request.Stream,
		User:              strings.TrimSpace(request.User),
		Tools:             tools,
		ToolChoice:        request.ToolChoice,
		ParallelToolCalls: request.ParallelToolCalls,
	}, nil
}

func ParseResponsesInputMessages(input json.RawMessage, instructions string) ([]ChatMessage, error) {
	items := make([]ChatMessage, 0, 8)

	instructions = strings.TrimSpace(instructions)
	if instructions != "" {
		content, _ := json.Marshal(instructions)
		items = append(items, ChatMessage{
			Role:    "developer",
			Content: content,
		})
	}

	trimmed := strings.TrimSpace(string(input))
	if trimmed == "" || trimmed == "null" {
		if len(items) > 0 {
			return items, nil
		}
		return nil, errors.New("responses input is empty")
	}

	switch trimmed[0] {
	case '"':
		var text string
		if err := json.Unmarshal(input, &text); err != nil {
			return nil, fmt.Errorf("invalid string responses input: %w", err)
		}
		content, _ := json.Marshal(text)
		items = append(items, ChatMessage{
			Role:    "user",
			Content: content,
		})
	case '{':
		parsed, err := parseSingleResponsesInputItem(input)
		if err != nil {
			return nil, err
		}
		items = append(items, parsed...)
	case '[':
		var rawItems []json.RawMessage
		if err := json.Unmarshal(input, &rawItems); err != nil {
			return nil, fmt.Errorf("invalid responses input array: %w", err)
		}
		for _, rawItem := range rawItems {
			parsed, err := parseSingleResponsesInputItem(rawItem)
			if err != nil {
				return nil, err
			}
			items = append(items, parsed...)
		}
	default:
		return nil, errors.New("unsupported responses input format")
	}

	if len(items) == 0 {
		return nil, errors.New("responses input yielded no usable messages")
	}
	return items, nil
}

func parseSingleResponsesInputItem(raw json.RawMessage) ([]ChatMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	if trimmed[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			return nil, fmt.Errorf("invalid responses text item: %w", err)
		}
		content, _ := json.Marshal(text)
		return []ChatMessage{{
			Role:    "user",
			Content: content,
		}}, nil
	}
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("unsupported responses input item: %s", trimmed)
	}

	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, fmt.Errorf("invalid responses input item object: %w", err)
	}

	itemType := strings.ToLower(strings.TrimSpace(jsonString(item["type"])))
	if itemType == "" {
		itemType = "message"
	}

	switch itemType {
	case "message":
		role := strings.ToLower(strings.TrimSpace(jsonString(item["role"])))
		if role == "" {
			role = "user"
		}
		content := normalizeResponsesMessageContent(item["content"])
		return []ChatMessage{{
			Role:    role,
			Content: content,
		}}, nil

	case "function_call":
		callID := strings.TrimSpace(jsonString(item["call_id"]))
		name := strings.TrimSpace(jsonString(item["name"]))
		arguments := strings.TrimSpace(jsonStringOrRaw(item["arguments"]))
		if name == "" {
			return nil, errors.New("responses function_call item requires name")
		}
		if arguments == "" {
			arguments = "{}"
		}
		if callID == "" {
			callID = "call_generated"
		}
		return []ChatMessage{{
			Role:    "assistant",
			Content: json.RawMessage("null"),
			ToolCalls: []ToolCall{{
				ID:   callID,
				Type: "function",
				Function: ToolCallFunction{
					Name:      name,
					Arguments: arguments,
				},
			}},
		}}, nil

	case "function_call_output":
		callID := strings.TrimSpace(jsonString(item["call_id"]))
		if callID == "" {
			return nil, errors.New("responses function_call_output item requires call_id")
		}
		output := normalizeResponsesToolOutput(item["output"])
		return []ChatMessage{{
			Role:       "tool",
			ToolCallID: callID,
			Content:    output,
		}}, nil

	case "input_text", "output_text", "text":
		text := strings.TrimSpace(jsonString(item["text"]))
		content, _ := json.Marshal(text)
		return []ChatMessage{{
			Role:    "user",
			Content: content,
		}}, nil

	default:
		text := strings.TrimSpace(jsonString(item["text"]))
		if text == "" {
			return nil, nil
		}
		content, _ := json.Marshal(text)
		return []ChatMessage{{
			Role:    "user",
			Content: content,
		}}, nil
	}
}

func normalizeResponsesMessageContent(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(`""`)
	}

	switch trimmed[0] {
	case '"':
		return raw
	case '{':
		text := strings.TrimSpace(jsonStringFromObject(raw, "text"))
		if text != "" {
			blob, _ := json.Marshal(text)
			return blob
		}
		return json.RawMessage(`""`)
	case '[':
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return json.RawMessage(`""`)
		}

		parts := make([]string, 0, len(entries))
		for _, entry := range entries {
			entryText := strings.TrimSpace(extractResponseContentText(entry))
			if entryText == "" {
				continue
			}
			parts = append(parts, entryText)
		}
		blob, _ := json.Marshal(strings.Join(parts, "\n"))
		return blob
	default:
		blob, _ := json.Marshal(trimmed)
		return blob
	}
}

func normalizeResponsesToolOutput(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(`""`)
	}

	switch trimmed[0] {
	case '"', '{':
		return raw
	case '[':
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return raw
		}

		textParts := make([]string, 0, len(entries))
		allTextLike := true
		for _, entry := range entries {
			text := strings.TrimSpace(extractResponseContentText(entry))
			if text == "" {
				allTextLike = false
				break
			}
			textParts = append(textParts, text)
		}
		if allTextLike {
			blob, _ := json.Marshal(strings.Join(textParts, "\n"))
			return blob
		}
		return raw
	default:
		return raw
	}
}

func extractResponseContentText(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if trimmed[0] == '"' {
		var text string
		_ = json.Unmarshal(raw, &text)
		return text
	}
	if trimmed[0] != '{' {
		return ""
	}
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil {
		return ""
	}
	itemType := strings.ToLower(strings.TrimSpace(jsonString(item["type"])))
	switch itemType {
	case "", "input_text", "output_text", "text":
		return strings.TrimSpace(jsonString(item["text"]))
	default:
		return strings.TrimSpace(jsonString(item["text"]))
	}
}

func normalizeResponsesTools(tools []ResponsesTool) ([]Tool, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	out := make([]Tool, 0, len(tools))
	for _, item := range tools {
		toolType := strings.ToLower(strings.TrimSpace(item.Type))
		if toolType == "" {
			toolType = "function"
		}

		function := item.Function
		if function == nil {
			name := strings.TrimSpace(item.Name)
			if name == "" && strings.TrimSpace(item.ServerLabel) != "" {
				name = strings.TrimSpace(item.ServerLabel) + ".tool"
			}
			if name == "" {
				continue
			}
			function = &ToolFunction{
				Name:        name,
				Description: strings.TrimSpace(item.Description),
				Parameters:  item.Parameters,
				Strict:      item.Strict,
			}
		}

		if strings.TrimSpace(function.Name) == "" {
			return nil, errors.New("responses tool name is required")
		}

		out = append(out, Tool{
			Type:     "function",
			Function: function,
		})
	}

	return out, nil
}

func BuildResponsesOutputItems(requestID string, text string, toolCalls []ToolCall) []ResponsesOutputItem {
	items := make([]ResponsesOutputItem, 0, 1+len(toolCalls))
	if strings.TrimSpace(text) != "" {
		items = append(items, ResponsesOutputItem{
			ID:     "msg_" + requestID,
			Type:   "message",
			Status: "completed",
			Role:   "assistant",
			Content: []ResponsesOutputContent{{
				Type:        "output_text",
				Text:        text,
				Annotations: []any{},
			}},
		})
	}

	for index, toolCall := range toolCalls {
		callID := strings.TrimSpace(toolCall.ID)
		if callID == "" {
			callID = fmt.Sprintf("call_%d", index+1)
		}
		itemID := "fc_" + callID
		items = append(items, ResponsesOutputItem{
			ID:        itemID,
			Type:      "function_call",
			Status:    "completed",
			CallID:    callID,
			Name:      strings.TrimSpace(toolCall.Function.Name),
			Arguments: toolCall.Function.Arguments,
		})
	}
	return items
}

func BuildResponsesOutputText(items []ResponsesOutputItem) string {
	if len(items) == 0 {
		return ""
	}

	ordered := make([]ResponsesOutputItem, 0, len(items))
	ordered = append(ordered, items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := strings.TrimSpace(ordered[i].Type)
		right := strings.TrimSpace(ordered[j].Type)
		if left == right {
			return ordered[i].ID < ordered[j].ID
		}
		if left == "message" {
			return true
		}
		return false
	})

	var parts []string
	for _, item := range ordered {
		if item.Type != "message" {
			continue
		}
		for _, content := range item.Content {
			if strings.TrimSpace(content.Type) == "output_text" && strings.TrimSpace(content.Text) != "" {
				parts = append(parts, content.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func jsonString(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	return ""
}

func jsonStringOrRaw(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	if strings.HasPrefix(trimmed, "\"") {
		return jsonString(raw)
	}
	return trimmed
}

func jsonStringFromObject(raw json.RawMessage, key string) string {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return jsonString(value[key])
}
