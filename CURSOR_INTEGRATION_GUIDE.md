# Cursor 集成诊断指南

## 问题描述

当前代理已完整实现 OpenAI 工具调用协议，但 Cursor 可能没有正确发送工具定义，导致模型返回文本说明而不是实际调用工具。

## 诊断步骤

### 1. 检查请求日志

查看数据库中的请求日志，确认 Cursor 是否发送了 `tools` 参数：

```bash
# 查看最近的请求
sqlite3 ./data/router.db "SELECT id, model, request_content, response_content, created_at FROM call_logs ORDER BY created_at DESC LIMIT 5;"
```

### 2. 测试工具调用功能

使用以下 curl 命令测试代理是否正确处理工具调用：

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20240620-v1:0",
    "messages": [
      {
        "role": "user",
        "content": "What is the weather in San Francisco?"
      }
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get the current weather in a given location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "The city and state, e.g. San Francisco, CA"
              },
              "unit": {
                "type": "string",
                "enum": ["celsius", "fahrenheit"]
              }
            },
            "required": ["location"]
          }
        }
      }
    ],
    "tool_choice": "auto"
  }'
```

**期望响应：**

```json
{
  "id": "chatcmpl-...",
  "object": "chat.completion",
  "created": 1707547690,
  "model": "anthropic.claude-3-5-sonnet-20240620-v1:0",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": null,
      "tool_calls": [{
        "id": "call_1",
        "type": "function",
        "function": {
          "name": "get_weather",
          "arguments": "{\"location\":\"San Francisco, CA\",\"unit\":\"fahrenheit\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }],
  "usage": {
    "prompt_tokens": 150,
    "completion_tokens": 50,
    "total_tokens": 200
  }
}
```

### 3. 测试完整的工具调用循环

```bash
# 第一步：发送带工具的请求
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20240620-v1:0",
    "messages": [
      {"role": "user", "content": "What is the weather in San Francisco?"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get the current weather",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {"type": "string"}
            },
            "required": ["location"]
          }
        }
      }
    ]
  }' > response1.json

# 第二步：发送工具结果
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20240620-v1:0",
    "messages": [
      {"role": "user", "content": "What is the weather in San Francisco?"},
      {
        "role": "assistant",
        "content": null,
        "tool_calls": [{
          "id": "call_1",
          "type": "function",
          "function": {
            "name": "get_weather",
            "arguments": "{\"location\":\"San Francisco, CA\"}"
          }
        }]
      },
      {
        "role": "tool",
        "tool_call_id": "call_1",
        "content": "{\"temperature\": 72, \"condition\": \"sunny\"}"
      }
    ]
  }'
```

## Cursor 配置检查

### 当前配置

确认 Cursor 设置中的配置：

1. **Base URL**: `http://localhost:8080/v1` 或 `http://<server-ip>:8080/v1`
2. **API Key**: 从管理面板获取的客户端 API 密钥
3. **Model**: `anthropic.claude-3-5-sonnet-20240620-v1:0`

### Cursor 版本要求

Cursor 需要支持 OpenAI 工具调用协议。确保：

- Cursor 版本 >= 0.40.0（支持工具调用）
- 在 Cursor 设置中启用了 "Agent Mode" 或 "Composer"

## 常见问题

### 问题 1：模型返回文本而不是工具调用

**原因：** Cursor 可能没有发送 `tools` 参数，或者模型认为不需要使用工具。

**解决方案：**
1. 检查 Cursor 是否在 Agent/Composer 模式下运行
2. 使用 `tool_choice: "required"` 强制模型使用工具
3. 查看请求日志确认 tools 参数是否存在

### 问题 2：工具调用格式不正确

**原因：** 响应格式可能与 Cursor 期望的不匹配。

**解决方案：**
- 当前代理已实现标准 OpenAI 格式，应该兼容
- 检查 Cursor 日志查看具体错误

### 问题 3：工具结果无法发送回模型

**原因：** Cursor 可能不支持多轮工具调用。

**解决方案：**
- 确保 Cursor 版本支持完整的工具调用循环
- 尝试使用 `/v1/responses` 端点（更现代的协议）

## 启用详细日志

修改代码以添加更详细的日志记录：

### 方案 1：添加请求/响应日志中间件

在 `cmd/server/routes_public.go` 中添加日志：

```go
// 在 handleChatCompletions 函数开始处添加
log.Printf("[DEBUG] Received chat completion request: model=%s, stream=%v, tools_count=%d",
    request.Model, request.Stream, len(request.Tools))

if len(request.Tools) > 0 {
    toolsJSON, _ := json.MarshalIndent(request.Tools, "", "  ")
    log.Printf("[DEBUG] Tools: %s", string(toolsJSON))
}
```

### 方案 2：查看数据库日志

```sql
-- 查看包含工具调用的请求
SELECT
    id,
    client_id,
    model,
    substr(request_content, 1, 200) as request_preview,
    substr(response_content, 1, 200) as response_preview,
    input_tokens,
    output_tokens,
    datetime(created_at, 'unixepoch') as created_time
FROM call_logs
WHERE request_content LIKE '%tools%'
ORDER BY created_at DESC
LIMIT 10;
```

## 验证代理功能

运行单元测试确认工具调用功能正常：

```bash
cd D:\360MoveData\Users\Administrator\Desktop\AI-workspace\salessavvy\aws-cursor-router
go test ./internal/bedrockproxy -v -run TestBuildToolConfiguration
go test ./internal/bedrockproxy -v -run TestExtractOutputPayloadWithToolCalls
go test ./internal/bedrockproxy -v -run TestBuildBedrockMessagesWithToolUseAndToolResult
```

## 下一步

1. **运行诊断测试**：使用上面的 curl 命令测试工具调用
2. **检查日志**：查看数据库中的请求内容
3. **确认 Cursor 配置**：确保使用正确的端点和模型
4. **更新 Cursor**：如果版本过旧，升级到最新版本

## 技术支持

如果问题仍然存在，请提供：

1. Cursor 版本号
2. 请求日志（从数据库导出）
3. Cursor 错误日志（如果有）
4. curl 测试结果

这将帮助进一步诊断问题。
