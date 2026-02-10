#!/bin/bash
# 应用强制工具调用补丁

SERVICE_FILE="internal/bedrockproxy/service.go"
BACKUP_FILE="internal/bedrockproxy/service.go.backup"

if [ "$1" = "--revert" ]; then
    echo "回滚补丁..."
    if [ -f "$BACKUP_FILE" ]; then
        cp "$BACKUP_FILE" "$SERVICE_FILE"
        echo "✓ 已恢复原始文件"
        rm "$BACKUP_FILE"
    else
        echo "✗ 未找到备份文件"
        exit 1
    fi
    exit 0
fi

echo "=== 应用强制工具调用补丁 ==="
echo ""

# 检查文件是否存在
if [ ! -f "$SERVICE_FILE" ]; then
    echo "✗ 未找到文件: $SERVICE_FILE"
    exit 1
fi

# 创建备份
echo "[1/3] 创建备份..."
cp "$SERVICE_FILE" "$BACKUP_FILE"
echo "✓ 备份已创建: $BACKUP_FILE"
echo ""

# 检查是否已经应用过补丁
if grep -q "强制工具调用" "$SERVICE_FILE"; then
    echo "⚠️ 补丁似乎已经应用过了"
    echo "如果需要重新应用，请先运行: ./apply_force_tool_patch.sh --revert"
    exit 0
fi

# 应用补丁
echo "[2/3] 应用补丁..."

# 使用 sed 或手动编辑提示
echo "请手动编辑 $SERVICE_FILE"
echo "在第 579 行左右找到以下代码："
echo ""
echo "  cfg := &brtypes.ToolConfiguration{"
echo "      Tools: bedrockTools,"
echo "  }"
echo "  if toolChoice != nil {"
echo "      cfg.ToolChoice = toolChoice"
echo "  }"
echo "  return cfg, nil"
echo ""
echo "替换为："
echo ""
echo "  cfg := &brtypes.ToolConfiguration{"
echo "      Tools: bedrockTools,"
echo "  }"
echo ""
echo "  // 强制工具调用"
echo "  if toolChoice == nil {"
echo "      toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}"
echo "  } else if _, isAuto := toolChoice.(*brtypes.ToolChoiceMemberAuto); isAuto {"
echo "      toolChoice = &brtypes.ToolChoiceMemberAny{Value: brtypes.AnyToolChoice{}}"
echo "  }"
echo ""
echo "  if toolChoice != nil {"
echo "      cfg.ToolChoice = toolChoice"
echo "  }"
echo "  return cfg, nil"
echo ""
echo "详细说明请参考: FORCE_TOOL_PATCH.md"
