# 🎯 立即执行 - 3 步解决问题

## 问题说明

**你的情况：**
- ✅ Cursor Agent 模式已启用
- ✅ 代理已配置到 Cursor
- ❌ 模型返回"操作完成"而不是实际调用工具修改代码

**原因：**
Claude 模型选择描述操作而不是执行操作（当 `tool_choice: "auto"` 时）

**解决方案：**
强制模型使用工具（将 `auto` 改为 `required`）

---

## ⚡ 立即执行（5 分钟）

### 步骤 1：应用补丁（1 分钟）

打开 PowerShell，运行：

```powershell
cd D:\360MoveData\Users\Administrator\Desktop\AI-workspace\salessavvy\aws-cursor-router

.\apply_force_tool_patch.ps1
```

**期望输出：**
```
=== 应用强制工具调用补丁 ===

[1/3] 创建备份...
✓ 备份已创建: internal\bedrockproxy\service.go.backup

[2/3] 应用补丁...
✓ 补丁已应用

[3/3] 验证 Go 语法...
✓ 语法验证通过

=== 补丁应用成功 ===
```

### 步骤 2：重启服务（1 分钟）

```bash
# 停止当前服务（按 Ctrl+C）

# 重新启动
go run ./cmd/server
```

### 步骤 3：在 Cursor 中测试（3 分钟）

1. 打开 Cursor
2. 按 **Cmd/Ctrl + I** 打开 Composer（Agent 模式）
3. 发送请求：
   ```
   读取 README.md 文件并告诉我内容
   ```

**期望行为：**
- ✅ 模型调用 `read_file` 工具
- ✅ Cursor 显示工具执行过程
- ✅ 模型基于实际文件内容回答
- ❌ 不应该只返回"操作完成"

---

## 🔍 验证补丁效果

### 方法 1：查看 Cursor 行为

**补丁前：**
```
用户: 读取 README.md 文件
模型: 操作完成。
```

**补丁后：**
```
用户: 读取 README.md 文件
模型: [调用 read_file 工具]
Cursor: [显示文件内容]
模型: 根据文件内容，这个项目是...
```

### 方法 2：启用调试日志

在 `.env` 文件中添加：
```bash
DEBUG_REQUESTS=true
```

重启服务后，在 Cursor 中测试，查看日志：

**应该看到：**
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ✓ 响应包含工具调用!
[DEBUG-xxx] finish_reason: tool_calls
```

### 方法 3：运行测试脚本

```powershell
.\test_tool_calling.ps1 -ApiKey "your-api-key"
```

**期望输出：**
```
✓ 模型成功调用工具!
  Tool Call ID: call_xxx
  Function: get_weather
  Arguments: {"location":"San Francisco, CA"}
```

---

## 📝 补丁详解

### 修改的文件
`internal/bedrockproxy/service.go` 第 579-586 行

### 修改内容

**修改前：**
```go
cfg := &brtypes.ToolConfiguration{
    Tools: bedrockTools,
}
if toolChoice != nil {
    cfg.ToolChoice = toolChoice
}
return cfg, nil
```

**修改后：**
```go
cfg := &brtypes.ToolConfiguration{
    Tools: bedrockTools,
}

// 强制工具调用：将 auto 改为 required
if toolChoice == nil {
    toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
} else if _, isAuto := toolChoice.(*brtypes.ToolChoiceMemberAuto); isAuto {
    toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
}

if toolChoice != nil {
    cfg.ToolChoice = toolChoice
}
return cfg, nil
```

### 工作原理

```
Cursor 发送请求
    ↓
包含 tools 定义
    ↓
tool_choice: "auto" (模型可以选择不用工具)
    ↓
补丁拦截
    ↓
改为 tool_choice: "required" (模型必须用工具)
    ↓
发送给 Bedrock
    ↓
模型被强制调用工具
    ↓
返回 tool_calls 而不是文本
```

---

## 🔄 如果需要回滚

```powershell
.\apply_force_tool_patch.ps1 -Revert
```

这会恢复原始文件。

---

## ✅ 成功标志

当一切正常时，你会看到：

### 在 Cursor 中
- 模型实际调用工具（read_file, edit_file 等）
- Cursor 显示工具执行过程
- 模型基于工具结果继续对话
- **不会**只返回"操作完成"

### 在服务器日志中（如果启用了 DEBUG_REQUESTS）
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ⚠️ tool_choice: auto
[DEBUG-xxx] ✓ 响应包含工具调用!
[DEBUG-xxx]   Tool: read_file
[DEBUG-xxx]   Arguments: {"path":"README.md"}
[DEBUG-xxx] finish_reason: tool_calls
```

---

## 🎯 总结

**问题：** 模型返回"操作完成"而不是实际调用工具

**原因：** `tool_choice: "auto"` 让模型自己决定，Claude 选择不用工具

**解决：** 补丁将 `auto` 强制改为 `required`，模型必须使用工具

**操作：**
```powershell
# 1. 应用补丁
.\apply_force_tool_patch.ps1

# 2. 重启服务
go run ./cmd/server

# 3. 在 Cursor 中测试
# 按 Cmd/Ctrl + I，发送：读取 README.md 文件

# 4. 验证模型实际调用了工具
```

---

**立即执行这 3 步，问题就解决了！** 🚀
