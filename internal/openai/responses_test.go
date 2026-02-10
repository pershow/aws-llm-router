package openai

import (
	"encoding/json"
	"testing"
)

func TestResponsesRequestToChat(t *testing.T) {
	maxOutput := 256
	temperature := 0.2
	topP := 0.9
	parallel := true

	request := ResponsesCreateRequest{
		Model:             "anthropic.claude-3-5-sonnet-20240620-v1:0",
		Input:             json.RawMessage(`"hello"`),
		Instructions:      "be concise",
		MaxOutputTokens:   &maxOutput,
		Temperature:       &temperature,
		TopP:              &topP,
		Stream:            true,
		ToolChoice:        json.RawMessage(`"auto"`),
		ParallelToolCalls: &parallel,
		Tools: []ResponsesTool{
			{
				Type:        "function",
				Name:        "search_docs",
				Description: "Search docs",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
		},
	}

	chatRequest, err := ResponsesRequestToChat(request)
	if err != nil {
		t.Fatalf("ResponsesRequestToChat returned error: %v", err)
	}

	if chatRequest.Model != request.Model {
		t.Fatalf("unexpected model: %q", chatRequest.Model)
	}
	if chatRequest.MaxTokens == nil || *chatRequest.MaxTokens != maxOutput {
		t.Fatalf("unexpected max tokens: %#v", chatRequest.MaxTokens)
	}
	if chatRequest.Temperature == nil || *chatRequest.Temperature != temperature {
		t.Fatalf("unexpected temperature: %#v", chatRequest.Temperature)
	}
	if chatRequest.TopP == nil || *chatRequest.TopP != topP {
		t.Fatalf("unexpected top_p: %#v", chatRequest.TopP)
	}
	if !chatRequest.Stream {
		t.Fatalf("expected stream=true")
	}
	if len(chatRequest.Messages) != 2 {
		t.Fatalf("expected 2 messages (developer + user), got %d", len(chatRequest.Messages))
	}
	if chatRequest.Messages[0].Role != "developer" {
		t.Fatalf("unexpected first role: %q", chatRequest.Messages[0].Role)
	}
	if chatRequest.Messages[1].Role != "user" {
		t.Fatalf("unexpected second role: %q", chatRequest.Messages[1].Role)
	}
	if len(chatRequest.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(chatRequest.Tools))
	}
	if chatRequest.Tools[0].Function == nil || chatRequest.Tools[0].Function.Name != "search_docs" {
		t.Fatalf("unexpected converted tool: %#v", chatRequest.Tools[0])
	}
}

func TestParseResponsesInputMessagesWithFunctionCallItems(t *testing.T) {
	input := json.RawMessage(`[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"What is the weather?"}]},
		{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"San Francisco\"}"},
		{"type":"function_call_output","call_id":"call_1","output":{"temp_c":21}}
	]`)

	messages, err := ParseResponsesInputMessages(input, "")
	if err != nil {
		t.Fatalf("ParseResponsesInputMessages returned error: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	userText, err := DecodeContentAsText(messages[0].Content)
	if err != nil {
		t.Fatalf("DecodeContentAsText returned error: %v", err)
	}
	if messages[0].Role != "user" || userText != "What is the weather?" {
		t.Fatalf("unexpected user message: role=%q content=%q", messages[0].Role, userText)
	}

	if messages[1].Role != "assistant" {
		t.Fatalf("unexpected assistant role: %q", messages[1].Role)
	}
	if len(messages[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(messages[1].ToolCalls))
	}
	if messages[1].ToolCalls[0].ID != "call_1" {
		t.Fatalf("unexpected tool call id: %q", messages[1].ToolCalls[0].ID)
	}
	if messages[1].ToolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("unexpected tool call name: %q", messages[1].ToolCalls[0].Function.Name)
	}

	if messages[2].Role != "tool" {
		t.Fatalf("unexpected tool role: %q", messages[2].Role)
	}
	if messages[2].ToolCallID != "call_1" {
		t.Fatalf("unexpected tool_call_id: %q", messages[2].ToolCallID)
	}
}

func TestBuildResponsesOutputItemsAndText(t *testing.T) {
	toolCalls := []ToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: ToolCallFunction{
				Name:      "lookup",
				Arguments: `{"query":"mcp"}`,
			},
		},
	}

	items := BuildResponsesOutputItems("req_123", "Done.", toolCalls)
	if len(items) != 2 {
		t.Fatalf("expected 2 output items, got %d", len(items))
	}
	if items[0].Type != "message" || items[0].Role != "assistant" {
		t.Fatalf("unexpected message item: %#v", items[0])
	}
	if items[1].Type != "function_call" || items[1].CallID != "call_1" {
		t.Fatalf("unexpected function_call item: %#v", items[1])
	}

	outputText := BuildResponsesOutputText(items)
	if outputText != "Done." {
		t.Fatalf("unexpected output_text: %q", outputText)
	}
}

func TestValidateResponsesCreateRequest(t *testing.T) {
	if err := ValidateResponsesCreateRequest(ResponsesCreateRequest{
		Input: json.RawMessage(`"hello"`),
	}); err == nil {
		t.Fatalf("expected error when model is empty")
	}

	if err := ValidateResponsesCreateRequest(ResponsesCreateRequest{
		Model: "m",
		Input: json.RawMessage(`null`),
	}); err == nil {
		t.Fatalf("expected error when input is null")
	}
}
