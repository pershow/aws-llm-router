package bedrockproxy

import (
	"encoding/json"
	"testing"

	"aws-cursor-router/internal/openai"
)

func TestResolveModelDirect(t *testing.T) {
	service := NewService(nil, "anthropic.default", map[string]string{
		"gpt-4o": "anthropic.claude-3-7-sonnet-20250219-v1:0",
	}, 2048, false, false)

	requested, bedrockID, err := service.ResolveModel("gpt-4o")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if requested != "gpt-4o" {
		t.Fatalf("unexpected requested: %s", requested)
	}
	if bedrockID != "gpt-4o" {
		t.Fatalf("unexpected bedrock id: %s", bedrockID)
	}
}

func TestResolveModelDefault(t *testing.T) {
	service := NewService(nil, "anthropic.default", nil, 2048, false, false)

	requested, bedrockID, err := service.ResolveModel("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if requested != "default" || bedrockID != "anthropic.default" {
		t.Fatalf("unexpected model resolution: %s %s", requested, bedrockID)
	}
}

func TestHasToolResponsesDetectsAssistantToolCalls(t *testing.T) {
	messages := []openai.ChatMessage{
		{
			Role:    "assistant",
			Content: json.RawMessage(`null`),
			ToolCalls: []openai.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: openai.ToolCallFunction{
						Name:      "Read",
						Arguments: `{"path":"src/main.ts"}`,
					},
				},
			},
		},
	}

	if !hasToolResponses(messages) {
		t.Fatalf("expected hasToolResponses=true when assistant tool_calls exist")
	}
}

func TestHasToolResponsesDetectsInlineToolResults(t *testing.T) {
	messages := []openai.ChatMessage{
		{
			Role: "user",
			Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"call_1","content":"ok"}
			]`),
		},
	}

	if !hasToolResponses(messages) {
		t.Fatalf("expected hasToolResponses=true when inline tool_result exists")
	}
}
