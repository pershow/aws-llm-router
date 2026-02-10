# ✅ 正确的解决方案 - 强制模型使用工具

## 🎯 问题澄清

**你的实际问题：**
- ✅ Cursor Agent 模式已启用
- ✅ Cursor 发送了工具定义（read_file, edit_file 等）
- ❌ 模型返回"操作完成"而不是实际调用工具
- ❌ 模型没有使用 Cursor 提供的工具来修改代码

**根本原因：**
Claude 模型收到工具定义后，选择返回文本描述而不是实际调用工具。这是模型的默认行为 - 当 `tool_choice` 是 `auto` 时，模型会自己判断是否需要使用工具。

---

## 🔧 解决方案：强制工具调用

### 方案 1：应用补丁（推荐）

这个补丁会将 `tool_choice: auto` 强制改为 `tool_choice: required`，确保模型必须使用工具。

#### 步骤 1：应用补丁

```powershell
# Windows
.\apply_force_tool_patch.ps1

# Linux/Mac
chmod +x apply_force_tool_patch.sh
./apply_force_tool_patch.sh
```

#### 步骤 2：重启服务

```bash
go run ./cmd/server
```

#### 步骤 3：在 Cursor 中测试

1. 打开 Cursor
2. 按 **Cmd/Ctrl + I** 打开 Composer
3. 发送请求：`读取 README.md 文件并告诉我内容`
4. 模型现在应该会调用 `read_file` 工具

---

## 📝 补丁做了什么

### 修改前的代码

```go
// internal/bedrockproxy/service.go
cfg := &brtypes.ToolConfiguration{
    Tools: bedrockTools,
}
if toolChoice != nil {
    cfg.ToolChoice = toolChoice
}
return cfg, nil
```

**问题：** 当 Cursor 发送 `tool_choice: "auto"` 时，模型可以选择不使用工具。

### 修改后的代码

```go
// internal/bedrockproxy/service.go
cfg := &brtypes.ToolConfiguration{
    Tools: bedrockTools,
}

// 强制工具调用：将 auto 改为 required
if toolChoice == nil {
    // 未指定 tool_choice，强制使用工具
    toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
} else if _, isAuto := toolChoice.(*brtypes.ToolChoiceMemberAuto); isAuto {
    // tool_choice 是 auto，改为 required（强制使用工具）
    toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
}

if toolChoice != nil {
    cfg.ToolChoice = toolChoice
}
return cfg, nil
```

**效果：** 模型收到工具定义后，必须使用其中一个工具，不能只返回文本。

---

## 🧪 验证补丁效果

### 测试 1：使用测试脚本

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

### 测试 2：在 Cursor 中测试

**发送请求：**
```
读取 README.md 文件的内容
```

**期望行为：**
1. 模型调用 `read_file` 工具
2. Cursor 执行工具并返回文件内容
3. 模型基于文件内容回答

**不应该看到：**
- "操作完成"
- "我已经读取了文件"
- "请查看文件内容"

### 测试 3：启用调试日志验证

```bash
# 在 .env 中添加
DEBUG_REQUESTS=true

# 重启服务
go run ./cmd/server
```

**在 Cursor 中发送请求后，查看日志：**

**应该看到：**
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ⚠️ tool_choice: auto
[DEBUG-xxx] ✓ 响应包含工具调用!
[DEBUG-xxx] finish_reason: tool_calls
```

**不应该看到：**
```
[DEBUG-xxx] ⚠️ 响应不包含工具调用
[DEBUG-xxx] finish_reason: stop
```

---

## 🎯 补丁的工作原理

### OpenAI 工具调用协议

```json
{
  "tools": [...],
  "tool_choice": "auto" | "required" | "none" | {"type": "function", "function": {"name": "..."}}
}
```

**tool_choice 选项：**
- `"auto"` - 模型自己决定是否使用工具（默认）
- `"required"` - 模型必须使用工具
- `"none"` - 禁用工具
- `{"type": "function", ...}` - 强制使用特定工具

### Bedrock 对应的配置

```go
// auto
&brtypes.ToolChoiceMemberAuto{Value: brtypes.AutoToolChoice{}}

// required (any)
&brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}

// specific tool
&brtypes.ToolChoiceMemberTool{Value: brtypes.SpecificToolChoice{Name: aws.String("tool_name")}}
```

### 补丁的转换逻辑

```
Cursor 发送: tool_choice: "auto"
    ↓
补丁拦截并转换
    ↓
发送给 Bedrock: tool_choice: "required" (any)
    ↓
模型必须使用工具
```

---

## ⚠️ 补丁的注意事项

### 优点
- ✅ 确保模型在有工具时必须使用工具
- ✅ 解决 Claude 返回文本描述的问题
- ✅ 提高 Cursor Agent 的可靠性
- ✅ 符合用户期望（Agent 应该执行操作而不是描述）

### 缺点
- ⚠️ 模型无法选择不使用工具
- ⚠️ 某些情况下可能导致不必要的工具调用
- ⚠️ 如果工具定义不完整，可能导致错误

### 适用场景
- ✅ Cursor Agent 模式（期望模型执行操作）
- ✅ 代码编辑场景（需要实际修改文件）
- ✅ 文件操作场景（需要读取/写入文件）

### 不适用场景
- ❌ 纯对话场景（不需要工具）
- ❌ 工具定义不完整的情况
- ❌ 需要模型灵活判断的场景

---

## 🔄 回滚补丁

如果补丁导致问题，可以随时回滚：

```powershell
# Windows
.\apply_force_tool_patch.ps1 -Revert

# Linux/Mac
./apply_force_tool_patch.sh --revert
```

---

## 📊 预期效果对比

### 应用补丁前

**用户请求：** "读取 README.md 文件"

**模型响应：**
```
操作完成。README.md 文件包含项目的基本信息...
```

**问题：** 模型没有实际调用 `read_file` 工具

### 应用补丁后

**用户请求：** "读取 README.md 文件"

**模型行为：**
1. 调用 `read_file` 工具
2. Cursor 执行工具并返回文件内容
3. 模型基于实际内容回答

**日志显示：**
```json
{
  "tool_calls": [{
    "id": "call_xxx",
    "type": "function",
    "function": {
      "name": "read_file",
      "arguments": "{\"path\":\"README.md\"}"
    }
  }]
}
```

---

## 🎯 总结

### 问题根源
- Cursor 发送了工具定义
- 但 `tool_choice: "auto"` 让模型自己决定
- Claude 选择返回文本而不是调用工具

### 解决方案
- 应用补丁将 `auto` 改为 `required`
- 强制模型必须使用工具
- 确保模型执行操作而不是描述操作

### 立即执行

```bash
# 1. 应用补丁
.\apply_force_tool_patch.ps1

# 2. 重启服务
go run ./cmd/server

# 3. 在 Cursor 中测试
# 按 Cmd/Ctrl + I，发送：读取 README.md 文件

# 4. 验证模型实际调用了工具
```

---

**这才是你真正需要的解决方案！** 🎯

应用补丁后，模型将被强制使用 Cursor 提供的工具来实际操作文件，而不是只返回"操作完成"这样的文本。
