package bedrockproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"aws-cursor-router/internal/openai"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/document"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	smithydocument "github.com/aws/smithy-go/document"
)

type ConverseAPI interface {
	Converse(ctx context.Context, params *bedrockruntime.ConverseInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseOutput, error)
	ConverseStream(ctx context.Context, params *bedrockruntime.ConverseStreamInput, optFns ...func(*bedrockruntime.Options)) (*bedrockruntime.ConverseStreamOutput, error)
}

type Service struct {
	client                ConverseAPI
	mu                    sync.RWMutex
	defaultModelID        string
	defaultMaxOutputToken int32
	forceToolUse          bool // 当请求包含 tools 时，强制模型调用工具
}

type ChatResult struct {
	Text         string
	ToolCalls    []openai.ToolCall
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	LatencyMs    int64
	FinishReason string
}

type StreamDelta struct {
	Role         string
	Text         string
	ToolCalls    []openai.ChatChunkToolCall
	FinishReason string
}

func NewService(
	client ConverseAPI,
	defaultModelID string,
	modelRouter map[string]string,
	defaultMaxOutputToken int32,
	forceToolUse bool,
) *Service {
	_ = modelRouter

	return &Service{
		client:                client,
		defaultModelID:        strings.TrimSpace(defaultModelID),
		defaultMaxOutputToken: defaultMaxOutputToken,
		forceToolUse:          forceToolUse,
	}
}

func (s *Service) ResolveModel(requestModel string) (string, string, error) {
	requestModel = strings.TrimSpace(requestModel)
	s.mu.RLock()
	defer s.mu.RUnlock()

	if requestModel == "" {
		if s.defaultModelID == "" {
			return "", "", errors.New("model is required")
		}
		return "default", s.defaultModelID, nil
	}

	return requestModel, requestModel, nil
}

func (s *Service) ReplaceModelRouter(modelRouter map[string]string) {
	_ = modelRouter
}

func (s *Service) ReplaceClient(client ConverseAPI) {
	s.mu.Lock()
	s.client = client
	s.mu.Unlock()
}

func (s *Service) SetDefaultModelID(defaultModelID string) {
	s.mu.Lock()
	s.defaultModelID = strings.TrimSpace(defaultModelID)
	s.mu.Unlock()
}

func (s *Service) HasClient() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client != nil
}

func (s *Service) UpsertModelMapping(alias, bedrockModelID string) error {
	_ = alias
	_ = bedrockModelID
	return nil
}

func (s *Service) DeleteModelMapping(alias string) bool {
	_ = alias
	return false
}

func (s *Service) ListModelMappings() map[string]string {
	return map[string]string{}
}

func (s *Service) ListModelAliases() []string {
	s.mu.RLock()
	aliases := make([]string, 0, 1)
	if s.defaultModelID != "" {
		aliases = append(aliases, "default")
	}
	s.mu.RUnlock()
	sort.Strings(aliases)
	return aliases
}

func (s *Service) Converse(ctx context.Context, request openai.ChatCompletionRequest, bedrockModelID string) (ChatResult, error) {
	// Fix messages: ensure tool_call IDs and fix missing tool responses
	request.Messages = openai.EnsureToolCallIDs(request.Messages)
	request.Messages = openai.FixMissingToolResponses(request.Messages)

	messages, system, err := BuildBedrockMessages(request.Messages)
	if err != nil {
		return ChatResult{}, err
	}

	s.mu.RLock()
	forceToolUse := s.forceToolUse
	s.mu.RUnlock()

	toolConfig, err := buildToolConfiguration(request.Tools, request.ToolChoice, forceToolUse)
	if err != nil {
		return ChatResult{}, err
	}

	s.mu.RLock()
	client := s.client
	defaultMaxOutputToken := s.defaultMaxOutputToken
	s.mu.RUnlock()

	if client == nil {
		return ChatResult{}, errors.New("bedrock client is not configured")
	}

	inferenceConfig := buildInferenceConfig(request, defaultMaxOutputToken)

	output, err := client.Converse(ctx, &bedrockruntime.ConverseInput{
		ModelId:         aws.String(bedrockModelID),
		Messages:        messages,
		System:          system,
		InferenceConfig: inferenceConfig,
		ToolConfig:      toolConfig,
	})
	if err != nil {
		return ChatResult{}, err
	}

	payload := extractOutputPayload(output.Output)
	result := ChatResult{
		Text:         payload.Text,
		ToolCalls:    payload.ToolCalls,
		FinishReason: mapStopReason(output.StopReason),
	}

	if output.Usage != nil {
		result.InputTokens = int(ptrInt32(output.Usage.InputTokens))
		result.OutputTokens = int(ptrInt32(output.Usage.OutputTokens))
		result.TotalTokens = int(ptrInt32(output.Usage.TotalTokens))
	}
	if output.Metrics != nil {
		result.LatencyMs = ptrInt64(output.Metrics.LatencyMs)
	}

	return result, nil
}

func (s *Service) ConverseStream(
	ctx context.Context,
	request openai.ChatCompletionRequest,
	bedrockModelID string,
	onDelta func(delta StreamDelta) error,
) (ChatResult, error) {
	// Fix messages: ensure tool_call IDs and fix missing tool responses
	request.Messages = openai.EnsureToolCallIDs(request.Messages)
	request.Messages = openai.FixMissingToolResponses(request.Messages)

	messages, system, err := BuildBedrockMessages(request.Messages)
	if err != nil {
		return ChatResult{}, err
	}

	s.mu.RLock()
	forceToolUse := s.forceToolUse
	s.mu.RUnlock()

	toolConfig, err := buildToolConfiguration(request.Tools, request.ToolChoice, forceToolUse)
	if err != nil {
		return ChatResult{}, err
	}

	// 调试日志：打印工具配置
	if toolConfig != nil {
		toolChoiceType := "nil"
		if toolConfig.ToolChoice != nil {
			switch toolConfig.ToolChoice.(type) {
			case *brtypes.ToolChoiceMemberAuto:
				toolChoiceType = "auto"
			case *brtypes.ToolChoiceMemberAny:
				toolChoiceType = "any (required)"
			case *brtypes.ToolChoiceMemberTool:
				toolChoiceType = "specific tool"
			default:
				toolChoiceType = fmt.Sprintf("%T", toolConfig.ToolChoice)
			}
		}
		fmt.Printf("[DEBUG] ConverseStream: forceToolUse=%v, tools=%d, toolChoice=%s\n",
			forceToolUse, len(toolConfig.Tools), toolChoiceType)
	}

	s.mu.RLock()
	client := s.client
	defaultMaxOutputToken := s.defaultMaxOutputToken
	s.mu.RUnlock()

	if client == nil {
		return ChatResult{}, errors.New("bedrock client is not configured")
	}

	inferenceConfig := buildInferenceConfig(request, defaultMaxOutputToken)

	output, err := client.ConverseStream(ctx, &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(bedrockModelID),
		Messages:        messages,
		System:          system,
		InferenceConfig: inferenceConfig,
		ToolConfig:      toolConfig,
	})
	if err != nil {
		return ChatResult{}, err
	}
	stream := output.GetStream()
	defer func() { _ = stream.Close() }()

	result := ChatResult{FinishReason: "stop"}
	var textBuilder strings.Builder
	roleSent := false
	toolCalls := make([]openai.ToolCall, 0, 2)
	toolCallIndexByContentBlock := make(map[int]int)

	for event := range stream.Events() {
		switch value := event.(type) {
		case *brtypes.ConverseStreamOutputMemberMessageStart:
			if !roleSent {
				roleSent = true
				if err := onDelta(StreamDelta{Role: string(value.Value.Role)}); err != nil {
					return ChatResult{}, err
				}
			}
		case *brtypes.ConverseStreamOutputMemberContentBlockStart:
			blockIndex := int(ptrInt32(value.Value.ContentBlockIndex))
			toolStart, ok := value.Value.Start.(*brtypes.ContentBlockStartMemberToolUse)
			if !ok {
				continue
			}
			if !roleSent {
				roleSent = true
				if err := onDelta(StreamDelta{Role: "assistant"}); err != nil {
					return ChatResult{}, err
				}
			}

			toolCallIndex := len(toolCalls)
			toolCallID := strings.TrimSpace(aws.ToString(toolStart.Value.ToolUseId))
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("toolcall_%d", toolCallIndex+1)
			}
			toolName := strings.TrimSpace(aws.ToString(toolStart.Value.Name))
			if toolName == "" {
				toolName = "unknown_tool"
			}

			toolCalls = append(toolCalls, openai.ToolCall{
				ID:   toolCallID,
				Type: "function",
				Function: openai.ToolCallFunction{
					Name: toolName,
				},
			})
			toolCallIndexByContentBlock[blockIndex] = toolCallIndex

			if err := onDelta(StreamDelta{
				ToolCalls: []openai.ChatChunkToolCall{{
					Index: toolCallIndex,
					ID:    toolCallID,
					Type:  "function",
					Function: &openai.ToolCallFunction{
						Name: toolName,
					},
				}},
			}); err != nil {
				return ChatResult{}, err
			}
		case *brtypes.ConverseStreamOutputMemberContentBlockDelta:
			blockIndex := int(ptrInt32(value.Value.ContentBlockIndex))
			switch delta := value.Value.Delta.(type) {
			case *brtypes.ContentBlockDeltaMemberText:
				if !roleSent {
					roleSent = true
					if err := onDelta(StreamDelta{Role: "assistant"}); err != nil {
						return ChatResult{}, err
					}
				}
				if delta.Value == "" {
					continue
				}
				textBuilder.WriteString(delta.Value)
				if err := onDelta(StreamDelta{Text: delta.Value}); err != nil {
					return ChatResult{}, err
				}
			case *brtypes.ContentBlockDeltaMemberToolUse:
				toolCallIndex, exists := toolCallIndexByContentBlock[blockIndex]
				if !exists {
					toolCallIndex = len(toolCalls)
					toolCalls = append(toolCalls, openai.ToolCall{
						ID:       fmt.Sprintf("toolcall_%d", toolCallIndex+1),
						Type:     "function",
						Function: openai.ToolCallFunction{},
					})
					toolCallIndexByContentBlock[blockIndex] = toolCallIndex
				}
				if delta.Value.Input == nil || *delta.Value.Input == "" {
					continue
				}
				toolCalls[toolCallIndex].Function.Arguments += *delta.Value.Input
				if err := onDelta(StreamDelta{
					ToolCalls: []openai.ChatChunkToolCall{{
						Index: toolCallIndex,
						Function: &openai.ToolCallFunction{
							Arguments: *delta.Value.Input,
						},
					}},
				}); err != nil {
					return ChatResult{}, err
				}
			}
		case *brtypes.ConverseStreamOutputMemberMessageStop:
			result.FinishReason = mapStopReason(value.Value.StopReason)
		case *brtypes.ConverseStreamOutputMemberMetadata:
			if value.Value.Usage != nil {
				result.InputTokens = int(ptrInt32(value.Value.Usage.InputTokens))
				result.OutputTokens = int(ptrInt32(value.Value.Usage.OutputTokens))
				result.TotalTokens = int(ptrInt32(value.Value.Usage.TotalTokens))
			}
			if value.Value.Metrics != nil {
				result.LatencyMs = ptrInt64(value.Value.Metrics.LatencyMs)
			}
		}
	}

	if err := stream.Err(); err != nil {
		return ChatResult{}, err
	}

	result.Text = textBuilder.String()
	result.ToolCalls = toolCalls
	return result, nil
}

func BuildBedrockMessages(messages []openai.ChatMessage) ([]brtypes.Message, []brtypes.SystemContentBlock, error) {
	outMessages := make([]brtypes.Message, 0, len(messages))
	outSystem := make([]brtypes.SystemContentBlock, 0, 2)

	for index, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))

		switch role {
		case "system", "developer":
			text, err := openai.DecodeContentAsText(message.Content)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid %s message content at index %d: %w", role, index, err)
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			outSystem = append(outSystem, &brtypes.SystemContentBlockMemberText{Value: text})

		case "assistant":
			blocks, err := buildAssistantContentBlocks(message)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid assistant message at index %d: %w", index, err)
			}
			if len(blocks) == 0 {
				continue
			}
			outMessages = append(outMessages, brtypes.Message{
				Role:    brtypes.ConversationRoleAssistant,
				Content: blocks,
			})

		case "tool":
			toolResult, err := buildToolResultContentBlock(message)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid tool message at index %d: %w", index, err)
			}
			outMessages = append(outMessages, brtypes.Message{
				Role:    brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{toolResult},
			})

		case "", "user", "function":
			text, err := openai.DecodeContentAsText(message.Content)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid user message content at index %d: %w", index, err)
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			outMessages = append(outMessages, brtypes.Message{
				Role:    brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberText{Value: text}},
			})

		default:
			// Keep compatibility by ignoring unknown roles instead of failing hard.
		}
	}

	if len(outMessages) == 0 {
		return nil, nil, errors.New("at least one non-system message is required")
	}
	return outMessages, outSystem, nil
}

func buildAssistantContentBlocks(message openai.ChatMessage) ([]brtypes.ContentBlock, error) {
	blocks := make([]brtypes.ContentBlock, 0, 1+len(message.ToolCalls))

	text, err := openai.DecodeContentAsText(message.Content)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, &brtypes.ContentBlockMemberText{Value: text})
	}

	toolUseBlocks, err := buildToolUseBlocks(message.ToolCalls)
	if err != nil {
		return nil, err
	}
	blocks = append(blocks, toolUseBlocks...)
	return blocks, nil
}

func buildToolUseBlocks(toolCalls []openai.ToolCall) ([]brtypes.ContentBlock, error) {
	if len(toolCalls) == 0 {
		return nil, nil
	}

	blocks := make([]brtypes.ContentBlock, 0, len(toolCalls))
	for index, toolCall := range toolCalls {
		toolType := strings.ToLower(strings.TrimSpace(toolCall.Type))
		if toolType == "" {
			toolType = "function"
		}
		if toolType != "function" {
			return nil, fmt.Errorf("unsupported tool call type: %s", toolType)
		}

		toolName := strings.TrimSpace(toolCall.Function.Name)
		if toolName == "" {
			return nil, errors.New("tool call function.name is required")
		}

		toolCallID := strings.TrimSpace(toolCall.ID)
		if toolCallID == "" {
			toolCallID = fmt.Sprintf("toolcall_%d", index+1)
		}

		argsRaw := strings.TrimSpace(toolCall.Function.Arguments)
		if argsRaw == "" {
			argsRaw = "{}"
		}
		var args any
		if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
			return nil, fmt.Errorf("invalid JSON in tool call arguments for %q: %w", toolName, err)
		}

		blocks = append(blocks, &brtypes.ContentBlockMemberToolUse{
			Value: brtypes.ToolUseBlock{
				Name:      aws.String(toolName),
				ToolUseId: aws.String(toolCallID),
				Input:     document.NewLazyDocument(args),
			},
		})
	}

	return blocks, nil
}

func buildToolResultContentBlock(message openai.ChatMessage) (brtypes.ContentBlock, error) {
	toolUseID := strings.TrimSpace(message.ToolCallID)
	if toolUseID == "" {
		toolUseID = strings.TrimSpace(message.Name)
	}
	if toolUseID == "" {
		return nil, errors.New("tool message requires tool_call_id")
	}

	resultContent, err := parseToolResultContent(message.Content)
	if err != nil {
		return nil, err
	}
	if len(resultContent) == 0 {
		resultContent = []brtypes.ToolResultContentBlock{
			&brtypes.ToolResultContentBlockMemberText{Value: ""},
		}
	}

	return &brtypes.ContentBlockMemberToolResult{
		Value: brtypes.ToolResultBlock{
			ToolUseId: aws.String(toolUseID),
			Content:   resultContent,
		},
	}, nil
}

func parseToolResultContent(raw json.RawMessage) ([]brtypes.ToolResultContentBlock, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []brtypes.ToolResultContentBlock{
			&brtypes.ToolResultContentBlockMemberText{Value: trimmed},
		}, nil
	}

	switch value := payload.(type) {
	case string:
		return []brtypes.ToolResultContentBlock{
			&brtypes.ToolResultContentBlockMemberText{Value: value},
		}, nil
	default:
		return []brtypes.ToolResultContentBlock{
			&brtypes.ToolResultContentBlockMemberJson{Value: document.NewLazyDocument(payload)},
		}, nil
	}
}

func buildToolConfiguration(tools []openai.Tool, rawToolChoice json.RawMessage, forceToolUse bool) (*brtypes.ToolConfiguration, error) {
	bedrockTools := make([]brtypes.Tool, 0, len(tools))
	for _, item := range tools {
		toolType := strings.ToLower(strings.TrimSpace(item.Type))
		if toolType == "" {
			toolType = "function"
		}
		if toolType != "function" || item.Function == nil {
			continue
		}

		functionName := strings.TrimSpace(item.Function.Name)
		if functionName == "" {
			return nil, errors.New("tool function name is required")
		}

		schema := map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
		schemaRaw := strings.TrimSpace(string(item.Function.Parameters))
		if schemaRaw != "" {
			if err := json.Unmarshal(item.Function.Parameters, &schema); err != nil {
				return nil, fmt.Errorf("invalid JSON schema for tool %q: %w", functionName, err)
			}
		}

		spec := brtypes.ToolSpecification{
			Name: aws.String(functionName),
			InputSchema: &brtypes.ToolInputSchemaMemberJson{
				Value: document.NewLazyDocument(schema),
			},
		}
		if description := strings.TrimSpace(item.Function.Description); description != "" {
			spec.Description = aws.String(description)
		}
		if item.Function.Strict != nil {
			spec.Strict = item.Function.Strict
		}

		bedrockTools = append(bedrockTools, &brtypes.ToolMemberToolSpec{Value: spec})
	}

	if len(bedrockTools) == 0 {
		return nil, nil
	}

	toolChoice, disableTools, err := parseToolChoice(rawToolChoice)
	if err != nil {
		return nil, err
	}
	if disableTools {
		return nil, nil
	}

	cfg := &brtypes.ToolConfiguration{
		Tools: bedrockTools,
	}

	// 如果启用了强制工具调用，且 toolChoice 是 auto 或 nil，则强制设置为 any (required)
	if forceToolUse {
		// 检查是否是 auto 或未设置
		_, isAuto := toolChoice.(*brtypes.ToolChoiceMemberAuto)
		if toolChoice == nil || isAuto {
			// 强制使用 any (required)，模型必须调用工具
			cfg.ToolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
		} else {
			cfg.ToolChoice = toolChoice
		}
	} else if toolChoice != nil {
		cfg.ToolChoice = toolChoice
	}

	return cfg, nil
}

func parseToolChoice(raw json.RawMessage) (brtypes.ToolChoice, bool, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, false, nil
	}

	if strings.HasPrefix(trimmed, "\"") {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, false, fmt.Errorf("invalid tool_choice: %w", err)
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "none":
			return nil, true, nil
		case "auto":
			return &brtypes.ToolChoiceMemberAuto{Value: brtypes.AutoToolChoice{}}, false, nil
		case "required":
			return &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}, false, nil
		default:
			return nil, false, fmt.Errorf("unsupported tool_choice value: %s", value)
		}
	}

	var objectChoice struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &objectChoice); err != nil {
		return nil, false, fmt.Errorf("invalid tool_choice object: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(objectChoice.Type)) {
	case "none":
		return nil, true, nil
	case "auto":
		return &brtypes.ToolChoiceMemberAuto{Value: brtypes.AutoToolChoice{}}, false, nil
	case "required":
		return &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}, false, nil
	case "function":
		name := strings.TrimSpace(objectChoice.Function.Name)
		if name == "" {
			return nil, false, errors.New("tool_choice.function.name is required")
		}
		return &brtypes.ToolChoiceMemberTool{
			Value: brtypes.SpecificToolChoice{
				Name: aws.String(name),
			},
		}, false, nil
	default:
		return nil, false, fmt.Errorf("unsupported tool_choice object type: %s", objectChoice.Type)
	}
}

type outputPayload struct {
	Text      string
	ToolCalls []openai.ToolCall
}

func extractOutputPayload(output brtypes.ConverseOutput) outputPayload {
	message, ok := output.(*brtypes.ConverseOutputMemberMessage)
	if !ok {
		return outputPayload{}
	}

	var builder strings.Builder
	toolCalls := make([]openai.ToolCall, 0, 2)
	for _, block := range message.Value.Content {
		switch value := block.(type) {
		case *brtypes.ContentBlockMemberText:
			builder.WriteString(value.Value)
		case *brtypes.ContentBlockMemberToolUse:
			toolCallID := strings.TrimSpace(aws.ToString(value.Value.ToolUseId))
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("toolcall_%d", len(toolCalls)+1)
			}

			toolName := strings.TrimSpace(aws.ToString(value.Value.Name))
			if toolName == "" {
				toolName = "unknown_tool"
			}

			arguments := documentToJSONString(value.Value.Input)

			toolCalls = append(toolCalls, openai.ToolCall{
				ID:   toolCallID,
				Type: "function",
				Function: openai.ToolCallFunction{
					Name:      toolName,
					Arguments: arguments,
				},
			})
		}
	}

	return outputPayload{
		Text:      builder.String(),
		ToolCalls: toolCalls,
	}
}

func documentToJSONString(input document.Interface) string {
	if input == nil {
		return "{}"
	}

	if marshaler, ok := any(input).(smithydocument.Marshaler); ok {
		blob, err := marshaler.MarshalSmithyDocument()
		if err == nil && len(blob) > 0 {
			return string(blob)
		}
	}

	var payload any
	if err := input.UnmarshalSmithyDocument(&payload); err != nil {
		return "{}"
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(blob)
}

func mapStopReason(reason brtypes.StopReason) string {
	switch reason {
	case brtypes.StopReasonMaxTokens:
		return "length"
	case brtypes.StopReasonToolUse:
		return "tool_calls"
	default:
		return "stop"
	}
}

func ptrInt32(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func ptrInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func buildInferenceConfig(request openai.ChatCompletionRequest, defaultMaxOutputToken int32) *brtypes.InferenceConfiguration {
	inferenceConfig := &brtypes.InferenceConfiguration{}
	hasAny := false

	if request.Temperature != nil {
		inferenceConfig.Temperature = aws.Float32(float32(*request.Temperature))
		hasAny = true
	}
	if request.TopP != nil {
		inferenceConfig.TopP = aws.Float32(float32(*request.TopP))
		hasAny = true
	}
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		inferenceConfig.MaxTokens = aws.Int32(int32(*request.MaxTokens))
		hasAny = true
	} else if defaultMaxOutputToken > 0 {
		inferenceConfig.MaxTokens = aws.Int32(defaultMaxOutputToken)
		hasAny = true
	}

	if !hasAny {
		return nil
	}
	return inferenceConfig
}
