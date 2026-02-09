package bedrockproxy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"aws-cursor-router/internal/openai"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
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
}

type ChatResult struct {
	Text         string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	LatencyMs    int64
	FinishReason string
}

type StreamDelta struct {
	Role         string
	Text         string
	FinishReason string
}

func NewService(
	client ConverseAPI,
	defaultModelID string,
	modelRouter map[string]string,
	defaultMaxOutputToken int32,
) *Service {
	_ = modelRouter

	return &Service{
		client:                client,
		defaultModelID:        strings.TrimSpace(defaultModelID),
		defaultMaxOutputToken: defaultMaxOutputToken,
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
	messages, system, err := BuildBedrockMessages(request.Messages)
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
	})
	if err != nil {
		return ChatResult{}, err
	}

	result := ChatResult{
		Text:         extractOutputText(output.Output),
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
	messages, system, err := BuildBedrockMessages(request.Messages)
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

	output, err := client.ConverseStream(ctx, &bedrockruntime.ConverseStreamInput{
		ModelId:         aws.String(bedrockModelID),
		Messages:        messages,
		System:          system,
		InferenceConfig: inferenceConfig,
	})
	if err != nil {
		return ChatResult{}, err
	}
	stream := output.GetStream()
	defer func() { _ = stream.Close() }()

	result := ChatResult{FinishReason: "stop"}
	var textBuilder strings.Builder
	roleSent := false

	for event := range stream.Events() {
		switch value := event.(type) {
		case *brtypes.ConverseStreamOutputMemberMessageStart:
			if !roleSent {
				roleSent = true
				if err := onDelta(StreamDelta{Role: string(value.Value.Role)}); err != nil {
					return ChatResult{}, err
				}
			}
		case *brtypes.ConverseStreamOutputMemberContentBlockDelta:
			textDelta, ok := value.Value.Delta.(*brtypes.ContentBlockDeltaMemberText)
			if !ok {
				continue
			}
			if !roleSent {
				roleSent = true
				if err := onDelta(StreamDelta{Role: "assistant"}); err != nil {
					return ChatResult{}, err
				}
			}
			if textDelta.Value == "" {
				continue
			}
			textBuilder.WriteString(textDelta.Value)
			if err := onDelta(StreamDelta{Text: textDelta.Value}); err != nil {
				return ChatResult{}, err
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
	return result, nil
}

func BuildBedrockMessages(messages []openai.ChatMessage) ([]brtypes.Message, []brtypes.SystemContentBlock, error) {
	outMessages := make([]brtypes.Message, 0, len(messages))
	outSystem := make([]brtypes.SystemContentBlock, 0, 2)

	for _, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		text, err := openai.DecodeContentAsText(message.Content)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid message content for role %q: %w", role, err)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}

		switch role {
		case "system":
			outSystem = append(outSystem, &brtypes.SystemContentBlockMemberText{Value: text})
		case "assistant":
			outMessages = append(outMessages, brtypes.Message{
				Role:    brtypes.ConversationRoleAssistant,
				Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberText{Value: text}},
			})
		case "user", "":
			outMessages = append(outMessages, brtypes.Message{
				Role:    brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{&brtypes.ContentBlockMemberText{Value: text}},
			})
		default:
			// Ignore unsupported roles for now to keep cursor compatibility stable.
		}
	}

	if len(outMessages) == 0 {
		return nil, nil, errors.New("at least one non-system message is required")
	}
	return outMessages, outSystem, nil
}

func extractOutputText(output brtypes.ConverseOutput) string {
	message, ok := output.(*brtypes.ConverseOutputMemberMessage)
	if !ok {
		return ""
	}

	var builder strings.Builder
	for _, block := range message.Value.Content {
		switch value := block.(type) {
		case *brtypes.ContentBlockMemberText:
			builder.WriteString(value.Value)
		}
	}
	return builder.String()
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
