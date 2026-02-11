package openai

import (
	"encoding/json"
	"testing"
)

func TestFixMissingToolResponsesRecognizesInlineToolResult(t *testing.T) {
	messages := []ChatMessage{
		{
			Role:    "user",
			Content: json.RawMessage(`"read file"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`null`),
			ToolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: ToolCallFunction{
						Name:      "Read",
						Arguments: `{"path":"src/main.ts"}`,
					},
				},
			},
		},
		{
			Role: "user",
			Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"call_1","content":"file body"}
			]`),
		},
	}

	fixed := FixMissingToolResponses(messages)
	if len(fixed) != len(messages) {
		t.Fatalf("expected no injected tool response, got len=%d want=%d", len(fixed), len(messages))
	}
	if fixed[2].Role != "user" {
		t.Fatalf("expected third message role=user, got %q", fixed[2].Role)
	}
}

func TestFixMissingToolResponsesInsertsOnlyMissingIDs(t *testing.T) {
	messages := []ChatMessage{
		{
			Role:    "assistant",
			Content: json.RawMessage(`null`),
			ToolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: ToolCallFunction{
						Name:      "Read",
						Arguments: `{"path":"a.ts"}`,
					},
				},
				{
					ID:   "call_2",
					Type: "function",
					Function: ToolCallFunction{
						Name:      "Read",
						Arguments: `{"path":"b.ts"}`,
					},
				},
			},
		},
		{
			Role:       "tool",
			ToolCallID: "call_1",
			Content:    json.RawMessage(`"ok"`),
		},
	}

	fixed := FixMissingToolResponses(messages)
	if len(fixed) != 3 {
		t.Fatalf("expected one injected missing tool response, got %d messages", len(fixed))
	}
	if fixed[1].Role != "tool" {
		t.Fatalf("expected injected tool response at index 1, got role=%q", fixed[1].Role)
	}
	if fixed[1].ToolCallID != "call_2" {
		t.Fatalf("expected injected tool_call_id=call_2, got %q", fixed[1].ToolCallID)
	}
}
