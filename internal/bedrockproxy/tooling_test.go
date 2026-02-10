package bedrockproxy

import (
	"encoding/json"
	"reflect"
	"testing"

	"aws-cursor-router/internal/openai"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

func TestBuildBedrockMessagesWithToolUseAndToolResult(t *testing.T) {
	messages := []openai.ChatMessage{
		{
			Role:    "user",
			Content: json.RawMessage(`"what is the weather in SF?"`),
		},
		{
			Role:    "assistant",
			Content: json.RawMessage(`null`),
			ToolCalls: []openai.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: openai.ToolCallFunction{
						Name:      "get_weather",
						Arguments: `{"city":"San Francisco"}`,
					},
				},
			},
		},
		{
			Role:       "tool",
			ToolCallID: "call_1",
			Content:    json.RawMessage(`{"temp_c": 21, "condition": "sunny"}`),
		},
	}

	outMessages, outSystem, err := BuildBedrockMessages(messages)
	if err != nil {
		t.Fatalf("BuildBedrockMessages returned error: %v", err)
	}
	if len(outSystem) != 0 {
		t.Fatalf("expected no system messages, got %d", len(outSystem))
	}
	if len(outMessages) != 3 {
		t.Fatalf("expected 3 output messages, got %d", len(outMessages))
	}

	assistantMsg := outMessages[1]
	if assistantMsg.Role != brtypes.ConversationRoleAssistant {
		t.Fatalf("expected assistant role, got %v", assistantMsg.Role)
	}
	if len(assistantMsg.Content) != 1 {
		t.Fatalf("expected assistant content length 1, got %d", len(assistantMsg.Content))
	}
	toolUse, ok := assistantMsg.Content[0].(*brtypes.ContentBlockMemberToolUse)
	if !ok {
		t.Fatalf("expected tool use content block, got %T", assistantMsg.Content[0])
	}
	if aws.ToString(toolUse.Value.Name) != "get_weather" {
		t.Fatalf("unexpected tool name: %s", aws.ToString(toolUse.Value.Name))
	}
	if aws.ToString(toolUse.Value.ToolUseId) != "call_1" {
		t.Fatalf("unexpected tool use id: %s", aws.ToString(toolUse.Value.ToolUseId))
	}
	assertJSONEqual(t, documentToJSONString(toolUse.Value.Input), `{"city":"San Francisco"}`)

	toolResultMsg := outMessages[2]
	if toolResultMsg.Role != brtypes.ConversationRoleUser {
		t.Fatalf("expected tool result as user role, got %v", toolResultMsg.Role)
	}
	if len(toolResultMsg.Content) != 1 {
		t.Fatalf("expected tool result content length 1, got %d", len(toolResultMsg.Content))
	}
	resultBlock, ok := toolResultMsg.Content[0].(*brtypes.ContentBlockMemberToolResult)
	if !ok {
		t.Fatalf("expected tool result content block, got %T", toolResultMsg.Content[0])
	}
	if aws.ToString(resultBlock.Value.ToolUseId) != "call_1" {
		t.Fatalf("unexpected tool result id: %s", aws.ToString(resultBlock.Value.ToolUseId))
	}
	if len(resultBlock.Value.Content) != 1 {
		t.Fatalf("expected tool result inner content length 1, got %d", len(resultBlock.Value.Content))
	}
	_, ok = resultBlock.Value.Content[0].(*brtypes.ToolResultContentBlockMemberJson)
	if !ok {
		t.Fatalf("expected JSON tool result content, got %T", resultBlock.Value.Content[0])
	}
}

func TestBuildToolConfiguration(t *testing.T) {
	tools := []openai.Tool{
		{
			Type: "function",
			Function: &openai.ToolFunction{
				Name:        "search_docs",
				Description: "Search docs",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
	}

	cfg, err := buildToolConfiguration(tools, json.RawMessage(`"required"`))
	if err != nil {
		t.Fatalf("buildToolConfiguration(required) returned error: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil config for required choice")
	}
	if len(cfg.Tools) != 1 {
		t.Fatalf("expected exactly one tool, got %d", len(cfg.Tools))
	}
	if _, ok := cfg.ToolChoice.(*brtypes.ToolChoiceMemberAny); !ok {
		t.Fatalf("expected ToolChoiceMemberAny, got %T", cfg.ToolChoice)
	}

	cfgNone, err := buildToolConfiguration(tools, json.RawMessage(`"none"`))
	if err != nil {
		t.Fatalf("buildToolConfiguration(none) returned error: %v", err)
	}
	if cfgNone != nil {
		t.Fatalf("expected nil config when tool_choice is none")
	}

	cfgSpecific, err := buildToolConfiguration(tools, json.RawMessage(`{"type":"function","function":{"name":"search_docs"}}`))
	if err != nil {
		t.Fatalf("buildToolConfiguration(function choice) returned error: %v", err)
	}
	if cfgSpecific == nil {
		t.Fatalf("expected non-nil config for function-specific choice")
	}
	if _, ok := cfgSpecific.ToolChoice.(*brtypes.ToolChoiceMemberTool); !ok {
		t.Fatalf("expected ToolChoiceMemberTool, got %T", cfgSpecific.ToolChoice)
	}
}

func TestExtractOutputPayloadWithToolCalls(t *testing.T) {
	output := &brtypes.ConverseOutputMemberMessage{
		Value: brtypes.Message{
			Role: brtypes.ConversationRoleAssistant,
			Content: []brtypes.ContentBlock{
				&brtypes.ContentBlockMemberText{Value: "Let me check."},
				&brtypes.ContentBlockMemberToolUse{
					Value: brtypes.ToolUseBlock{
						Name:      aws.String("search_docs"),
						ToolUseId: aws.String("call_42"),
						Input:     document.NewLazyDocument(map[string]any{"query": "mcp"}),
					},
				},
			},
		},
	}

	payload := extractOutputPayload(output)
	if payload.Text != "Let me check." {
		t.Fatalf("unexpected text payload: %q", payload.Text)
	}
	if len(payload.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %d", len(payload.ToolCalls))
	}
	toolCall := payload.ToolCalls[0]
	if toolCall.ID != "call_42" {
		t.Fatalf("unexpected tool call id: %s", toolCall.ID)
	}
	if toolCall.Type != "function" {
		t.Fatalf("unexpected tool call type: %s", toolCall.Type)
	}
	if toolCall.Function.Name != "search_docs" {
		t.Fatalf("unexpected tool function name: %s", toolCall.Function.Name)
	}
	assertJSONEqual(t, toolCall.Function.Arguments, `{"query":"mcp"}`)
}

func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()

	var gotValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("failed to unmarshal got JSON %q: %v", got, err)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("failed to unmarshal want JSON %q: %v", want, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("unexpected JSON payload:\n got: %s\nwant: %s", got, want)
	}
}
