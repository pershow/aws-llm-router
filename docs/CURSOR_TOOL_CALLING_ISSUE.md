# Cursor 工具调用问题排查与解决方案

## 问题描述

在 Cursor 中使用 aws-cursor-router 作为代理时，遇到工具调用（tool calling）失败的问题：

1. 模型开始调用工具，但参数传输过程中连接中断
2. 出现 `broken pipe` 错误
3. 工具调用无法完成，导致 Agent 模式无法正常工作

## 问题分析

### 1. 日志分析

查看 debug_logs 目录中的日志文件，发现以下问题：

```
data: {"id":"chatcmpl-req-...","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"pattern\""}}]},"finish_reason":null}]}
data: {"id":"chatcmpl-req-...","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":": \"^pa"}}]},"finish_reason":null}]}
data: {"id":"chatcmpl-req-...","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ckage"}}]},"finish_reason":null}]}
...
{"error":{"message":"bedrock stream failed: write tcp 192.168.192.2:8080->10.242.48.51:18830: write: broken pipe","type":"invalid_request_error","code":"502"}}
```

**关键发现**：
- 工具调用的 JSON 参数被逐字符流式传输（例如："^pa" → "ckage" → " main"）
- 细粒度的流式传输在长时间连接中容易导致 TCP 连接断开
- Cursor 客户端可能无法正确处理这种碎片化的工具参数

### 2. 根本原因

代码中有两种工具参数传输模式：

1. **逐 delta 转发模式**（默认）：
   - 每收到一个字符/片段就立即发送给客户端
   - 兼容 bedrock-access-gateway 的行为
   - 优点：实时性好
   - 缺点：网络不稳定时容易中断，Cursor 可能无法正确解析碎片化的 JSON

2. **缓冲模式**：
   - 在服务端缓冲完整的工具参数
   - 参数接收完成后一次性发送给客户端
   - 优点：更稳定，客户端更容易处理
   - 缺点：实时性稍差

默认配置使用逐 delta 模式，导致 Cursor 工具调用失败。

## 解决方案

### 方案 1：启用工具参数缓冲（推荐）

在 `.env` 文件中添加或修改以下配置：

```bash
# 缓冲工具调用参数
# 设置为 true 可避免工具参数逐字符流式传输导致的连接中断问题
# 启用后会在接收完整参数后一次性发送给客户端，提高稳定性
BUFFER_TOOL_CALL_ARGS=true

# 强制模型调用工具 (当请求包含 tools 时)
# 设置为 true 可解决 Cursor Agent 模式下模型不调用工具的问题
FORCE_TOOL_USE=true
```

**重启服务**使配置生效：
```bash
# 如果使用 systemd
sudo systemctl restart aws-cursor-router

# 如果直接运行
# 按 Ctrl+C 停止，然后重新运行
./aws-cursor-router
```

### 方案 2：增加超时时间（辅助）

如果网络环境较差，可以适当增加请求超时时间：

```bash
# 默认 300 秒，可以增加到 500 秒或更高
REQUEST_TIMEOUT_SECONDS=500
```

### 方案 3：优化网络连接

1. **检查防火墙设置**：
   - 确保代理服务器到 AWS Bedrock 的连接稳定
   - 检查是否有中间防火墙或负载均衡器截断长连接

2. **使用更稳定的网络**：
   - 如果可能，使用有线网络而不是 WiFi
   - 确保网络带宽充足

## 配置说明

### BUFFER_TOOL_CALL_ARGS

**作用**：控制工具调用参数的传输模式

**可选值**：
- `false`（默认）：逐 delta 转发，实时性好但可能不稳定
- `true`：缓冲完整参数后发送，更稳定但实时性稍差

**推荐设置**：
- Cursor 使用场景：`true`
- API 网关场景：`false`（如果下游客户端需要实时流式）

### FORCE_TOOL_USE

**作用**：强制模型在有工具定义时调用工具

**可选值**：
- `false`（默认）：让模型自行决定是否调用工具
- `true`：强制调用工具（推荐用于 Cursor Agent 模式）

**注意**：
- 该配置只在第一轮对话时生效
- 收到工具结果后会自动切换为 `auto` 模式，避免死循环

### MIN_TOOL_MAX_OUTPUT_TOKENS

**作用**：当请求包含工具时，确保 max_tokens 至少达到此值

**默认值**：8192

**原因**：
- 工具调用通常需要较长的输出来容纳完整的 JSON 参数
- 如果 max_tokens 太小，可能导致工具参数被截断（finish_reason=length）

## 验证解决方案

1. **修改配置**：
   ```bash
   echo "BUFFER_TOOL_CALL_ARGS=true" >> .env
   ```

2. **重启服务**

3. **在 Cursor 中测试**：
   - 打开一个项目
   - 使用 Agent 模式提出一个需要工具调用的请求
   - 例如："找到所有包含 'package main' 的 Go 文件"

4. **检查日志**：
   ```bash
   tail -f debug_logs/*.txt
   ```
   
   应该看到工具调用完整完成，没有 broken pipe 错误。

## 工作原理

### 修改前（BUFFER_TOOL_CALL_ARGS=false）

```
Bedrock Stream → Router → Cursor
每个字符立即转发:
  "{"  →  "p"  →  "a"  →  "t"  →  "t"  ...
  ↓       ↓      ↓      ↓      ↓
可能导致连接超时或 Cursor 解析失败
```

### 修改后（BUFFER_TOOL_CALL_ARGS=true）

```
Bedrock Stream → Router 缓冲区 → Cursor
完整参数一次发送:
  "{" + "p" + "a" + "t" + ... → 完整 JSON
  ↓
{"pattern": "^package main", "type": "go", ...}
  ↓
稳定传输，Cursor 正确解析
```

## 常见问题

### Q1: 启用缓冲后，工具调用是否还是实时的？

A: 是的，只是参数部分被缓冲。工具调用的开始（tool name、tool ID）仍然是实时发送的。参数完整接收后立即发送，延迟通常不到 1 秒。

### Q2: 是否会影响文本响应的实时性？

A: 不会。`BUFFER_TOOL_CALL_ARGS` 只影响工具参数，文本响应仍然是逐 token 流式传输。

### Q3: 为什么默认不启用缓冲？

A: 为了保持与 bedrock-access-gateway 的兼容性。但对于 Cursor 这种特定客户端，推荐启用缓冲以提高稳定性。

### Q4: 还是有问题怎么办？

1. 检查网络连接是否稳定
2. 增加 `REQUEST_TIMEOUT_SECONDS`
3. 查看完整的 debug_logs 日志
4. 在 GitHub 提 issue 并附上日志

## 相关代码

- 配置加载：`internal/config/config.go:62`
- 流式处理：`internal/bedrockproxy/service.go:438`
- 参数缓冲：`internal/bedrockproxy/service.go:277`

## 参考资料

- [OpenAI Chat Completion API 规范](https://platform.openai.com/docs/api-reference/chat)
- [AWS Bedrock Converse API 文档](https://docs.aws.amazon.com/bedrock/latest/APIReference/API_runtime_Converse.html)
- [项目 README](../README.md)
