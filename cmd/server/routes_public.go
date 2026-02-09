package main

import (
	"context"
	"encoding/json"
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
		RequestContent: openai.RenderMessagesForLog(request.Messages, a.cfg.MaxContentChars),
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
		responseContent = result.Text
		inputTokens = result.InputTokens
		outputTokens = result.OutputTokens
		totalTokens = result.TotalTokens
		latencyMs = result.LatencyMs
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

	assistantContent, _ := json.Marshal(result.Text)
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
				Role:    "assistant",
				Content: assistantContent,
			},
			FinishReason: defaultFinishReason(result.FinishReason),
		}},
		Usage: openai.Usage{
			PromptTokens:     result.InputTokens,
			CompletionTokens: result.OutputTokens,
			TotalTokens:      result.TotalTokens,
		},
	}

	responseContent = result.Text
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
