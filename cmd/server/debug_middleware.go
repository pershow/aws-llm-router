package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

var debugRequestCounter uint64

type sseDebugAnalysis struct {
	hasToolCalls      bool
	finishReason      string
	toolCallNames     []string
	completionTokens  int
	toolCallArguments map[int]string
}

func debugMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	debugEnabled := strings.EqualFold(strings.TrimSpace(os.Getenv("DEBUG_REQUESTS")), "true")
	debugLogDir := strings.TrimSpace(os.Getenv("DEBUG_LOG_DIR"))
	if debugLogDir == "" {
		debugLogDir = "./debug_logs"
	}

	if debugEnabled {
		if err := os.MkdirAll(debugLogDir, 0o755); err != nil {
			logger.Printf("warning: cannot create debug log dir %s: %v", debugLogDir, err)
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !debugEnabled || !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		startTime := time.Now()
		counter := atomic.AddUint64(&debugRequestCounter, 1)
		requestID := fmt.Sprintf("%s_%04d", time.Now().Format("20060102_150405"), counter)

		logger.Printf("[DEBUG-%s] === request start ===", requestID)
		logger.Printf("[DEBUG-%s] method=%s path=%s", requestID, r.Method, r.URL.Path)
		for name, values := range r.Header {
			if strings.EqualFold(name, "authorization") || strings.EqualFold(name, "x-api-key") {
				logger.Printf("[DEBUG-%s] header %s: [REDACTED]", requestID, name)
				continue
			}
			logger.Printf("[DEBUG-%s] header %s: %s", requestID, name, strings.Join(values, ", "))
		}

		var requestBody []byte
		var toolsCount int
		var toolChoice any
		var messagesCount int
		var requestMaxTokens int
		var requestMaxOutputTokens int

		if r.Body != nil {
			requestBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}
		if len(requestBody) > 0 {
			var reqData map[string]any
			if err := json.Unmarshal(requestBody, &reqData); err == nil {
				if tools, ok := reqData["tools"].([]any); ok {
					toolsCount = len(tools)
					logger.Printf("[DEBUG-%s] request includes %d tools", requestID, toolsCount)
					for i, item := range tools {
						toolMap, ok := item.(map[string]any)
						if !ok {
							continue
						}
						if fn, ok := toolMap["function"].(map[string]any); ok {
							if name, ok := fn["name"].(string); ok && strings.TrimSpace(name) != "" {
								logger.Printf("[DEBUG-%s] tool %d: %s", requestID, i+1, name)
							}
						}
					}
				} else {
					logger.Printf("[DEBUG-%s] request includes no tools", requestID)
				}

				if tc, ok := reqData["tool_choice"]; ok {
					toolChoice = tc
					logger.Printf("[DEBUG-%s] tool_choice=%v", requestID, toolChoice)
				}
				if messages, ok := reqData["messages"].([]any); ok {
					messagesCount = len(messages)
					logger.Printf("[DEBUG-%s] messages=%d", requestID, messagesCount)
				}
				if model, ok := reqData["model"].(string); ok {
					logger.Printf("[DEBUG-%s] model=%s", requestID, model)
				}
				if stream, ok := reqData["stream"].(bool); ok {
					logger.Printf("[DEBUG-%s] stream=%v", requestID, stream)
				}
				if raw, ok := reqData["max_tokens"]; ok {
					if value, ok := jsonNumberToInt(raw); ok {
						requestMaxTokens = value
						logger.Printf("[DEBUG-%s] max_tokens=%d", requestID, requestMaxTokens)
					}
				}
				if raw, ok := reqData["max_output_tokens"]; ok {
					if value, ok := jsonNumberToInt(raw); ok {
						requestMaxOutputTokens = value
						logger.Printf("[DEBUG-%s] max_output_tokens=%d", requestID, requestMaxOutputTokens)
					}
				}
			}

			reqFilePath := filepath.Join(debugLogDir, fmt.Sprintf("%s_request.json", requestID))
			saveDebugFile(logger, reqFilePath, requestBody, requestID, "request")
		}

		recorder := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			body:           &bytes.Buffer{},
		}

		next.ServeHTTP(recorder, r)

		duration := time.Since(startTime)
		logger.Printf("[DEBUG-%s] status=%d duration=%v", requestID, recorder.statusCode, duration)

		if recorder.body.Len() > 0 {
			respFilePath := filepath.Join(debugLogDir, fmt.Sprintf("%s_response.txt", requestID))
			contentType := strings.ToLower(strings.TrimSpace(w.Header().Get("Content-Type")))

			if strings.Contains(contentType, "text/event-stream") {
				logger.Printf("[DEBUG-%s] streaming response bytes=%d", requestID, recorder.body.Len())
				analysis := analyzeSSEDebug(recorder.body.String())

				if analysis.hasToolCalls {
					logger.Printf("[DEBUG-%s] stream includes tool calls: %v", requestID, analysis.toolCallNames)
				} else {
					logger.Printf("[DEBUG-%s] stream includes no tool calls", requestID)
				}
				if analysis.finishReason != "" {
					logger.Printf("[DEBUG-%s] finish_reason=%s", requestID, analysis.finishReason)
				}
				if analysis.completionTokens > 0 {
					logger.Printf("[DEBUG-%s] completion_tokens=%d", requestID, analysis.completionTokens)
				}

				saveDebugFile(logger, respFilePath, recorder.body.Bytes(), requestID, "response")
				maybeWriteToolTruncationWarning(
					logger,
					debugLogDir,
					requestID,
					analysis,
					requestMaxTokens,
					requestMaxOutputTokens,
				)
			} else {
				var prettyJSON bytes.Buffer
				if err := json.Indent(&prettyJSON, recorder.body.Bytes(), "", "  "); err == nil {
					var respData map[string]any
					if err := json.Unmarshal(recorder.body.Bytes(), &respData); err == nil {
						if choices, ok := respData["choices"].([]any); ok && len(choices) > 0 {
							if choice, ok := choices[0].(map[string]any); ok {
								if message, ok := choice["message"].(map[string]any); ok {
									if toolCalls := message["tool_calls"]; toolCalls != nil {
										logger.Printf("[DEBUG-%s] response includes tool_calls", requestID)
									} else {
										logger.Printf("[DEBUG-%s] response includes no tool_calls", requestID)
									}
								}
								if finishReason, ok := choice["finish_reason"]; ok {
									logger.Printf("[DEBUG-%s] finish_reason=%v", requestID, finishReason)
								}
							}
						}
					}
					saveDebugFile(logger, respFilePath, prettyJSON.Bytes(), requestID, "response")
				} else {
					saveDebugFile(logger, respFilePath, recorder.body.Bytes(), requestID, "response")
				}
			}
		}

		logger.Printf("[DEBUG-%s] summary path=%s tools=%d messages=%d duration=%v", requestID, r.URL.Path, toolsCount, messagesCount, duration)
		logger.Printf("[DEBUG-%s] files=%s/%s_*.json|txt|log", requestID, debugLogDir, requestID)
		logger.Printf("[DEBUG-%s] === request end ===", requestID)
	})
}

func analyzeSSEDebug(raw string) sseDebugAnalysis {
	analysis := sseDebugAnalysis{
		toolCallArguments: make(map[int]string),
	}
	toolNameSet := map[string]struct{}{}

	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk map[string]any
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if usage, ok := chunk["usage"].(map[string]any); ok {
			if value, ok := jsonNumberToInt(usage["completion_tokens"]); ok {
				analysis.completionTokens = value
			}
		}

		choices, ok := chunk["choices"].([]any)
		if !ok || len(choices) == 0 {
			continue
		}
		choice, ok := choices[0].(map[string]any)
		if !ok {
			continue
		}
		if fr, ok := choice["finish_reason"].(string); ok && strings.TrimSpace(fr) != "" {
			analysis.finishReason = strings.TrimSpace(fr)
		}

		delta, ok := choice["delta"].(map[string]any)
		if !ok {
			continue
		}
		toolCalls, ok := delta["tool_calls"].([]any)
		if !ok || len(toolCalls) == 0 {
			continue
		}

		analysis.hasToolCalls = true
		for _, rawToolCall := range toolCalls {
			toolCallMap, ok := rawToolCall.(map[string]any)
			if !ok {
				continue
			}
			toolIndex := 0
			if value, ok := jsonNumberToInt(toolCallMap["index"]); ok && value >= 0 {
				toolIndex = value
			}

			if fn, ok := toolCallMap["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok && strings.TrimSpace(name) != "" {
					toolNameSet[strings.TrimSpace(name)] = struct{}{}
				}
				if argsPart, ok := fn["arguments"].(string); ok && argsPart != "" {
					analysis.toolCallArguments[toolIndex] += argsPart
				}
			}
		}
	}

	analysis.toolCallNames = make([]string, 0, len(toolNameSet))
	for name := range toolNameSet {
		analysis.toolCallNames = append(analysis.toolCallNames, name)
	}
	sort.Strings(analysis.toolCallNames)
	return analysis
}

func maybeWriteToolTruncationWarning(
	logger *log.Logger,
	debugLogDir string,
	requestID string,
	analysis sseDebugAnalysis,
	requestMaxTokens int,
	requestMaxOutputTokens int,
) {
	if !analysis.hasToolCalls || strings.TrimSpace(analysis.finishReason) != "length" {
		return
	}

	invalidArgs := make([]string, 0)
	for index, args := range analysis.toolCallArguments {
		trimmed := strings.TrimSpace(args)
		if trimmed == "" {
			invalidArgs = append(invalidArgs, fmt.Sprintf("tool_call[%d]: empty arguments", index))
			continue
		}
		var parsed any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			preview := trimmed
			if len(preview) > 160 {
				preview = preview[:160] + "..."
			}
			invalidArgs = append(invalidArgs, fmt.Sprintf("tool_call[%d]: invalid JSON (%v), preview=%q", index, err, preview))
		}
	}

	lines := []string{
		"tool-call truncation warning",
		fmt.Sprintf("request_id: %s", requestID),
		fmt.Sprintf("finish_reason: %s", analysis.finishReason),
		fmt.Sprintf("completion_tokens: %d", analysis.completionTokens),
		fmt.Sprintf("request.max_tokens: %d", requestMaxTokens),
		fmt.Sprintf("request.max_output_tokens: %d", requestMaxOutputTokens),
		fmt.Sprintf("tool_calls: %v", analysis.toolCallNames),
		"diagnosis: model output hit token limit while streaming tool arguments.",
		"recommendation: increase MIN_TOOL_MAX_OUTPUT_TOKENS (and/or request max_tokens) to avoid truncated tool JSON.",
	}
	if len(invalidArgs) == 0 {
		lines = append(lines, "argument_integrity: no JSON parse errors detected in collected chunks")
	} else {
		lines = append(lines, "argument_integrity: detected invalid/truncated tool arguments")
		lines = append(lines, invalidArgs...)
	}

	content := strings.Join(lines, "\n") + "\n"
	warningFilePath := filepath.Join(debugLogDir, fmt.Sprintf("%s_warning.log", requestID))
	saveDebugFile(logger, warningFilePath, []byte(content), requestID, "warning")

	logger.Printf(
		"[WARN-%s] finish_reason=length with tool_calls; warning file written: %s",
		requestID,
		warningFilePath,
	)
}

func jsonNumberToInt(raw any) (int, bool) {
	switch value := raw.(type) {
	case float64:
		return int(value), true
	case float32:
		return int(value), true
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case json.Number:
		v, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}

func saveDebugFile(logger *log.Logger, filePath string, data []byte, requestID, dataType string) {
	if err := os.WriteFile(filePath, data, 0o644); err != nil {
		logger.Printf("[DEBUG-%s] warning: failed to save %s to %s: %v", requestID, dataType, filePath, err)
		return
	}
	logger.Printf("[DEBUG-%s] %s saved to %s (%d bytes)", requestID, dataType, filePath, len(data))
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
