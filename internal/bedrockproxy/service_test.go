package bedrockproxy

import (
	"encoding/json"
	"testing"

	"aws-cursor-router/internal/openai"
)

func TestResolveModelDirect(t *testing.T) {
	service := NewService(nil, "anthropic.default", map[string]string{
		"gpt-4o": "anthropic.claude-3-7-sonnet-20250219-v1:0",
	}, 2048, 8192, false, false)

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
	service := NewService(nil, "anthropic.default", nil, 2048, 8192, false, false)

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

func TestBuildInferenceConfigRaisesMaxTokensForTools(t *testing.T) {
	maxTokens := 4096
	request := openai.ChatCompletionRequest{
		MaxTokens: &maxTokens,
		Tools: []openai.Tool{
			{
				Type: "function",
				Function: &openai.ToolFunction{
					Name: "Write",
				},
			},
		},
	}

	cfg, raised, original, effective := buildInferenceConfig(request, 0, 8192)
	if cfg == nil || cfg.MaxTokens == nil {
		t.Fatalf("expected inference config with max tokens")
	}
	if !raised {
		t.Fatalf("expected max tokens to be raised")
	}
	if original != 4096 {
		t.Fatalf("unexpected original max tokens: %d", original)
	}
	if effective != 8192 {
		t.Fatalf("unexpected effective max tokens: %d", effective)
	}
	if *cfg.MaxTokens != 8192 {
		t.Fatalf("unexpected inference max tokens: %d", *cfg.MaxTokens)
	}
}

func TestBuildInferenceConfigKeepsRequestMaxTokensWhenAlreadyHigher(t *testing.T) {
	maxTokens := 12000
	request := openai.ChatCompletionRequest{
		MaxTokens: &maxTokens,
		Tools: []openai.Tool{
			{
				Type: "function",
				Function: &openai.ToolFunction{
					Name: "Write",
				},
			},
		},
	}

	cfg, raised, original, effective := buildInferenceConfig(request, 0, 8192)
	if cfg == nil || cfg.MaxTokens == nil {
		t.Fatalf("expected inference config with max tokens")
	}
	if raised {
		t.Fatalf("did not expect max tokens to be raised")
	}
	if original != 12000 || effective != 12000 {
		t.Fatalf("unexpected max tokens original=%d effective=%d", original, effective)
	}
	if *cfg.MaxTokens != 12000 {
		t.Fatalf("unexpected inference max tokens: %d", *cfg.MaxTokens)
	}
}

func TestBuildInferenceConfigDoesNotEnforceToolMinimumWithoutTools(t *testing.T) {
	request := openai.ChatCompletionRequest{}

	cfg, raised, original, effective := buildInferenceConfig(request, 2048, 8192)
	if cfg == nil || cfg.MaxTokens == nil {
		t.Fatalf("expected inference config with default max tokens")
	}
	if raised {
		t.Fatalf("did not expect max tokens to be raised when no tools are present")
	}
	if original != 0 || effective != 2048 {
		t.Fatalf("unexpected max tokens original=%d effective=%d", original, effective)
	}
	if *cfg.MaxTokens != 2048 {
		t.Fatalf("unexpected inference max tokens: %d", *cfg.MaxTokens)
	}
}
