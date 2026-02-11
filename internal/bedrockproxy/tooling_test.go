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

	cfg, err := buildToolConfiguration(tools, json.RawMessage(`"required"`), false)
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

	cfgNone, err := buildToolConfiguration(tools, json.RawMessage(`"none"`), false)
	if err != nil {
		t.Fatalf("buildToolConfiguration(none) returned error: %v", err)
	}
	if cfgNone != nil {
		t.Fatalf("expected nil config when tool_choice is none")
	}

	cfgSpecific, err := buildToolConfiguration(tools, json.RawMessage(`{"type":"function","function":{"name":"search_docs"}}`), false)
	if err != nil {
		t.Fatalf("buildToolConfiguration(function choice) returned error: %v", err)
	}
	if cfgSpecific == nil {
		t.Fatalf("expected non-nil config for function-specific choice")
	}
	if _, ok := cfgSpecific.ToolChoice.(*brtypes.ToolChoiceMemberTool); !ok {
		t.Fatalf("expected ToolChoiceMemberTool, got %T", cfgSpecific.ToolChoice)
	}

	// Test forceToolUse with auto choice
	cfgForced, err := buildToolConfiguration(tools, json.RawMessage(`"auto"`), true)
	if err != nil {
		t.Fatalf("buildToolConfiguration(auto, forceToolUse=true) returned error: %v", err)
	}
	if cfgForced == nil {
		t.Fatalf("expected non-nil config for forced tool use")
	}
	if _, ok := cfgForced.ToolChoice.(*brtypes.ToolChoiceMemberAny); !ok {
		t.Fatalf("expected ToolChoiceMemberAny when forceToolUse=true, got %T", cfgForced.ToolChoice)
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

func TestBuildBedrockMessagesWithInlineToolResultContent(t *testing.T) {
	messages := []openai.ChatMessage{
		{
			Role:    "user",
			Content: json.RawMessage(`"read src/main.ts"`),
		},
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
		{
			Role: "user",
			Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"call_1","content":"package main"}
			]`),
		},
	}

	outMessages, _, err := BuildBedrockMessages(messages)
	if err != nil {
		t.Fatalf("BuildBedrockMessages returned error: %v", err)
	}
	if len(outMessages) != 3 {
		t.Fatalf("expected 3 output messages, got %d", len(outMessages))
	}

	toolResultMsg := outMessages[2]
	if toolResultMsg.Role != brtypes.ConversationRoleUser {
		t.Fatalf("expected inline tool result mapped to user role, got %v", toolResultMsg.Role)
	}
	if len(toolResultMsg.Content) != 1 {
		t.Fatalf("expected one tool result block, got %d", len(toolResultMsg.Content))
	}
	resultBlock, ok := toolResultMsg.Content[0].(*brtypes.ContentBlockMemberToolResult)
	if !ok {
		t.Fatalf("expected tool result block, got %T", toolResultMsg.Content[0])
	}
	if aws.ToString(resultBlock.Value.ToolUseId) != "call_1" {
		t.Fatalf("unexpected tool result id: %s", aws.ToString(resultBlock.Value.ToolUseId))
	}
	if len(resultBlock.Value.Content) != 1 {
		t.Fatalf("expected one inner tool result content block, got %d", len(resultBlock.Value.Content))
	}
	textBlock, ok := resultBlock.Value.Content[0].(*brtypes.ToolResultContentBlockMemberText)
	if !ok {
		t.Fatalf("expected text tool result content, got %T", resultBlock.Value.Content[0])
	}
	if textBlock.Value != "package main" {
		t.Fatalf("unexpected tool result text: %q", textBlock.Value)
	}
}

func TestBuildBedrockMessagesWithFunctionCallOutputContent(t *testing.T) {
	messages := []openai.ChatMessage{
		{
			Role: "user",
			Content: json.RawMessage(`[
				{"type":"function_call_output","call_id":"call_2","output":{"ok":true}}
			]`),
		},
	}

	outMessages, _, err := BuildBedrockMessages(messages)
	if err != nil {
		t.Fatalf("BuildBedrockMessages returned error: %v", err)
	}
	if len(outMessages) != 1 {
		t.Fatalf("expected one output message, got %d", len(outMessages))
	}
	if len(outMessages[0].Content) != 1 {
		t.Fatalf("expected one content block, got %d", len(outMessages[0].Content))
	}
	resultBlock, ok := outMessages[0].Content[0].(*brtypes.ContentBlockMemberToolResult)
	if !ok {
		t.Fatalf("expected tool result block, got %T", outMessages[0].Content[0])
	}
	if aws.ToString(resultBlock.Value.ToolUseId) != "call_2" {
		t.Fatalf("unexpected tool result id: %s", aws.ToString(resultBlock.Value.ToolUseId))
	}
	if len(resultBlock.Value.Content) != 1 {
		t.Fatalf("expected one inner tool result content block, got %d", len(resultBlock.Value.Content))
	}
	if _, ok := resultBlock.Value.Content[0].(*brtypes.ToolResultContentBlockMemberJson); !ok {
		t.Fatalf("expected JSON tool result content, got %T", resultBlock.Value.Content[0])
	}
}

func TestBuildBedrockMessagesWithToolResultTextArrayContent(t *testing.T) {
	messages := []openai.ChatMessage{
		{
			Role:    "assistant",
			Content: json.RawMessage(`null`),
			ToolCalls: []openai.ToolCall{
				{
					ID:   "call_text_1",
					Type: "function",
					Function: openai.ToolCallFunction{
						Name:      "Read",
						Arguments: `{"path":"src/components/home/Hero.tsx"}`,
					},
				},
			},
		},
		{
			Role: "user",
			Content: json.RawMessage(`[
				{
					"type": "tool_result",
					"tool_use_id": "call_text_1",
					"content": [
						{"type":"text","text":"line 1"},
						{"type":"output_text","text":"line 2"}
					]
				}
			]`),
		},
	}

	outMessages, _, err := BuildBedrockMessages(messages)
	if err != nil {
		t.Fatalf("BuildBedrockMessages returned error: %v", err)
	}
	if len(outMessages) != 2 {
		t.Fatalf("expected 2 output messages, got %d", len(outMessages))
	}

	toolResultMsg := outMessages[1]
	if toolResultMsg.Role != brtypes.ConversationRoleUser {
		t.Fatalf("expected tool result message role user, got %v", toolResultMsg.Role)
	}
	if len(toolResultMsg.Content) != 1 {
		t.Fatalf("expected one tool result block, got %d", len(toolResultMsg.Content))
	}
	resultBlock, ok := toolResultMsg.Content[0].(*brtypes.ContentBlockMemberToolResult)
	if !ok {
		t.Fatalf("expected tool result block, got %T", toolResultMsg.Content[0])
	}
	if len(resultBlock.Value.Content) != 2 {
		t.Fatalf("expected two inner content blocks, got %d", len(resultBlock.Value.Content))
	}
	firstText, ok := resultBlock.Value.Content[0].(*brtypes.ToolResultContentBlockMemberText)
	if !ok {
		t.Fatalf("expected first inner block text, got %T", resultBlock.Value.Content[0])
	}
	secondText, ok := resultBlock.Value.Content[1].(*brtypes.ToolResultContentBlockMemberText)
	if !ok {
		t.Fatalf("expected second inner block text, got %T", resultBlock.Value.Content[1])
	}
	if firstText.Value != "line 1" || secondText.Value != "line 2" {
		t.Fatalf("unexpected text content: first=%q second=%q", firstText.Value, secondText.Value)
	}
}

func TestBuildBedrockMessagesWithAssistantInlineToolUseBlocks(t *testing.T) {
	messages := []openai.ChatMessage{
		{
			Role:    "user",
			Content: json.RawMessage(`"please read files"`),
		},
		{
			Role: "assistant",
			Content: json.RawMessage(`[
				{"type":"text","text":"Let me inspect the project."},
				{"type":"tool_use","id":"tool_1","name":"Read","input":{"path":"README.md"}},
				{"type":"tool_use","id":"tool_2","name":"LS","input":{"target_directory":"."}}
			]`),
		},
		{
			Role: "user",
			Content: json.RawMessage(`[
				{"type":"tool_result","tool_use_id":"tool_1","content":[{"type":"text","text":"ok"}]},
				{"type":"tool_result","tool_use_id":"tool_2","content":[{"type":"text","text":"ok"}]}
			]`),
		},
	}

	outMessages, _, err := BuildBedrockMessages(messages)
	if err != nil {
		t.Fatalf("BuildBedrockMessages returned error: %v", err)
	}
	if len(outMessages) != 3 {
		t.Fatalf("expected 3 output messages, got %d", len(outMessages))
	}

	assistantMsg := outMessages[1]
	if assistantMsg.Role != brtypes.ConversationRoleAssistant {
		t.Fatalf("expected assistant role, got %v", assistantMsg.Role)
	}
	if len(assistantMsg.Content) != 3 {
		t.Fatalf("expected assistant content length 3 (text + 2 tool_use), got %d", len(assistantMsg.Content))
	}

	if _, ok := assistantMsg.Content[0].(*brtypes.ContentBlockMemberText); !ok {
		t.Fatalf("expected first block to be text, got %T", assistantMsg.Content[0])
	}
	toolUse1, ok := assistantMsg.Content[1].(*brtypes.ContentBlockMemberToolUse)
	if !ok {
		t.Fatalf("expected second block to be tool_use, got %T", assistantMsg.Content[1])
	}
	toolUse2, ok := assistantMsg.Content[2].(*brtypes.ContentBlockMemberToolUse)
	if !ok {
		t.Fatalf("expected third block to be tool_use, got %T", assistantMsg.Content[2])
	}
	if aws.ToString(toolUse1.Value.ToolUseId) != "tool_1" || aws.ToString(toolUse2.Value.ToolUseId) != "tool_2" {
		t.Fatalf("unexpected tool use ids: %q %q", aws.ToString(toolUse1.Value.ToolUseId), aws.ToString(toolUse2.Value.ToolUseId))
	}

	toolResultMsg := outMessages[2]
	if toolResultMsg.Role != brtypes.ConversationRoleUser {
		t.Fatalf("expected tool result message role user, got %v", toolResultMsg.Role)
	}
	if len(toolResultMsg.Content) != 2 {
		t.Fatalf("expected 2 tool_result blocks, got %d", len(toolResultMsg.Content))
	}
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
