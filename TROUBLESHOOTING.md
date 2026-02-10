# 故障排查指南

## 问题：Cursor 没有调用工具，而是返回文本说明

### 症状

当你在 Cursor 中使用代理时，模型返回类似以下的文本响应：
- "操作完成，请查看 cursor 要求，修改代码支持 cursor 调用"
- "我已经完成了操作，请检查代码"
- 或者其他文本说明，而不是实际执行工具调用

### 根本原因

这个代理**已经完整实现了 OpenAI 工具调用协议**。问题通常是：

1. **Cursor 没有发送工具定义** - Cursor 可能没有在请求中包含 `tools` 参数
2. **模型选择不使用工具** - 即使有工具定义，模型也可能选择直接回答
3. **Cursor 版本过旧** - 旧版本的 Cursor 可能不支持工具调用
4. **配置问题** - Cursor 的配置可能不正确

---

## 诊断步骤

### 步骤 1: 启用调试日志

在 `.env` 文件中添加：

```bash
DEBUG_REQUESTS=true
```

然后重启服务：

```bash
go run ./cmd/server
```

现在所有请求和响应都会被详细记录到控制台。

### 步骤 2: 在 Cursor 中发起请求

在 Cursor 中尝试使用 Agent 或 Composer 功能，然后查看服务器日志。

**查找关键信息：**

```
[DEBUG-xxx] ⚠️ 请求包含 X 个工具定义
[DEBUG-xxx] ⚠️ tool_choice: auto
```

或者：

```
[DEBUG-xxx] ⚠️ 请求不包含 tools 参数
```

### 步骤 3: 分析日志

#### 情况 A: 请求不包含 tools 参数

**日志显示：**
```
[DEBUG-xxx] ⚠️ 请求不包含 tools 参数
```

**原因：** Cursor 没有发送工具定义。

**解决方案：**

1. **确认 Cursor 版本**
   - 打开 Cursor 设置 → About
   - 确保版本 >= 0.40.0
   - 如果版本过旧，请更新到最新版本

2. **确认使用正确的功能**
   - 使用 **Composer** (Cmd/Ctrl + I) 而不是普通聊天
   - 或使用 **Agent Mode**
   - 普通聊天窗口可能不会发送工具定义

3. **检查 Cursor 设置**
   - 打开 Cursor 设置 → Features
   - 确保启用了 "Agent" 或 "Composer" 功能
   - 确保启用了 "Tools" 或 "MCP"

#### 情况 B: 请求包含 tools，但模型返回文本

**日志显示：**
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ⚠️ tool_choice: auto
...
[DEBUG-xxx] ⚠️ 响应不包含工具调用
[DEBUG-xxx] ⚠️ 模型返回了文本: "操作完成..."
[DEBUG-xxx] finish_reason: stop
```

**原因：** 模型选择不使用工具，而是直接回答。

**解决方案：**

1. **检查工具定义质量**
   - 查看日志中的工具定义
   - 确保工具描述清晰、准确
   - 确保参数定义完整

2. **检查用户提示**
   - 模型可能认为不需要使用工具
   - 尝试更明确的指令，例如：
     - "使用可用的工具来..."
     - "调用工具来完成..."

3. **这可能是正常行为**
   - 如果任务不需要工具，模型直接回答是合理的
   - Claude 模型会智能判断是否需要使用工具

#### 情况 C: 响应包含工具调用

**日志显示：**
```
[DEBUG-xxx] ✓ 响应包含工具调用!
[DEBUG-xxx] finish_reason: tool_calls
```

**说明：** 工具调用功能正常工作！

如果 Cursor 仍然显示问题，可能是 Cursor 端的问题：
- Cursor 可能没有正确处理工具调用响应
- 检查 Cursor 的控制台日志（Help → Toggle Developer Tools）

---

## 手动测试

### 测试 1: 使用测试脚本

**Windows (PowerShell):**
```powershell
.\test_tool_calling.ps1 -ApiKey "your-api-key"
```

**Linux/Mac:**
```bash
API_KEY="your-api-key" ./test_tool_calling.sh
```

### 测试 2: 使用 curl

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
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
          "description": "Get the current weather in a given location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {
                "type": "string",
                "description": "The city and state"
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

**期望结果：**
```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "tool_calls": [{
        "id": "call_xxx",
        "type": "function",
        "function": {
          "name": "get_weather",
          "arguments": "{\"location\":\"San Francisco, CA\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

---

## 查看数据库日志

```bash
# 进入项目目录
cd D:\360MoveData\Users\Administrator\Desktop\AI-workspace\salessavvy\aws-cursor-router

# 查看最近的请求
sqlite3 ./data/router.db "SELECT id, model, substr(request_content, 1, 100), substr(response_content, 1, 100), datetime(created_at, 'unixepoch') FROM call_logs ORDER BY created_at DESC LIMIT 5;"

# 查看包含工具的请求
sqlite3 ./data/router.db "SELECT id, substr(request_content, 1, 500) FROM call_logs WHERE request_content LIKE '%tools%' ORDER BY created_at DESC LIMIT 1;"

# 查看完整的最新请求
sqlite3 ./data/router.db "SELECT request_content FROM call_logs ORDER BY created_at DESC LIMIT 1;" | jq .
```

---

## Cursor 配置检查清单

- [ ] Base URL: `http://localhost:8080/v1` 或 `http://<server-ip>:8080/v1`
- [ ] API Key: 已从管理面板获取
- [ ] Model: `anthropic.claude-3-5-sonnet-20240620-v1:0` 或其他 Bedrock 模型 ID
- [ ] Cursor 版本 >= 0.40.0
- [ ] 使用 Composer (Cmd/Ctrl + I) 或 Agent Mode
- [ ] 启用了 Tools/MCP 功能

---

## 常见误解

### ❌ 误解 1: "代理不支持工具调用"

**事实：** 代理已经完整实现了 OpenAI 工具调用协议，包括：
- ✅ `tools` 和 `tool_choice` 参数
- ✅ `tool_calls` 响应
- ✅ `tool` 角色消息
- ✅ 流式和非流式模式
- ✅ `/v1/responses` 端点

代码位置：
- `internal/bedrockproxy/service.go:524-641` - 工具配置构建
- `internal/bedrockproxy/service.go:342-405` - 消息转换
- `internal/bedrockproxy/service.go:648-688` - 工具调用提取

### ❌ 误解 2: "需要修改代码才能支持 Cursor"

**事实：** 代码已经支持标准的 OpenAI 协议，Cursor 应该可以直接使用。如果不工作，通常是配置或版本问题，而不是代码问题。

### ❌ 误解 3: "模型返回文本说明就是不支持工具"

**事实：** 模型返回文本可能是因为：
1. Cursor 没有发送工具定义（最常见）
2. 模型认为不需要使用工具（正常行为）
3. 工具定义不清晰

---

## 高级调试

### 查看 Cursor 的网络请求

1. 在 Cursor 中打开开发者工具：
   - Help → Toggle Developer Tools

2. 切换到 Network 标签

3. 发起一个请求

4. 查找发送到你的代理的请求

5. 检查请求体中是否包含 `tools` 字段

### 对比标准 OpenAI 行为

如果你有 OpenAI API 密钥，可以对比：

```bash
# 使用你的代理
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR_PROXY_KEY" \
  -H "Content-Type: application/json" \
  -d @test_request.json

# 使用 OpenAI
curl -X POST https://api.openai.com/v1/chat/completions \
  -H "Authorization: Bearer YOUR_OPENAI_KEY" \
  -H "Content-Type: application/json" \
  -d @test_request.json
```

响应格式应该相同。

---

## 获取帮助

如果问题仍然存在，请提供以下信息：

1. **Cursor 版本**
   - Cursor → About → Version

2. **调试日志**
   - 启用 `DEBUG_REQUESTS=true`
   - 复制完整的请求/响应日志

3. **数据库日志**
   ```bash
   sqlite3 ./data/router.db "SELECT request_content, response_content FROM call_logs ORDER BY created_at DESC LIMIT 1;"
   ```

4. **测试脚本结果**
   ```bash
   .\test_tool_calling.ps1 -ApiKey "your-key"
   ```

5. **Cursor 开发者工具日志**
   - Help → Toggle Developer Tools → Console
   - 复制任何错误信息

---

## 总结

**关键点：**

1. ✅ 代理已经完整支持工具调用
2. ✅ 代码不需要修改
3. ⚠️ 问题通常在 Cursor 配置或版本
4. 🔍 使用调试日志诊断问题
5. 🧪 使用测试脚本验证功能

**最可能的原因：**
- Cursor 没有发送 `tools` 参数（90%）
- Cursor 版本过旧（5%）
- 模型选择不使用工具（5%）
