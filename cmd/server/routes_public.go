package main

import (
	"context"
	"encoding/json"
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
		for _, tool := range request.Tools {
			if tool.Function != nil {
				toolNames = append(toolNames, tool.Function.Name)
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

	result, err := a.proxy.ConverseStream(ctx, request, bedrockModelID, func(delta bedrockproxy.StreamDelta) error {
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
		if len(delta.ToolCalls) > 0 {
			if err := writeSSEData(w, openai.ChatCompletionChunk{
				ID:      chunkID,
				Object:  "chat.completion.chunk",
				Created: createdAt,
				Model:   modelName,
				Choices: []openai.ChatChunkChoice{{
					Index: 0,
					Delta: openai.ChatChunkDelta{ToolCalls: delta.ToolCalls},
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
		_ = writeSSEData(w, openai.ChatCompletionChunk{
			ID:      chunkID,
			Object:  "chat.completion.chunk",
			Created: createdAt,
			Model:   modelName,
			Choices: []openai.ChatChunkChoice{},
			Error: &openai.OpenAIErrorPayload{
				Message: errorMessage,
				Type:    "server_error",
				Code:    "stream_error",
			},
		})
		_ = writeSSEDone(w)
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
	responseID := "resp-" + requestID
	createdAt := time.Now().Unix()
	statusCode := http.StatusOK

	emitEvent := func(payload any) error {
		if err := writeSSEData(w, payload); err != nil {
			return fmt.Errorf("stream write failed: %w", err)
		}
		return nil
	}

	baseResponse := openai.ResponsesCreateResponse{
		ID:                responseID,
		Object:            "response",
		CreatedAt:         createdAt,
		Status:            "in_progress",
		Model:             modelName,
		Output:            []openai.ResponsesOutputItem{},
		Usage:             openai.ResponsesUsage{},
		ParallelToolCalls: boolOrDefault(request.ParallelToolCalls, true),
		ToolChoice:        request.ToolChoice,
		Error:             nil,
		IncompleteDetails: nil,
	}

	if err := emitEvent(map[string]any{
		"type":     "response.created",
		"response": baseResponse,
	}); err != nil {
		statusCode = http.StatusBadGateway
		return bedrockproxy.ChatResult{}, statusCode, err.Error()
	}
	if err := emitEvent(map[string]any{
		"type":     "response.in_progress",
		"response": baseResponse,
	}); err != nil {
		statusCode = http.StatusBadGateway
		return bedrockproxy.ChatResult{}, statusCode, err.Error()
	}

	messageItemID := "msg_" + requestID
	messageOutputIndex := -1
	nextOutputIndex := 0
	var responseText strings.Builder
	toolStates := make(map[int]*openai.ResponsesFunctionCallState)

	result, err := a.proxy.ConverseStream(ctx, chatRequest, bedrockModelID, func(delta bedrockproxy.StreamDelta) error {
		if delta.Text != "" {
			if messageOutputIndex < 0 {
				messageOutputIndex = nextOutputIndex
				nextOutputIndex++
				item := openai.ResponsesOutputItem{
					ID:     messageItemID,
					Type:   "message",
					Status: "in_progress",
					Role:   "assistant",
					Content: []openai.ResponsesOutputContent{{
						Type:        "output_text",
						Annotations: []any{},
					}},
				}
				if err := emitEvent(map[string]any{
					"type":         "response.output_item.added",
					"response_id":  responseID,
					"output_index": messageOutputIndex,
					"item":         item,
				}); err != nil {
					return err
				}
			}

			responseText.WriteString(delta.Text)
			if err := emitEvent(map[string]any{
				"type":          "response.output_text.delta",
				"response_id":   responseID,
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

				item := openai.ResponsesOutputItem{
					ID:        state.ItemID,
					Type:      "function_call",
					Status:    "in_progress",
					CallID:    state.CallID,
					Name:      state.Name,
					Arguments: "",
				}
				if err := emitEvent(map[string]any{
					"type":         "response.output_item.added",
					"response_id":  responseID,
					"output_index": state.OutputIndex,
					"item":         item,
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
					if err := emitEvent(map[string]any{
						"type":         "response.function_call_arguments.delta",
						"response_id":  responseID,
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
		_ = emitEvent(map[string]any{
			"type":        "response.error",
			"response_id": responseID,
			"error": openai.OpenAIErrorPayload{
				Message: errorMessage,
				Type:    "server_error",
				Code:    "stream_error",
			},
		})
		_ = writeSSEDone(w)
		return bedrockproxy.ChatResult{Text: responseText.String()}, statusCode, errorMessage
	}

	result.Text = responseText.String()
	outputItems := openai.BuildResponsesOutputItems(requestID, result.Text, result.ToolCalls)
	outputText := openai.BuildResponsesOutputText(outputItems)
	completedResponse := openai.ResponsesCreateResponse{
		ID:        responseID,
		Object:    "response",
		CreatedAt: createdAt,
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
