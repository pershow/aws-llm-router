# 应用强制工具调用补丁
# 此脚本会修改 service.go 以强制模型使用工具

param(
    [switch]$Revert = $false
)

$serviceFile = "internal\bedrockproxy\service.go"
$backupFile = "internal\bedrockproxy\service.go.backup"

if ($Revert) {
    Write-Host "回滚补丁..." -ForegroundColor Yellow
    if (Test-Path $backupFile) {
        Copy-Item $backupFile $serviceFile -Force
        Write-Host "✓ 已恢复原始文件" -ForegroundColor Green
        Remove-Item $backupFile
    } else {
        Write-Host "✗ 未找到备份文件" -ForegroundColor Red
        exit 1
    }
    exit 0
}

Write-Host "=== 应用强制工具调用补丁 ===" -ForegroundColor Cyan
Write-Host ""

# 检查文件是否存在
if (-not (Test-Path $serviceFile)) {
    Write-Host "✗ 未找到文件: $serviceFile" -ForegroundColor Red
    exit 1
}

# 创建备份
Write-Host "[1/3] 创建备份..." -ForegroundColor Yellow
Copy-Item $serviceFile $backupFile -Force
Write-Host "✓ 备份已创建: $backupFile" -ForegroundColor Green
Write-Host ""

# 读取文件内容
Write-Host "[2/3] 应用补丁..." -ForegroundColor Yellow
$content = Get-Content $serviceFile -Raw

# 检查是否已经应用过补丁
if ($content -match "强制工具调用") {
    Write-Host "⚠️ 补丁似乎已经应用过了" -ForegroundColor Yellow
    Write-Host "如果需要重新应用，请先运行: .\apply_force_tool_patch.ps1 -Revert" -ForegroundColor Gray
    exit 0
}

# 查找并替换代码
$oldCode = @"
	cfg := &brtypes.ToolConfiguration{
		Tools: bedrockTools,
	}
	if toolChoice != nil {
		cfg.ToolChoice = toolChoice
	}
	return cfg, nil
"@

$newCode = @"
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
"@

if ($content -match [regex]::Escape($oldCode)) {
    $content = $content -replace [regex]::Escape($oldCode), $newCode
    Set-Content $serviceFile -Value $content -NoNewline
    Write-Host "✓ 补丁已应用" -ForegroundColor Green
} else {
    Write-Host "✗ 未找到匹配的代码块" -ForegroundColor Red
    Write-Host "文件可能已被修改，请手动应用补丁" -ForegroundColor Yellow
    Write-Host "参考: FORCE_TOOL_PATCH.md" -ForegroundColor Gray
    Remove-Item $backupFile
    exit 1
}
Write-Host ""

# 验证语法
Write-Host "[3/3] 验证 Go 语法..." -ForegroundColor Yellow
$result = go build -o nul ./internal/bedrockproxy 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ 语法验证通过" -ForegroundColor Green
} else {
    Write-Host "✗ 语法错误，正在回滚..." -ForegroundColor Red
    Copy-Item $backupFile $serviceFile -Force
    Write-Host "已回滚到原始版本" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "错误信息:" -ForegroundColor Red
    Write-Host $result
    exit 1
}
Write-Host ""

Write-Host "=== 补丁应用成功 ===" -ForegroundColor Green
Write-Host ""
Write-Host "下一步:" -ForegroundColor Yellow
Write-Host "  1. 重启服务: go run ./cmd/server" -ForegroundColor Gray
Write-Host "  2. 测试功能: .\test_tool_calling.ps1 -ApiKey 'your-key'" -ForegroundColor Gray
Write-Host "  3. 在 Cursor 中测试" -ForegroundColor Gray
Write-Host ""
Write-Host "如需回滚:" -ForegroundColor Yellow
Write-Host "  .\apply_force_tool_patch.ps1 -Revert" -ForegroundColor Gray
Write-Host ""
