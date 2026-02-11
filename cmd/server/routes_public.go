package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"aws-cursor-router/internal/auth"
	"aws-cursor-router/internal/bedrockproxy"
	"aws-cursor-router/internal/openai"
	"aws-cursor-router/internal/store"
)

func registerPublicRoutes(mux *http.ServeMux, app *App) {
	mux.HandleFunc("/healthz", app.handleHealthz)
	mux.HandleFunc("/v1/models", app.handleListModels)
	mux.HandleFunc("/v1/chat/completions", app.handleChatCompletions)
	mux.HandleFunc("/v1/responses", app.handleResponsesCreate)
	mux.HandleFunc("/debug/test-tool-call", app.handleTestToolCall)
}

// handleTestToolCall 是一个测试端点，用于验证工具调用功能
// 发送一个简单的请求，强制模型调用工具
func (a *App) handleTestToolCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 返回测试信息
	writeJSON(w, http.StatusOK, map[string]any{
		"message":        "工具调用测试端点",
		"force_tool_use": a.cfg.ForceToolUse,
		"instructions": []string{
			"使用 curl 测试工具调用:",
			"curl -X POST http://your-server:8080/v1/chat/completions \\",
			"  -H 'Content-Type: application/json' \\",
			"  -H 'Authorization: Bearer YOUR_API_KEY' \\",
			"  -d '{",
			"    \"model\": \"us.anthropic.claude-3-5-sonnet-20241022-v2:0\",",
			"    \"messages\": [{\"role\": \"user\", \"content\": \"列出当前目录的文件\"}],",
			"    \"tools\": [{",
			"      \"type\": \"function\",",
			"      \"function\": {",
			"        \"name\": \"exec\",",
			"        \"description\": \"执行 shell 命令\",",
			"        \"parameters\": {",
			"          \"type\": \"object\",",
			"          \"properties\": {",
			"            \"command\": {\"type\": \"string\", \"description\": \"要执行的命令\"}",
			"          },",
			"          \"required\": [\"command\"]",
			"        }",
			"      }",
			"    }],",
			"    \"tool_choice\": \"required\"",
			"  }'",
		},
	})
}

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                   true,
		"bedrock_client_ready": a.proxy.HasClient(),
	})
}

func (a *App) handleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	client, err := a.auth.Authenticate(r)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !client.AllowRequest() {
		writeOpenAIError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	models := modelsForClient(a.listCatalogModels(), client)
	now := time.Now().Unix()
	items := make([]openai.ModelInfo, 0, len(models))
	for _, modelID := range models {
		items = append(items, openai.ModelInfo{
			ID:      modelID,
			Object:  "model",
			Created: now,
			OwnedBy: "aws-bedrock",
		})
	}

	writeJSON(w, http.StatusOK, openai.ModelsResponse{
		Object: "list",
		Data:   items,
	})
}

func (a *App) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !a.proxy.HasClient() {
		writeOpenAIError(w, http.StatusServiceUnavailable, "bedrock client is not configured")
		return
	}

	client, err := a.auth.Authenticate(r)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !client.AllowRequest() {
		writeOpenAIError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	if err := a.checkGlobalCostLimit(); err != nil {
		writeOpenAIError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
	defer cancel()

	release, err := a.auth.Acquire(ctx, client)
	if err != nil {
		writeOpenAIError(w, http.StatusTooManyRequests, "concurrency limit exceeded")
		return
	}
	defer release()

	var request openai.ChatCompletionRequest
	if err := decodeJSONBody(w, r, a.cfg.MaxBodyBytes, &request); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := openai.ValidateChatRequest(request); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 关键调试日志：打印请求中的工具信息
	a.logger.Printf("========== 请求处理开始 ==========")
	a.logger.Printf("模型: %s, 流式: %v, 工具数量: %d", request.Model, request.Stream, len(request.Tools))
	if len(request.Tools) > 0 {
		toolNames := make([]string, 0, len(request.Tools))
		for i, tool := range request.Tools {
			// 使用 GetFunction() 获取函数定义
			fn := tool.GetFunction()
			// 打印每个工具的详细信息
			a.logger.Printf("工具 %d: type=%q, function=%v, name=%q", i, tool.Type, fn != nil, tool.Name)
			if fn != nil {
				toolNames = append(toolNames, fn.Name)
				a.logger.Printf("  -> name=%q", fn.Name)
			} else {
				a.logger.Printf("  -> ⚠️ Function 为 nil!")
			}
		}
		a.logger.Printf("工具列表: %v", toolNames)
	} else {
		a.logger.Printf("⚠️ 请求中没有工具定义！")
	}
	if len(request.ToolChoice) > 0 {
		a.logger.Printf("tool_choice: %s", string(request.ToolChoice))
	} else {
		a.logger.Printf("tool_choice: (未设置)")
	}
	a.logger.Printf("FORCE_TOOL_USE 配置: %v", a.cfg.ForceToolUse)
	a.logger.Printf("===================================")

	resolvedModel, bedrockModelID, err := a.proxy.ResolveModel(request.Model)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	requestID := strings.TrimSpace(r.Header.Get("x-request-id"))
	if requestID == "" {
		requestID = newRequestID()
	}
	startedAt := time.Now().UTC()
	logModel := resolvedModel
	if logModel == "default" {
		logModel = bedrockModelID
	}

	record := store.CallRecord{
		RequestID:      requestID,
		ClientID:       client.ID,
		Model:          logModel,
		BedrockModelID: bedrockModelID,
		RequestContent: openai.RenderRequestForLog(request, a.cfg.MaxContentChars),
		IsStream:       request.Stream,
		CreatedAt:      startedAt,
	}

	statusCode := http.StatusOK
	errorMessage := ""
	responseContent := ""
	inputTokens := 0
	outputTokens := 0
	totalTokens := 0
	latencyMs := int64(0)

	defer func() {
		record.StatusCode = statusCode
		record.ErrorMessage = truncateRunes(errorMessage, a.cfg.MaxContentChars)
		record.ResponseContent = truncateRunes(responseContent, a.cfg.MaxContentChars)
		record.InputTokens = inputTokens
		record.OutputTokens = outputTokens
		record.TotalTokens = totalTokens
		if latencyMs > 0 {
			record.LatencyMs = latencyMs
		} else {
			record.LatencyMs = time.Since(startedAt).Milliseconds()
		}
		if !a.store.Enqueue(record) {
			a.logger.Printf("warning: dropped call log for request_id=%s client_id=%s", requestID, client.ID)
			return
		}
		a.addCostFromUsage(record.BedrockModelID, int64(record.InputTokens), int64(record.OutputTokens))
	}()

	if !a.isModelEnabled(bedrockModelID) {
		statusCode = http.StatusForbidden
		errorMessage = "model is not enabled by admin"
		writeOpenAIError(w, statusCode, errorMessage)
		return
	}
	if !client.IsModelAllowed(resolvedModel, bedrockModelID) {
		statusCode = http.StatusForbidden
		errorMessage = "model is not allowed for this api key"
		writeOpenAIError(w, statusCode, errorMessage)
		return
	}

	if request.Stream {
		result, streamStatus, streamErr := a.handleChatCompletionsStream(
			w,
			ctx,
			request,
			requestID,
			resolvedModel,
			bedrockModelID,
		)
		statusCode = streamStatus
		errorMessage = streamErr
		inputTokens = result.InputTokens
		outputTokens = result.OutputTokens
		totalTokens = result.TotalTokens
		latencyMs = result.LatencyMs
		responseContent = renderAssistantContentForLog(result.Text, result.ToolCalls)
		if latencyMs == 0 {
			latencyMs = time.Since(startedAt).Milliseconds()
		}
		return
	}

	result, err := a.proxy.Converse(ctx, request, bedrockModelID)
	if err != nil {
		statusCode = http.StatusBadGateway
		errorMessage = err.Error()
		writeOpenAIError(w, statusCode, "bedrock call failed: "+err.Error())
		return
	}

	assistantContent := buildAssistantMessageContent(result.Text, len(result.ToolCalls) > 0)
	modelName := resolvedModel
	if modelName == "default" {
		modelName = bedrockModelID
	}

	response := openai.ChatCompletionResponse{
		ID:      "chatcmpl-" + requestID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   modelName,
		Choices: []openai.ChatCompletionChoice{{
			Index: 0,
			Message: openai.ChatMessage{
				Role:      "assistant",
				Content:   assistantContent,
				ToolCalls: result.ToolCalls,
			},
			FinishReason: defaultFinishReason(result.FinishReason),
		}},
		Usage: openai.Usage{
			PromptTokens:     result.InputTokens,
			CompletionTokens: result.OutputTokens,
			TotalTokens:      result.TotalTokens,
		},
	}

	responseContent = renderAssistantContentForLog(result.Text, result.ToolCalls)
	inputTokens = result.InputTokens
	outputTokens = result.OutputTokens
	totalTokens = result.TotalTokens
	latencyMs = result.LatencyMs
	if latencyMs == 0 {
		latencyMs = time.Since(startedAt).Milliseconds()
	}

	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleResponsesCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !a.proxy.HasClient() {
		writeOpenAIError(w, http.StatusServiceUnavailable, "bedrock client is not configured")
		return
	}

	client, err := a.auth.Authenticate(r)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if !client.AllowRequest() {
		writeOpenAIError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	if err := a.checkGlobalCostLimit(); err != nil {
		writeOpenAIError(w, http.StatusTooManyRequests, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.RequestTimeout)
	defer cancel()

	release, err := a.auth.Acquire(ctx, client)
	if err != nil {
		writeOpenAIError(w, http.StatusTooManyRequests, "concurrency limit exceeded")
		return
	}
	defer release()

	var request openai.ResponsesCreateRequest
	if err := decodeJSONBody(w, r, a.cfg.MaxBodyBytes, &request); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := openai.ValidateResponsesCreateRequest(request); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	chatRequest, err := openai.ResponsesRequestToChat(request)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	resolvedModel, bedrockModelID, err := a.proxy.ResolveModel(chatRequest.Model)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error())
		return
	}

	requestID := strings.TrimSpace(r.Header.Get("x-request-id"))
	if requestID == "" {
		requestID = newRequestID()
	}
	startedAt := time.Now().UTC()
	logModel := resolvedModel
	if logModel == "default" {
		logModel = bedrockModelID
	}

	record := store.CallRecord{
		RequestID:      requestID,
		ClientID:       client.ID,
		Model:          logModel,
		BedrockModelID: bedrockModelID,
		RequestContent: openai.RenderRequestForLog(chatRequest, a.cfg.MaxContentChars),
		IsStream:       chatRequest.Stream,
		CreatedAt:      startedAt,
	}

	statusCode := http.StatusOK
	errorMessage := ""
	responseContent := ""
	inputTokens := 0
	outputTokens := 0
	totalTokens := 0
	latencyMs := int64(0)

	defer func() {
		record.StatusCode = statusCode
		record.ErrorMessage = truncateRunes(errorMessage, a.cfg.MaxContentChars)
		record.ResponseContent = truncateRunes(responseContent, a.cfg.MaxContentChars)
		record.InputTokens = inputTokens
		record.OutputTokens = outputTokens
		record.TotalTokens = totalTokens
		if latencyMs > 0 {
			record.LatencyMs = latencyMs
		} else {
			record.LatencyMs = time.Since(startedAt).Milliseconds()
		}
		if !a.store.Enqueue(record) {
			a.logger.Printf("warning: dropped call log for request_id=%s client_id=%s", requestID, client.ID)
			return
		}
		a.addCostFromUsage(record.BedrockModelID, int64(record.InputTokens), int64(record.OutputTokens))
	}()

	if !a.isModelEnabled(bedrockModelID) {
		statusCode = http.StatusForbidden
		errorMessage = "model is not enabled by admin"
		writeOpenAIError(w, statusCode, errorMessage)
		return
	}
	if !client.IsModelAllowed(resolvedModel, bedrockModelID) {
		statusCode = http.StatusForbidden
		errorMessage = "model is not allowed for this api key"
		writeOpenAIError(w, statusCode, errorMessage)
		return
	}

	if chatRequest.Stream {
		result, streamStatus, streamErr := a.handleResponsesStream(
			w,
			ctx,
			request,
			chatRequest,
			requestID,
			resolvedModel,
			bedrockModelID,
		)
		statusCode = streamStatus
		errorMessage = streamErr
		inputTokens = result.InputTokens
		outputTokens = result.OutputTokens
		totalTokens = result.TotalTokens
		latencyMs = result.LatencyMs
		responseItems := openai.BuildResponsesOutputItems(requestID, result.Text, result.ToolCalls)
		responseContent = renderResponsesOutputForLog(responseItems)
		if latencyMs == 0 {
			latencyMs = time.Since(startedAt).Milliseconds()
		}
		return
	}

	result, err := a.proxy.Converse(ctx, chatRequest, bedrockModelID)
	if err != nil {
		statusCode = http.StatusBadGateway
		errorMessage = err.Error()
		writeOpenAIError(w, statusCode, "bedrock call failed: "+err.Error())
		return
	}

	modelName := resolvedModel
	if modelName == "default" {
		modelName = bedrockModelID
	}
	outputItems := openai.BuildResponsesOutputItems(requestID, result.Text, result.ToolCalls)
	outputText := openai.BuildResponsesOutputText(outputItems)

	response := openai.ResponsesCreateResponse{
		ID:        "resp-" + requestID,
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Status:    "completed",
		Model:     modelName,
		Output:    outputItems,
		Usage: openai.ResponsesUsage{
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			TotalTokens:  result.TotalTokens,
		},
		ParallelToolCalls: boolOrDefault(request.ParallelToolCalls, true),
		ToolChoice:        request.ToolChoice,
		OutputText:        outputText,
		Error:             nil,
		IncompleteDetails: nil,
	}

	responseContent = renderResponsesOutputForLog(outputItems)
	inputTokens = result.InputTokens
	outputTokens = result.OutputTokens
	totalTokens = result.TotalTokens
	latencyMs = result.LatencyMs
	if latencyMs == 0 {
		latencyMs = time.Since(startedAt).Milliseconds()
	}

	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleChatCompletionsStream(
	w http.ResponseWriter,
	ctx context.Context,
	request openai.ChatCompletionRequest,
	requestID string,
	resolvedModel string,
	bedrockModelID string,
) (bedrockproxy.ChatResult, int, string) {
	setSSEHeaders(w)
	modelName := resolvedModel
	if modelName == "default" {
		modelName = bedrockModelID
	}
	chunkID := "chatcmpl-" + requestID
	createdAt := time.Now().Unix()
	statusCode := http.StatusOK
	var responseText strings.Builder

	// 使用与请求断开无关的 context，避免 Cursor/代理断开时取消 Bedrock 流；
	// 仅受 REQUEST_TIMEOUT 限制。客户端断开时会在下次 writeSSEData 失败并停止。
	streamCtx, streamCancel := context.WithTimeout(context.Background(), a.cfg.RequestTimeout)
	defer streamCancel()

	result, err := a.proxy.ConverseStream(streamCtx, request, bedrockModelID, func(delta bedrockproxy.StreamDelta) error {
		// 根据 OpenAI 规范，tool_calls 的第一个 chunk 需要同时包含 role 和 tool_calls
		// 所以我们需要检查是否同时有 role 和 tool_calls，如果有则合并到一个 chunk 中发送
		if len(delta.ToolCalls) > 0 {
			chunkDelta := openai.ChatChunkDelta{ToolCalls: delta.ToolCalls}
			if delta.Role != "" {
				chunkDelta.Role = delta.Role
			}
			if err := writeSSEData(w, openai.ChatCompletionChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   modelName,
				Choices: []openai.ChatChunkChoice{{
					Index: 0,
					Delta: chunkDelta,
				}},
			}); err != nil {
				return err
			}
			return nil
		}
		if delta.Role != "" {
			if err := writeSSEData(w, openai.ChatCompletionChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   modelName,
				Choices: []openai.ChatChunkChoice{{
					Index: 0,
					Delta: openai.ChatChunkDelta{Role: delta.Role},
				}},
			}); err != nil {
				return err
			}
		}
		if delta.Text != "" {
			responseText.WriteString(delta.Text)
			if err := writeSSEData(w, openai.ChatCompletionChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   modelName,
				Choices: []openai.ChatChunkChoice{{
					Index: 0,
					Delta: openai.ChatChunkDelta{Content: delta.Text},
				}},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		statusCode = http.StatusBadGateway
		errorMessage := "bedrock stream failed: " + err.Error()
		// context canceled 多为客户端断开、代理超时或服务端 REQUEST_TIMEOUT 过短
		if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled") {
			errorMessage += " (请求被取消：请检查 Cursor/代理是否过早断开，或调大环境变量 REQUEST_TIMEOUT_SECONDS)"
		}

		// 在尚未开始向客户端写入任何 SSE 数据时，直接按 OpenAI 错误格式返回 JSON，
		// 这样 Cursor 可以在 UI 中清晰展示错误信息，而不会出现“什么都没显示”的情况。
		writeOpenAIError(w, statusCode, errorMessage)

		return bedrockproxy.ChatResult{Text: responseText.String()}, statusCode, errorMessage
	}

	finishReason := defaultFinishReason(result.FinishReason)
	
	// 调试日志：打印流式响应结果
	a.logger.Printf("========== 流式响应完成 ==========")
	a.logger.Printf("finish_reason: %s (原始: %s)", finishReason, result.FinishReason)
	a.logger.Printf("工具调用数量: %d", len(result.ToolCalls))
	if len(result.ToolCalls) > 0 {
		for i, tc := range result.ToolCalls {
			a.logger.Printf("  工具调用 %d: id=%s, name=%s", i+1, tc.ID, tc.Function.Name)
		}
	}
	a.logger.Printf("===================================")
	
	if err := writeSSEData(w, openai.ChatCompletionChunk{
		ID:      chunkID,
		Object:  "chat.completion.chunk",
		Created: createdAt,
		Model:   modelName,
		Choices: []openai.ChatChunkChoice{{
			Index:        0,
			Delta:        openai.ChatChunkDelta{},
			FinishReason: &finishReason,
		}},
		Usage: &openai.Usage{
			PromptTokens:     result.InputTokens,
			CompletionTokens: result.OutputTokens,
			TotalTokens:      result.TotalTokens,
		},
	}); err != nil {
		statusCode = http.StatusBadGateway
		errorMessage := "stream write failed: " + err.Error()
		return bedrockproxy.ChatResult{Text: responseText.String()}, statusCode, errorMessage
	}
	if err := writeSSEDone(w); err != nil {
		statusCode = http.StatusBadGateway
		errorMessage := "stream completion failed: " + err.Error()
		return bedrockproxy.ChatResult{Text: responseText.String()}, statusCode, errorMessage
	}

	result.Text = responseText.String()
	return result, http.StatusOK, ""
}

func (a *App) handleResponsesStream(
	w http.ResponseWriter,
	ctx context.Context,
	request openai.ResponsesCreateRequest,
	chatRequest openai.ChatCompletionRequest,
	requestID string,
	resolvedModel string,
	bedrockModelID string,
) (bedrockproxy.ChatResult, int, string) {
	setSSEHeaders(w)

	modelName := resolvedModel
	if modelName == "default" {
		modelName = bedrockModelID
	}
	responseID := "resp_" + requestID
	createdAt := time.Now().Unix()
	statusCode := http.StatusOK

	// sequence_number 用于标识事件顺序，从 0 开始递增
	var seqNum int64 = 0
	nextSeq := func() int64 {
		n := seqNum
		seqNum++
		return n
	}

	emitEvent := func(payload map[string]any) error {
		// 根据 OpenAI 规范，每个事件都需要 sequence_number
		payload["sequence_number"] = nextSeq()
		if err := writeSSEData(w, payload); err != nil {
			return fmt.Errorf("stream write failed: %w", err)
		}
		return nil
	}

	baseResponse := map[string]any{
		"id":                  responseID,
		"object":              "response",
		"created_at":          createdAt,
		"status":              "in_progress",
		"model":               modelName,
		"output":              []any{},
		"parallel_tool_calls": boolOrDefault(request.ParallelToolCalls, true),
		"tool_choice":         request.ToolChoice,
		"error":               nil,
		"incomplete_details":  nil,
		"usage":               nil,
	}

	// response.created
	if err := emitEvent(map[string]any{
		"type":     "response.created",
		"response": baseResponse,
	}); err != nil {
		statusCode = http.StatusBadGateway
		return bedrockproxy.ChatResult{}, statusCode, err.Error()
	}

	// response.in_progress
	if err := emitEvent(map[string]any{
		"type":     "response.in_progress",
		"response": baseResponse,
	}); err != nil {
		statusCode = http.StatusBadGateway
		return bedrockproxy.ChatResult{}, statusCode, err.Error()
	}

	messageItemID := "msg_" + requestID
	messageOutputIndex := -1
	messageContentPartAdded := false
	nextOutputIndex := 0
	var responseText strings.Builder
	toolStates := make(map[int]*openai.ResponsesFunctionCallState)

	streamCtx, streamCancel := context.WithTimeout(context.Background(), a.cfg.RequestTimeout)
	defer streamCancel()

	result, err := a.proxy.ConverseStream(streamCtx, chatRequest, bedrockModelID, func(delta bedrockproxy.StreamDelta) error {
		if delta.Text != "" {
			if messageOutputIndex < 0 {
				messageOutputIndex = nextOutputIndex
				nextOutputIndex++
				// response.output_item.added for message
				if err := emitEvent(map[string]any{
					"type":         "response.output_item.added",
					"output_index": messageOutputIndex,
					"item": map[string]any{
						"id":      messageItemID,
						"type":    "message",
						"status":  "in_progress",
						"role":    "assistant",
						"content": []any{},
					},
				}); err != nil {
					return err
				}
			}

			// response.content_part.added (只发送一次)
			if !messageContentPartAdded {
				messageContentPartAdded = true
				if err := emitEvent(map[string]any{
					"type":          "response.content_part.added",
					"item_id":       messageItemID,
					"output_index":  messageOutputIndex,
					"content_index": 0,
					"part": map[string]any{
						"type":        "output_text",
						"text":        "",
						"annotations": []any{},
					},
				}); err != nil {
					return err
				}
			}

			responseText.WriteString(delta.Text)
			// response.output_text.delta
			if err := emitEvent(map[string]any{
				"type":          "response.output_text.delta",
				"output_index":  messageOutputIndex,
				"item_id":       messageItemID,
				"content_index": 0,
				"delta":         delta.Text,
			}); err != nil {
				return err
			}
		}

		for _, chunk := range delta.ToolCalls {
			state, exists := toolStates[chunk.Index]
			if !exists {
				callID := strings.TrimSpace(chunk.ID)
				if callID == "" {
					callID = fmt.Sprintf("call_%d", chunk.Index+1)
				}
				name := ""
				if chunk.Function != nil {
					name = strings.TrimSpace(chunk.Function.Name)
				}
				state = &openai.ResponsesFunctionCallState{
					OutputIndex: nextOutputIndex,
					ItemID:      "fc_" + callID,
					CallID:      callID,
					Name:        name,
				}
				toolStates[chunk.Index] = state
				nextOutputIndex++

				// response.output_item.added for function_call
				if err := emitEvent(map[string]any{
					"type":         "response.output_item.added",
					"output_index": state.OutputIndex,
					"item": map[string]any{
						"id":        state.ItemID,
						"type":      "function_call",
						"status":    "in_progress",
						"call_id":   state.CallID,
						"name":      state.Name,
						"arguments": "",
					},
				}); err != nil {
					return err
				}
			}

			if chunk.Function != nil {
				if state.Name == "" {
					state.Name = strings.TrimSpace(chunk.Function.Name)
				}
				if chunk.Function.Arguments != "" {
					state.Arguments += chunk.Function.Arguments
					// response.function_call_arguments.delta
					if err := emitEvent(map[string]any{
						"type":         "response.function_call_arguments.delta",
						"output_index": state.OutputIndex,
						"item_id":      state.ItemID,
						"call_id":      state.CallID,
						"delta":        chunk.Function.Arguments,
					}); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		statusCode = http.StatusBadGateway
		errorMessage := "bedrock stream failed: " + err.Error()
		if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled") {
			errorMessage += " (请求被取消：请检查 Cursor/代理是否过早断开，或调大环境变量 REQUEST_TIMEOUT_SECONDS)"
		}
		_ = emitEvent(map[string]any{
			"type": "error",
			"error": map[string]any{
				"message": errorMessage,
				"type":    "server_error",
				"code":    "stream_error",
			},
		})
		_ = writeSSEDone(w)
		return bedrockproxy.ChatResult{Text: responseText.String()}, statusCode, errorMessage
	}

	result.Text = responseText.String()
	outputItems := openai.BuildResponsesOutputItems(requestID, result.Text, result.ToolCalls)
	outputText := openai.BuildResponsesOutputText(outputItems)

	// 发送完成事件
	// 1. 消息项的完成事件
	if messageOutputIndex >= 0 {
		// response.output_text.done
		if err := emitEvent(map[string]any{
			"type":          "response.output_text.done",
			"output_index":  messageOutputIndex,
			"item_id":       messageItemID,
			"content_index": 0,
			"text":          result.Text,
		}); err != nil {
			statusCode = http.StatusBadGateway
			return result, statusCode, err.Error()
		}
		// response.content_part.done
		if err := emitEvent(map[string]any{
			"type":          "response.content_part.done",
			"item_id":       messageItemID,
			"output_index":  messageOutputIndex,
			"content_index": 0,
			"part": map[string]any{
				"type":        "output_text",
				"text":        result.Text,
				"annotations": []any{},
			},
		}); err != nil {
			statusCode = http.StatusBadGateway
			return result, statusCode, err.Error()
		}
		// response.output_item.done for message
		if err := emitEvent(map[string]any{
			"type":         "response.output_item.done",
			"output_index": messageOutputIndex,
			"item": map[string]any{
				"id":     messageItemID,
				"type":   "message",
				"status": "completed",
				"role":   "assistant",
				"content": []map[string]any{{
					"type":        "output_text",
					"text":        result.Text,
					"annotations": []any{},
				}},
			},
		}); err != nil {
			statusCode = http.StatusBadGateway
			return result, statusCode, err.Error()
		}
	}

	// 2. 工具调用项的完成事件
	for _, state := range toolStates {
		// response.function_call_arguments.done
		if err := emitEvent(map[string]any{
			"type":         "response.function_call_arguments.done",
			"output_index": state.OutputIndex,
			"item_id":      state.ItemID,
			"call_id":      state.CallID,
			"arguments":    state.Arguments,
		}); err != nil {
			statusCode = http.StatusBadGateway
			return result, statusCode, err.Error()
		}
		// response.output_item.done for function_call
		if err := emitEvent(map[string]any{
			"type":         "response.output_item.done",
			"output_index": state.OutputIndex,
			"item": map[string]any{
				"id":        state.ItemID,
				"type":      "function_call",
				"status":    "completed",
				"call_id":   state.CallID,
				"name":      state.Name,
				"arguments": state.Arguments,
			},
		}); err != nil {
			statusCode = http.StatusBadGateway
			return result, statusCode, err.Error()
		}
	}

	// 构建完成的 output 数组
	completedOutput := make([]map[string]any, 0)
	if messageOutputIndex >= 0 {
		completedOutput = append(completedOutput, map[string]any{
			"id":     messageItemID,
			"type":   "message",
			"status": "completed",
			"role":   "assistant",
			"content": []map[string]any{{
				"type":        "output_text",
				"text":        result.Text,
				"annotations": []any{},
			}},
		})
	}
	for _, state := range toolStates {
		completedOutput = append(completedOutput, map[string]any{
			"id":        state.ItemID,
			"type":      "function_call",
			"status":    "completed",
			"call_id":   state.CallID,
			"name":      state.Name,
			"arguments": state.Arguments,
		})
	}

	// response.completed
	completedResponse := map[string]any{
		"id":                  responseID,
		"object":              "response",
		"created_at":          createdAt,
		"status":              "completed",
		"model":               modelName,
		"output":              completedOutput,
		"output_text":         outputText,
		"parallel_tool_calls": boolOrDefault(request.ParallelToolCalls, true),
		"tool_choice":         request.ToolChoice,
		"error":               nil,
		"incomplete_details":  nil,
		"usage": map[string]any{
			"input_tokens":  result.InputTokens,
			"output_tokens": result.OutputTokens,
			"total_tokens":  result.TotalTokens,
		},
	}
	if err := emitEvent(map[string]any{
		"type":     "response.completed",
		"response": completedResponse,
	}); err != nil {
		statusCode = http.StatusBadGateway
		errorMessage := err.Error()
		return result, statusCode, errorMessage
	}

	if err := writeSSEDone(w); err != nil {
		statusCode = http.StatusBadGateway
		errorMessage := "stream completion failed: " + err.Error()
		return result, statusCode, errorMessage
	}

	return result, http.StatusOK, ""
}

func modelsForClient(catalog []string, client *auth.Client) []string {
	catalog = normalizeModelIDs(catalog)
	if client == nil {
		return catalog
	}
	if len(client.AllowedModels) == 0 {
		return catalog
	}

	if len(catalog) == 0 {
		out := make([]string, 0, len(client.AllowedModels))
		for modelID := range client.AllowedModels {
			out = append(out, modelID)
		}
		sort.Strings(out)
		return out
	}

	out := make([]string, 0, len(catalog))
	for _, modelID := range catalog {
		if _, ok := client.AllowedModels[strings.ToLower(modelID)]; ok {
			out = append(out, modelID)
		}
	}
	return out
}

func defaultFinishReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "stop"
	}
	return reason
}

func buildAssistantMessageContent(text string, hasToolCalls bool) json.RawMessage {
	if hasToolCalls && strings.TrimSpace(text) == "" {
		return json.RawMessage("null")
	}
	payload, err := json.Marshal(text)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return payload
}

func renderAssistantContentForLog(text string, toolCalls []openai.ToolCall) string {
	text = strings.TrimSpace(text)
	if len(toolCalls) == 0 {
		return text
	}
	payload, err := json.Marshal(toolCalls)
	if err != nil {
		return text
	}
	if text == "" {
		return "tool_calls=" + string(payload)
	}
	return text + "\ntool_calls=" + string(payload)
}

func renderResponsesOutputForLog(items []openai.ResponsesOutputItem) string {
	if len(items) == 0 {
		return ""
	}
	payload, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	return string(payload)
}

func boolOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
