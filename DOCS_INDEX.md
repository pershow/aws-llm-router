# 📖 文档导航

## 🚨 遇到 Cursor 工具调用问题？

### 👉 从这里开始：[START_HERE.md](START_HERE.md)

---

## 📚 文档结构

### 快速入门（推荐顺序）

1. **[START_HERE.md](START_HERE.md)** ⭐
   - 3 步快速解决方案
   - 10 分钟完成诊断
   - 最适合第一次使用

2. **[QUICK_REFERENCE.md](QUICK_REFERENCE.md)**
   - 1 页快速参考卡片
   - 常用命令和判断标准
   - 适合快速查阅

3. **[QUICK_DIAGNOSIS.md](QUICK_DIAGNOSIS.md)**
   - 5 分钟快速诊断
   - 详细的日志判断
   - 针对已启用 Agent 模式的用户

### 完整指南

4. **[SOLUTION_GUIDE.md](SOLUTION_GUIDE.md)** 📘
   - 完整的解决方案指南
   - 包含所有场景和解决方案
   - 主要参考文档

5. **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)**
   - 详细的故障排查步骤
   - 常见问题解答
   - 高级调试技巧

### 技术文档

6. **[FORCE_TOOL_PATCH.md](FORCE_TOOL_PATCH.md)**
   - 强制工具调用补丁说明
   - 技术实现细节
   - 适用场景和注意事项

7. **[CURSOR_INTEGRATION_GUIDE.md](CURSOR_INTEGRATION_GUIDE.md)**
   - Cursor 集成详细指南
   - 配置说明和测试方法
   - 数据库日志查询

### 项目信息

8. **[CHANGES.md](CHANGES.md)**
   - 项目改动总结
   - 新增文件清单
   - 修改内容说明

9. **[COMPLETION_SUMMARY.md](COMPLETION_SUMMARY.md)**
   - 完成总结报告
   - 交付成果清单
   - 质量保证说明

---

## 🛠️ 工具和脚本

### 测试脚本

- **`test_tool_calling.ps1`** (Windows)
  ```powershell
  .\test_tool_calling.ps1 -ApiKey "your-api-key"
  ```

- **`test_tool_calling.sh`** (Linux/Mac)
  ```bash
  chmod +x test_tool_calling.sh
  API_KEY="your-api-key" ./test_tool_calling.sh
  ```

### 补丁脚本

- **`apply_force_tool_patch.ps1`** (Windows)
  ```powershell
  # 应用补丁
  .\apply_force_tool_patch.ps1

  # 回滚补丁
  .\apply_force_tool_patch.ps1 -Revert
  ```

- **`apply_force_tool_patch.sh`** (Linux/Mac)
  ```bash
  chmod +x apply_force_tool_patch.sh
  ./apply_force_tool_patch.sh
  ```

---

## 🎯 按场景选择文档

### 场景 1：第一次遇到问题
→ 阅读 **[START_HERE.md](START_HERE.md)**

### 场景 2：需要快速查阅命令
→ 查看 **[QUICK_REFERENCE.md](QUICK_REFERENCE.md)**

### 场景 3：已经启用调试，需要判断日志
→ 参考 **[QUICK_DIAGNOSIS.md](QUICK_DIAGNOSIS.md)**

### 场景 4：需要完整的解决方案
→ 阅读 **[SOLUTION_GUIDE.md](SOLUTION_GUIDE.md)**

### 场景 5：问题复杂，需要深入排查
→ 查看 **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)**

### 场景 6：需要应用补丁
→ 参考 **[FORCE_TOOL_PATCH.md](FORCE_TOOL_PATCH.md)**

### 场景 7：了解项目改动
→ 阅读 **[CHANGES.md](CHANGES.md)**

---

## 📊 文档关系图

```
START_HERE.md (开始)
    ↓
QUICK_REFERENCE.md (快速参考)
    ↓
QUICK_DIAGNOSIS.md (快速诊断)
    ↓
SOLUTION_GUIDE.md (完整方案) ← 主文档
    ↓
    ├─→ TROUBLESHOOTING.md (详细排查)
    ├─→ FORCE_TOOL_PATCH.md (补丁说明)
    └─→ CURSOR_INTEGRATION_GUIDE.md (集成指南)
```

---

## ⏱️ 预计阅读时间

| 文档 | 阅读时间 | 适用场景 |
|------|----------|----------|
| START_HERE.md | 5 分钟 | 快速开始 |
| QUICK_REFERENCE.md | 2 分钟 | 快速查阅 |
| QUICK_DIAGNOSIS.md | 5 分钟 | 诊断问题 |
| SOLUTION_GUIDE.md | 10 分钟 | 完整方案 |
| TROUBLESHOOTING.md | 15 分钟 | 深入排查 |
| FORCE_TOOL_PATCH.md | 5 分钟 | 应用补丁 |
| CURSOR_INTEGRATION_GUIDE.md | 10 分钟 | 集成配置 |

---

## 🎓 学习路径

### 初学者路径
1. START_HERE.md
2. QUICK_REFERENCE.md
3. 实践操作
4. 如有问题 → SOLUTION_GUIDE.md

### 高级用户路径
1. QUICK_DIAGNOSIS.md
2. TROUBLESHOOTING.md
3. FORCE_TOOL_PATCH.md
4. 深入理解代码

---

## 🔗 快速链接

- **立即开始：** [START_HERE.md](START_HERE.md)
- **快速参考：** [QUICK_REFERENCE.md](QUICK_REFERENCE.md)
- **完整方案：** [SOLUTION_GUIDE.md](SOLUTION_GUIDE.md)
- **测试脚本：** `test_tool_calling.ps1` / `test_tool_calling.sh`
- **补丁脚本：** `apply_force_tool_patch.ps1` / `apply_force_tool_patch.sh`

---

## 💡 提示

- 所有文档都是中文，易于理解
- 包含大量示例和命令
- 支持 Windows、Linux、Mac
- 提供自动化脚本工具

---

**开始解决问题：** 打开 [START_HERE.md](START_HERE.md) 👈
