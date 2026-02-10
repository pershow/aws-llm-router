# 工具调用测试脚本
# 用于验证 AWS Cursor Router 的工具调用功能

param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$ApiKey = "",
    [string]$Model = "anthropic.claude-3-5-sonnet-20240620-v1:0"
)

if ($ApiKey -eq "") {
    Write-Host "错误: 请提供 API Key" -ForegroundColor Red
    Write-Host "使用方法: .\test_tool_calling.ps1 -ApiKey 'your-api-key'" -ForegroundColor Yellow
    exit 1
}

Write-Host "=== AWS Cursor Router 工具调用测试 ===" -ForegroundColor Cyan
Write-Host "Base URL: $BaseUrl" -ForegroundColor Gray
Write-Host "Model: $Model" -ForegroundColor Gray
Write-Host ""

# 测试 1: 健康检查
Write-Host "[测试 1] 健康检查..." -ForegroundColor Yellow
try {
    $health = Invoke-RestMethod -Uri "$BaseUrl/healthz" -Method Get
    Write-Host "✓ 服务正常运行" -ForegroundColor Green
} catch {
    Write-Host "✗ 服务不可用: $_" -ForegroundColor Red
    exit 1
}
Write-Host ""

# 测试 2: 获取模型列表
Write-Host "[测试 2] 获取模型列表..." -ForegroundColor Yellow
try {
    $models = Invoke-RestMethod -Uri "$BaseUrl/v1/models" -Method Get -Headers @{
        "Authorization" = "Bearer $ApiKey"
    }
    Write-Host "✓ 可用模型数量: $($models.data.Count)" -ForegroundColor Green
    $models.data | ForEach-Object { Write-Host "  - $($_.id)" -ForegroundColor Gray }
} catch {
    Write-Host "✗ 获取模型失败: $_" -ForegroundColor Red
    exit 1
}
Write-Host ""

# 测试 3: 简单对话（无工具）
Write-Host "[测试 3] 简单对话测试（无工具）..." -ForegroundColor Yellow
$simpleRequest = @{
    model = $Model
    messages = @(
        @{
            role = "user"
            content = "Hello, please respond with 'Hi there!'"
        }
    )
} | ConvertTo-Json -Depth 10

try {
    $response = Invoke-RestMethod -Uri "$BaseUrl/v1/chat/completions" -Method Post -Headers @{
        "Authorization" = "Bearer $ApiKey"
        "Content-Type" = "application/json"
    } -Body $simpleRequest

    Write-Host "✓ 响应成功" -ForegroundColor Green
    Write-Host "  Response: $($response.choices[0].message.content)" -ForegroundColor Gray
    Write-Host "  Tokens: $($response.usage.total_tokens)" -ForegroundColor Gray
} catch {
    Write-Host "✗ 请求失败: $_" -ForegroundColor Red
    exit 1
}
Write-Host ""

# 测试 4: 工具调用测试
Write-Host "[测试 4] 工具调用测试..." -ForegroundColor Yellow
$toolRequest = @{
    model = $Model
    messages = @(
        @{
            role = "user"
            content = "What is the weather in San Francisco?"
        }
    )
    tools = @(
        @{
            type = "function"
            function = @{
                name = "get_weather"
                description = "Get the current weather in a given location"
                parameters = @{
                    type = "object"
                    properties = @{
                        location = @{
                            type = "string"
                            description = "The city and state, e.g. San Francisco, CA"
                        }
                        unit = @{
                            type = "string"
                            enum = @("celsius", "fahrenheit")
                            description = "The temperature unit"
                        }
                    }
                    required = @("location")
                }
            }
        }
    )
    tool_choice = "auto"
} | ConvertTo-Json -Depth 10

try {
    $response = Invoke-RestMethod -Uri "$BaseUrl/v1/chat/completions" -Method Post -Headers @{
        "Authorization" = "Bearer $ApiKey"
        "Content-Type" = "application/json"
    } -Body $toolRequest

    $message = $response.choices[0].message

    if ($message.tool_calls -and $message.tool_calls.Count -gt 0) {
        Write-Host "✓ 模型成功调用工具!" -ForegroundColor Green
        Write-Host "  Finish Reason: $($response.choices[0].finish_reason)" -ForegroundColor Gray
        Write-Host "  Tool Calls:" -ForegroundColor Gray
        foreach ($toolCall in $message.tool_calls) {
            Write-Host "    - ID: $($toolCall.id)" -ForegroundColor Gray
            Write-Host "      Function: $($toolCall.function.name)" -ForegroundColor Gray
            Write-Host "      Arguments: $($toolCall.function.arguments)" -ForegroundColor Gray
        }

        # 保存工具调用信息用于下一步测试
        $script:toolCallId = $message.tool_calls[0].id
        $script:toolCallFunction = $message.tool_calls[0].function.name
        $script:toolCallArguments = $message.tool_calls[0].function.arguments

    } else {
        Write-Host "✗ 模型没有调用工具，而是返回了文本响应" -ForegroundColor Red
        Write-Host "  Response: $($message.content)" -ForegroundColor Gray
        Write-Host "  Finish Reason: $($response.choices[0].finish_reason)" -ForegroundColor Gray
        Write-Host ""
        Write-Host "可能的原因:" -ForegroundColor Yellow
        Write-Host "  1. 模型认为不需要使用工具" -ForegroundColor Gray
        Write-Host "  2. 工具定义不够清晰" -ForegroundColor Gray
        Write-Host "  3. 尝试使用 tool_choice='required' 强制工具调用" -ForegroundColor Gray
        exit 1
    }
} catch {
    Write-Host "✗ 请求失败: $_" -ForegroundColor Red
    Write-Host "错误详情: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
Write-Host ""

# 测试 5: 工具结果回传
Write-Host "[测试 5] 工具结果回传测试..." -ForegroundColor Yellow
$toolResultRequest = @{
    model = $Model
    messages = @(
        @{
            role = "user"
            content = "What is the weather in San Francisco?"
        }
        @{
            role = "assistant"
            content = $null
            tool_calls = @(
                @{
                    id = $script:toolCallId
                    type = "function"
                    function = @{
                        name = $script:toolCallFunction
                        arguments = $script:toolCallArguments
                    }
                }
            )
        }
        @{
            role = "tool"
            tool_call_id = $script:toolCallId
            content = '{"temperature": 72, "unit": "fahrenheit", "condition": "sunny", "humidity": 65}'
        }
    )
} | ConvertTo-Json -Depth 10

try {
    $response = Invoke-RestMethod -Uri "$BaseUrl/v1/chat/completions" -Method Post -Headers @{
        "Authorization" = "Bearer $ApiKey"
        "Content-Type" = "application/json"
    } -Body $toolResultRequest

    Write-Host "✓ 工具结果处理成功" -ForegroundColor Green
    Write-Host "  Final Response: $($response.choices[0].message.content)" -ForegroundColor Gray
    Write-Host "  Finish Reason: $($response.choices[0].finish_reason)" -ForegroundColor Gray
    Write-Host "  Total Tokens: $($response.usage.total_tokens)" -ForegroundColor Gray
} catch {
    Write-Host "✗ 请求失败: $_" -ForegroundColor Red
    exit 1
}
Write-Host ""

# 测试 6: 强制工具调用 (tool_choice = required)
Write-Host "[测试 6] 强制工具调用测试 (tool_choice=required)..." -ForegroundColor Yellow
$forcedToolRequest = @{
    model = $Model
    messages = @(
        @{
            role = "user"
            content = "Tell me about the weather"
        }
    )
    tools = @(
        @{
            type = "function"
            function = @{
                name = "get_weather"
                description = "Get the current weather in a given location"
                parameters = @{
                    type = "object"
                    properties = @{
                        location = @{
                            type = "string"
                            description = "The city and state"
                        }
                    }
                    required = @("location")
                }
            }
        }
    )
    tool_choice = "required"
} | ConvertTo-Json -Depth 10

try {
    $response = Invoke-RestMethod -Uri "$BaseUrl/v1/chat/completions" -Method Post -Headers @{
        "Authorization" = "Bearer $ApiKey"
        "Content-Type" = "application/json"
    } -Body $forcedToolRequest

    $message = $response.choices[0].message

    if ($message.tool_calls -and $message.tool_calls.Count -gt 0) {
        Write-Host "✓ 强制工具调用成功!" -ForegroundColor Green
        Write-Host "  Tool: $($message.tool_calls[0].function.name)" -ForegroundColor Gray
    } else {
        Write-Host "✗ 即使使用 required，模型仍未调用工具" -ForegroundColor Red
        Write-Host "  Response: $($message.content)" -ForegroundColor Gray
    }
} catch {
    Write-Host "✗ 请求失败: $_" -ForegroundColor Red
}
Write-Host ""

# 测试 7: 流式工具调用
Write-Host "[测试 7] 流式工具调用测试..." -ForegroundColor Yellow
Write-Host "  (注意: PowerShell 的 Invoke-RestMethod 不支持 SSE 流式响应)" -ForegroundColor Gray
Write-Host "  建议使用 curl 或其他工具测试流式功能" -ForegroundColor Gray
Write-Host ""

Write-Host "=== 测试完成 ===" -ForegroundColor Cyan
Write-Host ""
Write-Host "总结:" -ForegroundColor Yellow
Write-Host "  ✓ 如果所有测试通过，说明代理的工具调用功能正常" -ForegroundColor Green
Write-Host "  ✓ Cursor 应该能够正常使用工具调用功能" -ForegroundColor Green
Write-Host ""
Write-Host "如果 Cursor 仍然无法调用工具，请检查:" -ForegroundColor Yellow
Write-Host "  1. Cursor 版本是否支持工具调用 (>= 0.40.0)" -ForegroundColor Gray
Write-Host "  2. Cursor 是否在 Agent/Composer 模式下运行" -ForegroundColor Gray
Write-Host "  3. 查看数据库日志确认 Cursor 发送的请求格式" -ForegroundColor Gray
Write-Host ""
