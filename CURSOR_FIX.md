# Cursor 工具调用问题快速修复

## 问题现象

在 Cursor 中使用时，工具调用失败并出现 `broken pipe` 错误。

## 快速解决方案

在 `.env` 文件中添加：

```bash
# 缓冲工具参数以避免连接中断
BUFFER_TOOL_CALL_ARGS=true

# 强制工具调用（推荐用于 Cursor Agent 模式）
FORCE_TOOL_USE=true
```

重启服务即可。

## 原因

默认配置下，工具调用的 JSON 参数是逐字符流式传输的，这在网络不稳定或 Cursor 客户端处理时可能导致连接中断。启用 `BUFFER_TOOL_CALL_ARGS=true` 后，会先缓冲完整参数再发送，大大提高稳定性。

## 详细说明

查看 [docs/CURSOR_TOOL_CALLING_ISSUE.md](./docs/CURSOR_TOOL_CALLING_ISSUE.md) 了解完整的问题分析和解决方案。
