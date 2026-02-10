# 🚀 Cursor 工具调用 - 快速参考

## 问题：模型返回文本而不是调用工具

### ⚡ 3 步快速诊断

```bash
# 1. 启用调试
echo DEBUG_REQUESTS=true >> .env

# 2. 重启服务
go run ./cmd/server

# 3. 在 Cursor 中测试，然后查看日志
```

---

## 📋 日志判断

### ✅ 看到这个 → 代理正常，Cursor 端问题
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ✓ 响应包含工具调用!
```
**解决：** 重启 Cursor，更新 Cursor，检查开发者工具

### ⚠️ 看到这个 → 模型不使用工具
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ⚠️ 响应不包含工具调用
```
**解决：** 运行 `.\apply_force_tool_patch.ps1`

### ❌ 看到这个 → Cursor 配置问题
```
[DEBUG-xxx] ⚠️ 请求不包含 tools 参数
```
**解决：** 检查 Cursor 设置，确保使用 Agent 模式（Cmd/Ctrl+I）

---

## 🔧 快速修复

### 方案 1：强制工具调用（推荐）
```powershell
# 应用补丁
.\apply_force_tool_patch.ps1

# 重启服务
go run ./cmd/server

# 测试
.\test_tool_calling.ps1 -ApiKey "your-key"
```

### 方案 2：验证代理功能
```powershell
# 运行测试
.\test_tool_calling.ps1 -ApiKey "your-key"

# 如果测试通过 → 问题在 Cursor
# 如果测试失败 → 问题在代理配置
```

---

## 📱 Cursor 配置检查

```
Settings → Models → Custom
├─ Base URL: http://localhost:8080/v1
├─ API Key: [从管理面板获取]
└─ Model: anthropic.claude-3-5-sonnet-20240620-v1:0

使用方式：
├─ ✅ Cmd/Ctrl + I (Composer/Agent)
└─ ❌ Cmd/Ctrl + L (普通聊天)
```

---

## 🧪 测试命令

```bash
# 健康检查
curl http://localhost:8080/healthz

# 工具调用测试
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20240620-v1:0",
    "messages": [{"role": "user", "content": "天气如何？"}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "获取天气",
        "parameters": {
          "type": "object",
          "properties": {"location": {"type": "string"}},
          "required": ["location"]
        }
      }
    }]
  }'
```

---

## 📊 期望结果

### 正常的工具调用响应
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
          "name": "get_weather",
          "arguments": "{\"location\":\"Tokyo\"}"
        }
      }]
    },
    "finish_reason": "tool_calls"
  }]
}
```

### ❌ 错误的响应（返回文本）
```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "操作完成，请查看 cursor 要求",
      "tool_calls": null
    },
    "finish_reason": "stop"
  }]
}
```

---

## 🔍 数据库查询

```bash
# 查看最新请求
sqlite3 ./data/router.db "SELECT substr(request_content,1,200) FROM call_logs ORDER BY created_at DESC LIMIT 1;"

# 查看包含工具的请求
sqlite3 ./data/router.db "SELECT id FROM call_logs WHERE request_content LIKE '%tools%' ORDER BY created_at DESC LIMIT 5;"

# 导出完整日志
sqlite3 ./data/router.db "SELECT request_content, response_content FROM call_logs ORDER BY created_at DESC LIMIT 1;" > last_request.json
```

---

## 🎯 关键文件

| 文件 | 用途 |
|------|------|
| `SOLUTION_GUIDE.md` | 完整解决方案（从这里开始） |
| `QUICK_DIAGNOSIS.md` | 快速诊断指南 |
| `TROUBLESHOOTING.md` | 详细故障排查 |
| `FORCE_TOOL_PATCH.md` | 补丁说明 |
| `test_tool_calling.ps1` | 测试脚本 |
| `apply_force_tool_patch.ps1` | 应用补丁脚本 |

---

## ⚙️ 环境变量

```bash
# .env 文件
DEBUG_REQUESTS=true          # 启用调试日志
AWS_REGION=us-east-1         # AWS 区域
DEFAULT_MODEL_ID=anthropic.claude-3-5-sonnet-20240620-v1:0
```

---

## 🆘 获取帮助

提供以下信息：
1. Cursor 版本（Cursor → About）
2. 调试日志（启用 DEBUG_REQUESTS=true）
3. 测试脚本输出
4. 数据库最新请求

---

## ✅ 成功标志

- [x] 测试脚本显示 "✓ 模型成功调用工具!"
- [x] 日志显示 "✓ 响应包含工具调用!"
- [x] Cursor 实际执行工具而不是返回文本
- [x] finish_reason 是 "tool_calls" 而不是 "stop"

---

## 🎉 一句话总结

**代理已支持工具调用，如果不工作：**
1. 启用 `DEBUG_REQUESTS=true` 查看日志
2. 如果 Cursor 发送了工具但模型不用 → 运行 `.\apply_force_tool_patch.ps1`
3. 如果 Cursor 没发送工具 → 检查 Cursor 配置和版本
4. 运行 `.\test_tool_calling.ps1` 验证代理功能

**立即开始：** 打开 `SOLUTION_GUIDE.md` 按步骤操作
