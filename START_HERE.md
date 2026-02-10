# 🎯 立即开始 - 3 步解决 Cursor 工具调用问题

## 你现在的情况
- ✅ AWS Bedrock 代理已配置
- ✅ Cursor Agent 模式已启用
- ❌ 模型返回"操作完成，请查看 cursor 要求"而不是实际调用工具

---

## 🚀 立即执行（总共 10 分钟）

### 步骤 1：启用调试（2 分钟）

打开 `.env` 文件，添加这一行：

```bash
DEBUG_REQUESTS=true
```

保存文件。

### 步骤 2：重启服务（1 分钟）

```bash
# 停止当前服务（按 Ctrl+C）
# 重新启动
go run ./cmd/server
```

### 步骤 3：在 Cursor 中测试（2 分钟）

1. 打开 Cursor
2. 按 **Cmd/Ctrl + I** 打开 Composer（Agent 模式）
3. 发送请求：`读取 README.md 文件的内容`

### 步骤 4：查看日志并判断（5 分钟）

在服务器控制台中查找以下内容：

#### 🔍 情况 A：看到这个
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ✓ 响应包含工具调用!
[DEBUG-xxx] finish_reason: tool_calls
```

**说明：** 代理工作正常，问题在 Cursor 端

**解决方案：**
1. 重启 Cursor
2. 更新 Cursor 到最新版本
3. 检查 Cursor 开发者工具（Help → Toggle Developer Tools → Console）

---

#### 🔍 情况 B：看到这个
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ⚠️ 响应不包含工具调用
[DEBUG-xxx] ⚠️ 模型返回了文本: "操作完成..."
```

**说明：** 模型收到工具但选择不使用

**解决方案：** 应用强制工具调用补丁

```powershell
# 运行补丁脚本
.\apply_force_tool_patch.ps1

# 重启服务
go run ./cmd/server

# 在 Cursor 中重新测试
```

---

#### 🔍 情况 C：看到这个
```
[DEBUG-xxx] ⚠️ 请求不包含 tools 参数
```

**说明：** Cursor 没有发送工具定义

**解决方案：**
1. 确认使用 **Cmd/Ctrl + I**（Composer/Agent 模式）
2. 不要使用 **Cmd/Ctrl + L**（普通聊天）
3. 检查 Cursor 版本 >= 0.40.0
4. 检查 Cursor 设置：
   - Settings → Models → 确认 Base URL: `http://localhost:8080/v1`
   - 确认 API Key 正确
   - 确认 Model ID 正确

---

## 🧪 验证功能（可选）

运行测试脚本验证代理功能：

```powershell
.\test_tool_calling.ps1 -ApiKey "your-api-key"
```

**期望输出：**
```
✓ 服务正常运行
✓ 可用模型数量: X
✓ 响应成功
✓ 模型成功调用工具!
✓ 工具结果处理成功
```

如果测试通过但 Cursor 不工作 → 问题在 Cursor 端

---

## 📚 详细文档

如果需要更多信息，查看：

| 文档 | 用途 | 阅读时间 |
|------|------|----------|
| `QUICK_REFERENCE.md` | 快速参考卡片 | 2 分钟 |
| `SOLUTION_GUIDE.md` | 完整解决方案 | 10 分钟 |
| `TROUBLESHOOTING.md` | 详细故障排查 | 15 分钟 |

---

## 🎯 最可能的情况

根据经验，90% 的情况是：

**Cursor 没有发送工具定义**
- 原因：使用了普通聊天而不是 Agent 模式
- 解决：使用 Cmd/Ctrl + I 而不是 Cmd/Ctrl + L

**模型选择不使用工具**
- 原因：Claude 认为不需要使用工具
- 解决：运行 `.\apply_force_tool_patch.ps1`

---

## ✅ 成功标志

当一切正常时，你会看到：

**服务器日志：**
```
[DEBUG-xxx] ⚠️ 请求包含 5 个工具定义
[DEBUG-xxx] ✓ 响应包含工具调用!
[DEBUG-xxx] finish_reason: tool_calls
```

**Cursor 行为：**
- 模型实际调用工具（例如 `read_file`）
- 显示工具执行结果
- 基于结果继续对话
- **不会**返回"操作完成，请查看..."

---

## 🆘 需要帮助？

如果以上步骤都不能解决问题，提供以下信息：

1. **Cursor 版本**（Cursor → About）
2. **调试日志**（从 `[DEBUG-xxx]` 开始的完整日志）
3. **测试脚本输出**（`.\test_tool_calling.ps1` 的结果）

---

## 🎉 总结

**代理已经支持工具调用，无需修改代码！**

**立即行动：**
1. 添加 `DEBUG_REQUESTS=true` 到 `.env`
2. 重启服务
3. 在 Cursor Agent 中测试
4. 根据日志判断并操作

**大多数情况下，问题在于：**
- Cursor 配置或使用方式
- 模型选择不使用工具（可用补丁解决）

**开始吧！** 🚀
