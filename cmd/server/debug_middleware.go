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
	"strings"
	"sync/atomic"
	"time"
)

var debugRequestCounter uint64

// 调试中间件 - 记录所有请求和响应的详细信息
// 设置环境变量 DEBUG_REQUESTS=true 启用
// 设置环境变量 DEBUG_LOG_DIR=./debug_logs 指定日志目录（可选）
func debugMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	debugEnabled := os.Getenv("DEBUG_REQUESTS") == "true"
	debugLogDir := os.Getenv("DEBUG_LOG_DIR")
	if debugLogDir == "" {
		debugLogDir = "./debug_logs"
	}

	// 创建日志目录
	if debugEnabled {
		if err := os.MkdirAll(debugLogDir, 0755); err != nil {
			logger.Printf("警告: 无法创建调试日志目录 %s: %v", debugLogDir, err)
		}
	}

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
		counter := atomic.AddUint64(&debugRequestCounter, 1)
		requestID := fmt.Sprintf("%s_%04d", time.Now().Format("20060102_150405"), counter)

		// 记录请求头
		logger.Printf("[DEBUG-%s] === 新请求 ===", requestID)
		logger.Printf("[DEBUG-%s] Method: %s", requestID, r.Method)
		logger.Printf("[DEBUG-%s] Path: %s", requestID, r.URL.Path)
		logger.Printf("[DEBUG-%s] Headers:", requestID)
		
		headersForFile := make(map[string]string)
		for name, values := range r.Header {
			// 隐藏敏感信息
			if strings.ToLower(name) == "authorization" || strings.ToLower(name) == "x-api-key" {
				logger.Printf("[DEBUG-%s]   %s: [REDACTED]", requestID, name)
				headersForFile[name] = "[REDACTED]"
			} else {
				logger.Printf("[DEBUG-%s]   %s: %s", requestID, name, strings.Join(values, ", "))
				headersForFile[name] = strings.Join(values, ", ")
			}
		}

		// 读取并记录请求体
		var requestBody []byte
		var toolsCount int
		var toolChoice interface{}
		var messagesCount int
		
		if r.Body != nil {
			requestBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(requestBody))

			if len(requestBody) > 0 {
				// 解析请求数据
				var reqData map[string]interface{}
				if err := json.Unmarshal(requestBody, &reqData); err == nil {
					if tools, ok := reqData["tools"]; ok {
						if toolsArray, ok := tools.([]interface{}); ok {
							toolsCount = len(toolsArray)
							logger.Printf("[DEBUG-%s] ⚠️ 请求包含 %d 个工具定义", requestID, toolsCount)
							
							// 列出工具名称
							for i, tool := range toolsArray {
								if toolMap, ok := tool.(map[string]interface{}); ok {
									if fn, ok := toolMap["function"].(map[string]interface{}); ok {
										if name, ok := fn["name"].(string); ok {
											logger.Printf("[DEBUG-%s]   工具 %d: %s", requestID, i+1, name)
										}
									}
								}
							}
						}
					} else {
						logger.Printf("[DEBUG-%s] ⚠️ 请求不包含 tools 参数", requestID)
					}

					if tc, ok := reqData["tool_choice"]; ok {
						toolChoice = tc
						logger.Printf("[DEBUG-%s] ⚠️ tool_choice: %v", requestID, toolChoice)
					}
					
					if messages, ok := reqData["messages"].([]interface{}); ok {
						messagesCount = len(messages)
						logger.Printf("[DEBUG-%s] 消息数量: %d", requestID, messagesCount)
					}
					
					if model, ok := reqData["model"].(string); ok {
						logger.Printf("[DEBUG-%s] 模型: %s", requestID, model)
					}
					
					if stream, ok := reqData["stream"].(bool); ok {
						logger.Printf("[DEBUG-%s] 流式: %v", requestID, stream)
					}
				}

				// 保存完整请求到文件
				reqFilePath := filepath.Join(debugLogDir, fmt.Sprintf("%s_request.json", requestID))
				saveDebugFile(logger, reqFilePath, requestBody, requestID, "请求")
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
			// 保存完整响应到文件
			respFilePath := filepath.Join(debugLogDir, fmt.Sprintf("%s_response.txt", requestID))
			
			// 对于流式响应，检查是否包含工具调用
			if strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") {
				logger.Printf("[DEBUG-%s] 流式响应，共 %d 字节", requestID, recorder.body.Len())
				
				// 解析流式响应，检查工具调用
				lines := strings.Split(recorder.body.String(), "\n")
				hasToolCalls := false
				var finishReason string
				toolCallNames := []string{}
				
				for _, line := range lines {
					if strings.HasPrefix(line, "data: ") {
						data := strings.TrimPrefix(line, "data: ")
						if data == "[DONE]" {
							continue
						}
						
						var chunk map[string]interface{}
						if err := json.Unmarshal([]byte(data), &chunk); err == nil {
							if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
								if choice, ok := choices[0].(map[string]interface{}); ok {
									// 检查 finish_reason
									if fr, ok := choice["finish_reason"].(string); ok && fr != "" {
										finishReason = fr
									}
									
									// 检查 delta 中的 tool_calls
									if delta, ok := choice["delta"].(map[string]interface{}); ok {
										if toolCalls, ok := delta["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
											hasToolCalls = true
											for _, tc := range toolCalls {
												if tcMap, ok := tc.(map[string]interface{}); ok {
													if fn, ok := tcMap["function"].(map[string]interface{}); ok {
														if name, ok := fn["name"].(string); ok && name != "" {
															toolCallNames = append(toolCallNames, name)
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
				
				if hasToolCalls {
					logger.Printf("[DEBUG-%s] ✓ 流式响应包含工具调用!", requestID)
					if len(toolCallNames) > 0 {
						logger.Printf("[DEBUG-%s] ✓ 调用的工具: %v", requestID, toolCallNames)
					}
				} else {
					logger.Printf("[DEBUG-%s] ⚠️ 流式响应不包含工具调用", requestID)
				}
				
				if finishReason != "" {
					logger.Printf("[DEBUG-%s] finish_reason: %s", requestID, finishReason)
				}
				
				// 保存流式响应
				saveDebugFile(logger, respFilePath, recorder.body.Bytes(), requestID, "响应")
				
			} else {
				// 非流式响应
				var prettyJSON bytes.Buffer
				if err := json.Indent(&prettyJSON, recorder.body.Bytes(), "", "  "); err == nil {
					// 检查响应中的工具调用
					var respData map[string]interface{}
					if err := json.Unmarshal(recorder.body.Bytes(), &respData); err == nil {
						if choices, ok := respData["choices"].([]interface{}); ok && len(choices) > 0 {
							if choice, ok := choices[0].(map[string]interface{}); ok {
								if message, ok := choice["message"].(map[string]interface{}); ok {
									if toolCalls, ok := message["tool_calls"]; ok && toolCalls != nil {
										logger.Printf("[DEBUG-%s] ✓ 响应包含工具调用!", requestID)
										if tcArray, ok := toolCalls.([]interface{}); ok {
											for _, tc := range tcArray {
												if tcMap, ok := tc.(map[string]interface{}); ok {
													if fn, ok := tcMap["function"].(map[string]interface{}); ok {
														if name, ok := fn["name"].(string); ok {
															logger.Printf("[DEBUG-%s] ✓ 调用工具: %s", requestID, name)
														}
													}
												}
											}
										}
									} else {
										logger.Printf("[DEBUG-%s] ⚠️ 响应不包含工具调用", requestID)
										if content, ok := message["content"]; ok {
											contentStr := fmt.Sprintf("%v", content)
											if len(contentStr) > 200 {
												contentStr = contentStr[:200] + "..."
											}
											logger.Printf("[DEBUG-%s] ⚠️ 模型返回了文本: %s", requestID, contentStr)
										}
									}

									if finishReason, ok := choice["finish_reason"]; ok {
										logger.Printf("[DEBUG-%s] finish_reason: %v", requestID, finishReason)
									}
								}
							}
						}
					}
					
					// 保存格式化的 JSON 响应
					saveDebugFile(logger, respFilePath, prettyJSON.Bytes(), requestID, "响应")
				} else {
					saveDebugFile(logger, respFilePath, recorder.body.Bytes(), requestID, "响应")
				}
			}
		}

		// 输出摘要
		logger.Printf("[DEBUG-%s] === 请求摘要 ===", requestID)
		logger.Printf("[DEBUG-%s] 路径: %s | 工具数: %d | 消息数: %d | 耗时: %v", 
			requestID, r.URL.Path, toolsCount, messagesCount, duration)
		logger.Printf("[DEBUG-%s] 日志文件: %s/%s_*.json/txt", requestID, debugLogDir, requestID)
		logger.Printf("[DEBUG-%s] === 请求结束 ===\n", requestID)
	})
}

// saveDebugFile 保存调试数据到文件
func saveDebugFile(logger *log.Logger, filePath string, data []byte, requestID, dataType string) {
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		logger.Printf("[DEBUG-%s] 警告: 无法保存%s到文件 %s: %v", requestID, dataType, filePath, err)
	} else {
		logger.Printf("[DEBUG-%s] %s已保存到: %s (%d 字节)", requestID, dataType, filePath, len(data))
	}
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
