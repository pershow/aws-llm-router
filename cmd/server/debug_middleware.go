package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// 调试中间件 - 记录所有请求和响应的详细信息
// 设置环境变量 DEBUG_REQUESTS=true 启用
func debugMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	debugEnabled := os.Getenv("DEBUG_REQUESTS") == "true"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !debugEnabled {
			next.ServeHTTP(w, r)
			return
		}

		// 只记录 API 请求
		if !strings.HasPrefix(r.URL.Path, "/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		startTime := time.Now()
		requestID := time.Now().Format("20060102150405.000")

		// 记录请求头
		logger.Printf("[DEBUG-%s] === 新请求 ===", requestID)
		logger.Printf("[DEBUG-%s] Method: %s", requestID, r.Method)
		logger.Printf("[DEBUG-%s] Path: %s", requestID, r.URL.Path)
		logger.Printf("[DEBUG-%s] Headers:", requestID)
		for name, values := range r.Header {
			// 隐藏敏感信息
			if strings.ToLower(name) == "authorization" || strings.ToLower(name) == "x-api-key" {
				logger.Printf("[DEBUG-%s]   %s: [REDACTED]", requestID, name)
			} else {
				logger.Printf("[DEBUG-%s]   %s: %s", requestID, name, strings.Join(values, ", "))
			}
		}

		// 读取并记录请求体
		var requestBody []byte
		if r.Body != nil {
			requestBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(requestBody))

			if len(requestBody) > 0 {
				logger.Printf("[DEBUG-%s] Request Body:", requestID)

				// 尝试格式化 JSON
				var prettyJSON bytes.Buffer
				if err := json.Indent(&prettyJSON, requestBody, "", "  "); err == nil {
					logger.Printf("[DEBUG-%s] %s", requestID, prettyJSON.String())

					// 特别标注工具相关信息
					var reqData map[string]interface{}
					if err := json.Unmarshal(requestBody, &reqData); err == nil {
						if tools, ok := reqData["tools"]; ok {
							if toolsArray, ok := tools.([]interface{}); ok {
								logger.Printf("[DEBUG-%s] ⚠️ 请求包含 %d 个工具定义", requestID, len(toolsArray))
							}
						} else {
							logger.Printf("[DEBUG-%s] ⚠️ 请求不包含 tools 参数", requestID)
						}

						if toolChoice, ok := reqData["tool_choice"]; ok {
							logger.Printf("[DEBUG-%s] ⚠️ tool_choice: %v", requestID, toolChoice)
						}
					}
				} else {
					logger.Printf("[DEBUG-%s] %s", requestID, string(requestBody))
				}
			}
		}

		// 包装 ResponseWriter 以捕获响应
		recorder := &responseRecorder{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
			body:           &bytes.Buffer{},
		}

		// 调用下一个处理器
		next.ServeHTTP(recorder, r)

		// 记录响应
		duration := time.Since(startTime)
		logger.Printf("[DEBUG-%s] Response Status: %d", requestID, recorder.statusCode)
		logger.Printf("[DEBUG-%s] Duration: %v", requestID, duration)

		if recorder.body.Len() > 0 {
			logger.Printf("[DEBUG-%s] Response Body:", requestID)

			// 对于流式响应，只记录前几行
			if strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
				lines := strings.Split(recorder.body.String(), "\n")
				maxLines := 20
				if len(lines) > maxLines {
					for i := 0; i < maxLines; i++ {
						logger.Printf("[DEBUG-%s]   %s", requestID, lines[i])
					}
					logger.Printf("[DEBUG-%s]   ... (%d more lines)", requestID, len(lines)-maxLines)
				} else {
					logger.Printf("[DEBUG-%s] %s", requestID, recorder.body.String())
				}
			} else {
				// 尝试格式化 JSON 响应
				var prettyJSON bytes.Buffer
				if err := json.Indent(&prettyJSON, recorder.body.Bytes(), "", "  "); err == nil {
					logger.Printf("[DEBUG-%s] %s", requestID, prettyJSON.String())

					// 检查响应中的工具调用
					var respData map[string]interface{}
					if err := json.Unmarshal(recorder.body.Bytes(), &respData); err == nil {
						if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
							if choice, ok := choices[0].(map[string]interface{}); ok {
								if message, ok := choice["message"].(map[string]interface{}); ok {
									if toolCalls, ok := message["tool_calls"]; ok && toolCalls != nil {
										logger.Printf("[DEBUG-%s] ✓ 响应包含工具调用!", requestID)
									} else {
										logger.Printf("[DEBUG-%s] ⚠️ 响应不包含工具调用", requestID)
										if content, ok := message["content"]; ok {
											logger.Printf("[DEBUG-%s] ⚠️ 模型返回了文本: %v", requestID, content)
										}
									}

									if finishReason, ok := choice["finish_reason"]; ok {
										logger.Printf("[DEBUG-%s] finish_reason: %v", requestID, finishReason)
									}
								}
							}
						}
					}
				} else {
					// 如果不是 JSON，直接输出（限制长度）
					bodyStr := recorder.body.String()
					if len(bodyStr) > 1000 {
						logger.Printf("[DEBUG-%s] %s... (truncated)", requestID, bodyStr[:1000])
					} else {
						logger.Printf("[DEBUG-%s] %s", requestID, bodyStr)
					}
				}
			}
		}

		logger.Printf("[DEBUG-%s] === 请求结束 ===\n", requestID)
	})
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
	// 记录响应体（但不影响实际写入）
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
