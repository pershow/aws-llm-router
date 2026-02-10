# Cursor 工具调用问题 - 完整解决方案

## 📋 问题总结

**你的情况：**
- ✅ AWS Bedrock 代理已配置到 Cursor
- ✅ Cursor Agent 模式已启用
- ❌ 模型返回"操作完成，请查看 cursor 要求"而不是实际调用工具

**根本原因：**
代理已经完整实现了工具调用功能，但可能存在以下情况之一：
1. Cursor 没有发送工具定义（配置问题）
2. Cursor 发送了工具，但模型选择不使用（模型行为问题）
3. 响应格式问题（兼容性问题）

---

## 🚀 立即执行（按顺序）

### 步骤 1：启用调试日志（2分钟）

1. 打开 `.env` 文件
2. 添加或修改：
   ```bash
   DEBUG_REQUESTS=true
   ```
3. 保存文件

### 步骤 2：重启服务（1分钟）

```bash
# 停止当前服务（按 Ctrl+C）
# 重新启动
go run ./cmd/server
```

### 步骤 3：在 Cursor 中测试（2分钟）

1. 打开 Cursor
2. 使用 **Cmd/Ctrl + I** 打开 Composer（Agent 模式）
3. 发送一个需要工具的请求，例如：
   ```
   读取 README.md 文件的内容
   ```
4. 观察模型的响应

### 步骤 4：查看服务器日志（3分钟）

在服务器控制台中查找关键信息：

#### 场景 A：Cursor 发送了工具 ✅

```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ⚠️ tool_choice: auto
```

**然后查看响应部分：**

**情况 A1：响应包含工具调用** ✅
```
[DEBUG-xxx] ✓ 响应包含工具调用!
[DEBUG-xxx] finish_reason: tool_calls
```

→ **代理工作正常！问题在 Cursor 端**
→ 跳转到"解决方案 A1"

**情况 A2：响应是文本** ⚠️
```
[DEBUG-xxx] ⚠️ 响应不包含工具调用
[DEBUG-xxx] ⚠️ 模型返回了文本: "操作完成..."
[DEBUG-xxx] finish_reason: stop
```

→ **模型选择不使用工具**
→ 跳转到"解决方案 A2"

#### 场景 B：Cursor 没有发送工具 ❌

```
[DEBUG-xxx] ⚠️ 请求不包含 tools 参数
```

→ **Cursor 配置问题**
→ 跳转到"解决方案 B"

---

## 🔧 解决方案

### 解决方案 A1：Cursor 端问题

**问题：** 代理返回了正确的工具调用，但 Cursor 没有执行

**步骤：**

1. **检查 Cursor 开发者工具**
   ```
   Help → Toggle Developer Tools → Console
   ```
   查看是否有 JavaScript 错误

2. **重启 Cursor**
   ```
   完全退出 Cursor 并重新打开
   ```

3. **更新 Cursor**
   ```
   Help → Check for Updates
   确保版本 >= 0.40.0
   ```

4. **尝试其他客户端**
   - 使用 Continue.dev 或 Cline 测试
   - 如果其他客户端工作正常，说明是 Cursor 的 bug

5. **联系 Cursor 支持**
   - 提供调试日志
   - 说明使用的是 OpenAI 兼容端点

### 解决方案 A2：强制工具调用

**问题：** 模型收到工具定义但选择不使用

**步骤：**

1. **应用强制工具调用补丁**
   ```powershell
   .\apply_force_tool_patch.ps1
   ```

2. **重启服务**
   ```bash
   go run ./cmd/server
   ```

3. **重新测试**
   - 在 Cursor 中再次发送请求
   - 查看日志确认模型现在调用工具

4. **如果仍然不工作**
   - 查看日志中的工具定义
   - 确认工具描述是否清晰
   - 尝试更明确的用户指令

**补丁说明：**
- 将 `tool_choice: auto` 改为 `tool_choice: required`
- 强制模型在有工具时必须使用工具
- 详见 `FORCE_TOOL_PATCH.md`

**回滚补丁：**
```powershell
.\apply_force_tool_patch.ps1 -Revert
```

### 解决方案 B：Cursor 配置问题

**问题：** Cursor 没有发送工具定义

**步骤：**

1. **确认 Cursor 设置**
   - 打开 Cursor 设置 → Models
   - 确认 Base URL: `http://localhost:8080/v1`
   - 确认 API Key 正确
   - 确认 Model ID 正确

2. **确认使用 Agent 模式**
   - 使用 **Cmd/Ctrl + I** 打开 Composer
   - 不要使用普通聊天窗口（Cmd/Ctrl + L）
   - Agent 模式才会发送工具定义

3. **检查 Cursor 版本**
   ```
   Cursor → About
   确保版本 >= 0.40.0
   ```

4. **检查 Cursor 功能设置**
   - 打开 Cursor 设置 → Features
   - 确保启用了 "Agent" 功能
   - 确保启用了 "Tools" 或 "MCP"

5. **重新配置端点**
   - 删除现有配置
   - 重新添加自定义端点
   - 确保所有字段正确

---

## 🧪 验证测试

### 测试 1：代理功能测试

**Windows:**
```powershell
.\test_tool_calling.ps1 -ApiKey "your-api-key"
```

**Linux/Mac:**
```bash
chmod +x test_tool_calling.sh
API_KEY="your-api-key" ./test_tool_calling.sh
```

**期望结果：**
```
✓ 服务正常运行
✓ 可用模型数量: X
✓ 响应成功
✓ 模型成功调用工具!
✓ 工具结果处理成功
✓ 强制工具调用成功!
```

**如果测试失败：**
- 检查 AWS 配置
- 检查 API Key
- 查看错误信息

**如果测试成功但 Cursor 不工作：**
- 问题在 Cursor 端
- 按照"解决方案 A1"操作

### 测试 2：手动 curl 测试

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "anthropic.claude-3-5-sonnet-20240620-v1:0",
    "messages": [
      {"role": "user", "content": "What is the weather in Tokyo?"}
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
  }' | jq .
```

**期望看到：**
```json
{
  "choices": [{
    "message": {
      "tool_calls": [...]
    },
    "finish_reason": "tool_calls"
  }]
}
```

---

## 📊 诊断流程图

```
开始
  ↓
启用 DEBUG_REQUESTS=true
  ↓
在 Cursor 中发送请求
  ↓
查看日志
  ↓
┌─────────────────────────────────┐
│ 请求包含 tools 参数？           │
└─────────────────────────────────┘
  ↓ 是                    ↓ 否
  ↓                       ↓
┌─────────────────┐   ┌──────────────────┐
│ 响应包含工具调用？│   │ Cursor 配置问题  │
└─────────────────┘   │ → 解决方案 B     │
  ↓ 是      ↓ 否      └──────────────────┘
  ↓         ↓
┌─────┐  ┌──────────────────┐
│Cursor│  │ 模型选择不使用工具│
│端问题│  │ → 解决方案 A2    │
│→A1  │  └──────────────────┘
└─────┘
```

---

## 📝 常见问题

### Q1: 补丁会影响正常使用吗？

**A:** 补丁会强制模型在有工具时必须使用工具。这在大多数情况下是期望的行为，但某些情况下模型可能会不必要地调用工具。如果遇到问题，可以随时回滚。

### Q2: 为什么模型会返回文本而不是调用工具？

**A:** Claude 模型会智能判断是否需要使用工具。如果模型认为可以直接回答，就不会调用工具。这是正常行为，但在 Agent 场景下通常不是期望的。

### Q3: 如何确认是代理问题还是 Cursor 问题？

**A:** 运行测试脚本。如果测试脚本显示工具调用成功，说明代理正常，问题在 Cursor 端。

### Q4: 支持哪些 Bedrock 模型？

**A:** 支持所有 Claude 3 及更新版本的模型，包括：
- `anthropic.claude-3-5-sonnet-20241022-v2:0` (推荐)
- `anthropic.claude-3-5-sonnet-20240620-v1:0`
- `anthropic.claude-3-sonnet-20240229-v1:0`
- `anthropic.claude-3-haiku-20240307-v1:0`

### Q5: 可以在生产环境使用吗？

**A:** 可以。代理已经实现了完整的功能，包括：
- 认证和授权
- 速率限制
- 并发控制
- 成本跟踪
- 请求日志

---

## 🎯 快速检查清单

在寻求帮助之前，请确认：

- [ ] 已启用 `DEBUG_REQUESTS=true`
- [ ] 已重启服务
- [ ] 已在 Cursor Agent 模式（Cmd/Ctrl + I）中测试
- [ ] 已查看服务器日志
- [ ] 已运行测试脚本 `test_tool_calling.ps1`
- [ ] Cursor 版本 >= 0.40.0
- [ ] Base URL 配置正确：`http://localhost:8080/v1`
- [ ] API Key 正确
- [ ] Model ID 正确

---

## 📞 获取帮助

如果问题仍然存在，请提供以下信息：

### 1. 环境信息
```bash
# Cursor 版本
Cursor → About → Version

# 代理版本
git log -1 --oneline

# Go 版本
go version
```

### 2. 配置信息
```bash
# .env 文件（隐藏敏感信息）
cat .env | grep -v "KEY\|SECRET\|TOKEN"
```

### 3. 调试日志
```bash
# 最近一次请求的完整日志
# 从服务器控制台复制 [DEBUG-xxx] 开头的所有行
```

### 4. 数据库日志
```bash
sqlite3 ./data/router.db "SELECT request_content, response_content FROM call_logs ORDER BY created_at DESC LIMIT 1;" | jq .
```

### 5. 测试结果
```bash
.\test_tool_calling.ps1 -ApiKey "your-key"
# 复制完整输出
```

---

## 📚 相关文档

- `README.md` - 项目概述和快速开始
- `QUICK_DIAGNOSIS.md` - 快速诊断指南
- `TROUBLESHOOTING.md` - 详细故障排查
- `FORCE_TOOL_PATCH.md` - 强制工具调用补丁说明
- `CURSOR_INTEGRATION_GUIDE.md` - Cursor 集成指南

---

## ✅ 成功标志

当一切正常工作时，你应该看到：

**服务器日志：**
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ⚠️ tool_choice: auto
...
[DEBUG-xxx] ✓ 响应包含工具调用!
[DEBUG-xxx] finish_reason: tool_calls
```

**Cursor 行为：**
- 模型调用工具（例如 read_file）
- 显示工具执行结果
- 基于结果继续对话
- 不会返回"操作完成，请查看..."之类的文本

**测试脚本：**
```
✓ 服务正常运行
✓ 模型成功调用工具!
✓ 工具结果处理成功
```

---

## 🎉 总结

1. **代理已经支持工具调用** - 代码不需要修改
2. **问题通常在配置或模型行为** - 不是代码问题
3. **调试日志是关键** - 启用后可以快速定位问题
4. **测试脚本可以验证** - 确认代理功能正常
5. **补丁可以强制工具使用** - 解决模型不调用工具的问题

**立即开始：**
```bash
# 1. 启用调试
echo "DEBUG_REQUESTS=true" >> .env

# 2. 重启服务
go run ./cmd/server

# 3. 在 Cursor 中测试

# 4. 查看日志并按照本文档操作
```

祝你成功！🚀
