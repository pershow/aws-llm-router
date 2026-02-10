# 强制工具调用补丁

## 问题描述

即使 Cursor 发送了工具定义，Claude 模型有时会选择返回文本说明而不是实际调用工具。

## 解决方案

修改代理，当请求包含工具但没有指定 `tool_choice` 时，自动设置为 `required`（强制使用工具）。

## 应用补丁

### 方法 1：手动编辑

编辑文件 `internal/bedrockproxy/service.go`，找到第 579 行左右的代码：

```go
cfg := &brtypes.ToolConfiguration{
    Tools: bedrockTools,
}
if toolChoice != nil {
    cfg.ToolChoice = toolChoice
}
return cfg, nil
```

修改为：

```go
cfg := &brtypes.ToolConfiguration{
    Tools: bedrockTools,
}

// 强制工具调用：如果有工具但 tool_choice 为 auto 或未指定，改为 required
// 这确保模型必须使用工具而不是返回文本说明
if toolChoice == nil {
    // 未指定 tool_choice，默认强制使用工具
    toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
} else if _, isAuto := toolChoice.(*brtypes.ToolChoiceMemberAuto); isAuto {
    // tool_choice 是 auto，改为 required
    toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
}

if toolChoice != nil {
    cfg.ToolChoice = toolChoice
}
return cfg, nil
```

### 方法 2：使用提供的补丁脚本

运行以下 PowerShell 脚本：

```powershell
# 应用强制工具调用补丁
.\apply_force_tool_patch.ps1
```

## 验证

1. 重新编译并运行服务：
   ```bash
   go run ./cmd/server
   ```

2. 运行测试：
   ```bash
   .\test_tool_calling.ps1 -ApiKey "your-key"
   ```

3. 在 Cursor 中测试

## 回滚

如果需要恢复原始行为：

```bash
# 恢复备份
cp internal/bedrockproxy/service.go.backup internal/bedrockproxy/service.go
```

## 注意事项

**优点：**
- ✅ 确保模型在有工具时必须使用工具
- ✅ 解决 Claude 返回文本说明的问题
- ✅ 提高 Cursor Agent 的可靠性

**缺点：**
- ⚠️ 模型无法选择不使用工具
- ⚠️ 某些情况下可能导致不必要的工具调用
- ⚠️ 如果工具定义不完整，可能导致错误

**建议：**
- 仅在确认 Cursor 发送了工具定义但模型不使用时应用
- 先尝试不使用补丁，只在必要时使用
- 可以添加环境变量控制此行为

## 可选：环境变量控制

如果你想通过环境变量控制此行为，可以进一步修改：

### 1. 在 `internal/config/config.go` 中添加：

```go
type Config struct {
    // ... 现有字段 ...
    ForceToolUse bool
}

func Load() (Config, error) {
    // ... 现有代码 ...

    forceToolUse := os.Getenv("FORCE_TOOL_USE") == "true"

    return Config{
        // ... 现有字段 ...
        ForceToolUse: forceToolUse,
    }, nil
}
```

### 2. 在 `internal/bedrockproxy/service.go` 中修改：

```go
type Service struct {
    client               *bedrockruntime.Client
    defaultModelID       string
    enabledModels        map[string]struct{}
    defaultMaxOutputToken int32
    forceToolUse         bool  // 新增
}

func NewService(client *bedrockruntime.Client, defaultModelID string, enabledModels map[string]struct{}, defaultMaxOutputToken int32, forceToolUse bool) *Service {
    return &Service{
        client:                client,
        defaultModelID:        defaultModelID,
        enabledModels:         enabledModels,
        defaultMaxOutputToken: defaultMaxOutputToken,
        forceToolUse:          forceToolUse,  // 新增
    }
}
```

### 3. 在 `buildToolConfiguration` 中使用：

```go
func (s *Service) buildToolConfiguration(tools []openai.Tool, rawToolChoice json.RawMessage) (*brtypes.ToolConfiguration, error) {
    // ... 现有代码 ...

    cfg := &brtypes.ToolConfiguration{
        Tools: bedrockTools,
    }

    // 如果启用了强制工具使用
    if s.forceToolUse && toolChoice == nil {
        toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}
    }

    if toolChoice != nil {
        cfg.ToolChoice = toolChoice
    }
    return cfg, nil
}
```

### 4. 在 `.env` 中配置：

```bash
# 强制模型在有工具时必须使用工具
FORCE_TOOL_USE=true
```

这样你可以随时通过环境变量开关此功能。
