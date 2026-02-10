# 快速诊断：Cursor Agent 模式工具调用问题

## 你的情况

✅ 已配置 AWS Bedrock 代理到 Cursor
✅ 已启用 Cursor Agent 模式
❌ 模型返回"操作完成，请查看 cursor 要求"而不是实际调用工具

---

## 立即诊断（5分钟）

### 第 1 步：启用调试日志

编辑 `.env` 文件，添加：

```bash
DEBUG_REQUESTS=true
```

### 第 2 步：重启服务

```bash
# 停止当前服务 (Ctrl+C)
# 重新启动
go run ./cmd/server
```

### 第 3 步：在 Cursor 中发起请求

在 Cursor Agent 模式下发送一个需要工具的请求，例如：
- "读取 README.md 文件的内容"
- "搜索代码中的 TODO 注释"
- "修改 main.go 文件"

### 第 4 步：查看日志输出

在服务器控制台中查找：

#### 场景 A：Cursor 发送了工具定义

```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ⚠️ tool_choice: auto
```

然后查看响应：

**如果看到：**
```
[DEBUG-xxx] ✓ 响应包含工具调用!
[DEBUG-xxx] finish_reason: tool_calls
```

✅ **代理工作正常！** 问题在 Cursor 端。

**解决方案：**
1. 检查 Cursor 开发者工具（Help → Toggle Developer Tools → Console）
2. 查看是否有 JavaScript 错误
3. 尝试重启 Cursor
4. 更新 Cursor 到最新版本

**如果看到：**
```
[DEBUG-xxx] ⚠️ 响应不包含工具调用
[DEBUG-xxx] ⚠️ 模型返回了文本: "操作完成..."
[DEBUG-xxx] finish_reason: stop
```

⚠️ **模型选择不使用工具**

**可能原因：**
1. **工具定义不清晰** - 查看日志中的工具定义，检查描述是否准确
2. **模型误解了任务** - Claude 认为不需要使用工具
3. **提示词问题** - Cursor 发送的系统提示可能有问题

**解决方案：**
- 继续到"高级诊断"部分

#### 场景 B：Cursor 没有发送工具定义

```
[DEBUG-xxx] ⚠️ 请求不包含 tools 参数
```

❌ **Cursor 配置问题**

**解决方案：**

1. **确认 Cursor 设置**
   - 打开 Cursor 设置 → Models
   - 确认使用的是自定义端点
   - Base URL: `http://localhost:8080/v1`
   - 确认 API Key 正确

2. **确认 Agent 模式正确启用**
   - 使用 Cmd/Ctrl + I 打开 Composer
   - 或使用 Cmd/Ctrl + K 打开 Agent
   - 不要使用普通聊天窗口

3. **检查 Cursor 版本**
   - Cursor → About
   - 版本应该 >= 0.40.0
   - 如果过旧，更新到最新版本

---

## 高级诊断

### 查看完整的请求内容

如果启用了 `DEBUG_REQUESTS=true`，日志会显示完整的请求 JSON。

查找以下内容：

#### 1. 检查工具定义

```json
"tools": [
  {
    "type": "function",
    "function": {
      "name": "read_file",
      "description": "Read the contents of a file",
      "parameters": { ... }
    }
  }
]
```

**问题检查：**
- ✅ 工具名称是否清晰？
- ✅ 描述是否准确说明了工具的用途？
- ✅ 参数定义是否完整？

#### 2. 检查系统提示

查找日志中的系统消息：

```json
{
  "role": "system",
  "content": "You are a helpful assistant..."
}
```

**问题检查：**
- ⚠️ 系统提示是否告诉模型"不要实际执行操作"？
- ⚠️ 系统提示是否说"只需要说明如何操作"？
- ⚠️ 系统提示是否与工具使用冲突？

#### 3. 检查用户消息

```json
{
  "role": "user",
  "content": "读取 README.md 文件"
}
```

**问题检查：**
- ✅ 指令是否明确？
- ✅ 是否需要使用工具才能完成？

---

## 测试代理功能

### 使用测试脚本验证

**Windows:**
```powershell
.\test_tool_calling.ps1 -ApiKey "your-api-key"
```

**Linux/Mac:**
```bash
chmod +x test_tool_calling.sh
API_KEY="your-api-key" ./test_tool_calling.sh
```

如果测试脚本显示 ✓ 工具调用成功，说明：
- ✅ 代理功能正常
- ❌ 问题在 Cursor 或 Cursor 发送的请求

---

## 已知问题和解决方案

### 问题 1：Claude 模型过于"谨慎"

**症状：** 模型返回"我会帮你操作"而不是实际调用工具

**原因：** Claude 模型有时会选择描述操作而不是执行操作

**解决方案：**

1. **使用 tool_choice: "required"**

   这需要修改 Cursor 的行为，但我们可以在代理端强制：

   编辑 `internal/bedrockproxy/service.go`，在 `buildToolConfiguration` 函数中添加：

   ```go
   // 如果有工具但没有指定 tool_choice，默认使用 required
   if len(bedrockTools) > 0 && toolChoice == nil {
       cfg.ToolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
   }
   ```

2. **检查模型版本**

   某些 Claude 模型版本对工具调用的支持更好：
   - ✅ 推荐：`anthropic.claude-3-5-sonnet-20241022-v2:0`（最新）
   - ✅ 推荐：`anthropic.claude-3-5-sonnet-20240620-v1:0`
   - ⚠️ 较旧：`anthropic.claude-3-sonnet-20240229-v1:0`

### 问题 2：Cursor 发送的工具定义不完整

**症状：** 日志显示工具定义缺少关键信息

**解决方案：** 这是 Cursor 的问题，需要等待 Cursor 更新

**临时方案：** 使用其他支持工具调用的客户端测试，例如：
- Continue.dev
- Cline (VSCode 扩展)
- 直接使用 API

### 问题 3：响应格式不兼容

**症状：** 代理返回工具调用，但 Cursor 不识别

**诊断：** 查看日志中的响应格式，确认是否符合 OpenAI 标准

**标准格式：**
```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": null,
      "tool_calls": [{
        "id": "call_xxx",
        "type": "function",
        "function": {
          "name": "tool_name",
          "arguments": "{\"param\":\"value\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

---

## 强制工具调用补丁

如果你想强制模型在有工具时必须使用工具，可以应用以下补丁：

### 补丁文件：force_tool_use.patch

创建文件 `force_tool_use.patch`：

```diff
--- a/internal/bedrockproxy/service.go
+++ b/internal/bedrockproxy/service.go
@@ -579,6 +579,11 @@ func buildToolConfiguration(tools []openai.Tool, rawToolChoice json.RawMessage)
 	cfg := &brtypes.ToolConfiguration{
 		Tools: bedrockTools,
 	}
+	// 强制使用工具：如果有工具但没有指定 tool_choice，默认使用 any (required)
+	if toolChoice == nil {
+		toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
+	}
+
 	if toolChoice != nil {
 		cfg.ToolChoice = toolChoice
 	}
```

应用补丁：

```bash
# 备份原文件
cp internal/bedrockproxy/service.go internal/bedrockproxy/service.go.backup

# 应用补丁（手动编辑或使用 patch 命令）
# 在 service.go 的第 582 行后添加上述代码
```

**注意：** 这会强制模型在有工具时必须使用工具，可能导致某些情况下的不当行为。

---

## 查看实际数据

### 查看数据库中的请求

```bash
# 查看最新请求的完整内容
sqlite3 ./data/router.db "SELECT request_content FROM call_logs ORDER BY created_at DESC LIMIT 1;" | jq .

# 查看最新响应的完整内容
sqlite3 ./data/router.db "SELECT response_content FROM call_logs ORDER BY created_at DESC LIMIT 1;" | jq .

# 查看包含工具的请求
sqlite3 ./data/router.db "SELECT id, substr(request_content, 1, 200) FROM call_logs WHERE request_content LIKE '%tools%' ORDER BY created_at DESC LIMIT 3;"
```

### 导出日志用于分析

```bash
# 导出最近 10 条请求
sqlite3 ./data/router.db "SELECT json_object('id', id, 'request', json(request_content), 'response', json(response_content)) FROM call_logs ORDER BY created_at DESC LIMIT 10;" > recent_logs.json
```

---

## 下一步行动

### 如果代理测试通过，但 Cursor 仍然不工作：

1. **检查 Cursor 日志**
   - Help → Toggle Developer Tools → Console
   - 查找错误信息

2. **尝试其他客户端**
   - Continue.dev
   - Cline
   - 直接 API 调用

3. **联系 Cursor 支持**
   - 提供调试日志
   - 说明你使用的是 OpenAI 兼容端点

### 如果代理测试失败：

1. **检查 AWS 配置**
   - 确认 AWS 凭证正确
   - 确认有权限访问 Bedrock
   - 确认模型已启用

2. **检查模型支持**
   - 不是所有 Bedrock 模型都支持工具调用
   - 确认使用的是 Claude 3 或更新版本

3. **查看错误日志**
   - 检查服务器日志中的错误信息
   - 检查 AWS Bedrock 错误

---

## 总结

**立即执行：**

1. ✅ 添加 `DEBUG_REQUESTS=true` 到 `.env`
2. ✅ 重启服务
3. ✅ 在 Cursor Agent 中发起请求
4. ✅ 查看日志输出
5. ✅ 运行测试脚本验证

**根据日志结果：**

- 如果请求包含 tools 且响应包含 tool_calls → Cursor 端问题
- 如果请求包含 tools 但响应是文本 → 模型选择问题（考虑强制补丁）
- 如果请求不包含 tools → Cursor 配置问题

**需要帮助？**

提供以下信息：
1. 调试日志（完整的一次请求/响应）
2. Cursor 版本
3. 使用的模型 ID
4. 测试脚本的输出
