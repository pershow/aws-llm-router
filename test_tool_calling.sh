#!/bin/bash
# 工具调用测试脚本 (Linux/Mac)
# 用于验证 AWS Cursor Router 的工具调用功能

BASE_URL="${BASE_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-}"
MODEL="${MODEL:-anthropic.claude-3-5-sonnet-20240620-v1:0}"

if [ -z "$API_KEY" ]; then
    echo "错误: 请提供 API Key"
    echo "使用方法: API_KEY='your-api-key' ./test_tool_calling.sh"
    exit 1
fi

echo "=== AWS Cursor Router 工具调用测试 ==="
echo "Base URL: $BASE_URL"
echo "Model: $MODEL"
echo ""

# 测试 1: 健康检查
echo "[测试 1] 健康检查..."
if curl -s -f "$BASE_URL/healthz" > /dev/null; then
    echo "✓ 服务正常运行"
else
    echo "✗ 服务不可用"
    exit 1
fi
echo ""

# 测试 2: 获取模型列表
echo "[测试 2] 获取模型列表..."
curl -s -X GET "$BASE_URL/v1/models" \
    -H "Authorization: Bearer $API_KEY" | jq -r '.data[].id' | head -5
echo ""

# 测试 3: 工具调用测试
echo "[测试 3] 工具调用测试..."
RESPONSE=$(curl -s -X POST "$BASE_URL/v1/chat/completions" \
    -H "Authorization: Bearer $API_KEY" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "'"$MODEL"'",
        "messages": [
            {
                "role": "user",
                "content": "What is the weather in San Francisco?"
            }
        ],
        "tools": [
            {
                "type": "function",
                "function": {
                    "name": "get_weather",
                    "description": "Get the current weather in a given location",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "location": {
                                "type": "string",
                                "description": "The city and state, e.g. San Francisco, CA"
                            },
                            "unit": {
                                "type": "string",
                                "enum": ["celsius", "fahrenheit"]
                            }
                        },
                        "required": ["location"]
                    }
                }
            }
        ],
        "tool_choice": "auto"
    }')

echo "$RESPONSE" | jq .

TOOL_CALLS=$(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls')
if [ "$TOOL_CALLS" != "null" ] && [ "$TOOL_CALLS" != "[]" ]; then
    echo "✓ 模型成功调用工具!"
    echo "  Tool Call ID: $(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls[0].id')"
    echo "  Function: $(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls[0].function.name')"
    echo "  Arguments: $(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls[0].function.arguments')"

    # 保存用于下一步测试
    TOOL_CALL_ID=$(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls[0].id')
    TOOL_FUNCTION=$(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls[0].function.name')
    TOOL_ARGUMENTS=$(echo "$RESPONSE" | jq -r '.choices[0].message.tool_calls[0].function.arguments')
else
    echo "✗ 模型没有调用工具，而是返回了文本响应"
    echo "  Response: $(echo "$RESPONSE" | jq -r '.choices[0].message.content')"
    exit 1
fi
echo ""

# 测试 4: 工具结果回传
echo "[测试 4] 工具结果回传测试..."
curl -s -X POST "$BASE_URL/v1/chat/completions" \
    -H "Authorization: Bearer $API_KEY" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "'"$MODEL"'",
        "messages": [
            {
                "role": "user",
                "content": "What is the weather in San Francisco?"
            },
            {
                "role": "assistant",
                "content": null,
                "tool_calls": [
                    {
                        "id": "'"$TOOL_CALL_ID"'",
                        "type": "function",
                        "function": {
                            "name": "'"$TOOL_FUNCTION"'",
                            "arguments": "'"$TOOL_ARGUMENTS"'"
                        }
                    }
                ]
            },
            {
                "role": "tool",
                "tool_call_id": "'"$TOOL_CALL_ID"'",
                "content": "{\"temperature\": 72, \"unit\": \"fahrenheit\", \"condition\": \"sunny\"}"
            }
        ]
    }' | jq -r '.choices[0].message.content'
echo ""

# 测试 5: 流式工具调用
echo "[测试 5] 流式工具调用测试..."
curl -N -X POST "$BASE_URL/v1/chat/completions" \
    -H "Authorization: Bearer $API_KEY" \
    -H "Content-Type: application/json" \
    -d '{
        "model": "'"$MODEL"'",
        "messages": [
            {
                "role": "user",
                "content": "What is the weather in Tokyo?"
            }
        ],
        "tools": [
            {
                "type": "function",
                "function": {
                    "name": "get_weather",
                    "description": "Get the current weather",
                    "parameters": {
                        "type": "object",
                        "properties": {
                            "location": {"type": "string"}
                        },
                        "required": ["location"]
                    }
                }
            }
        ],
        "stream": true
    }'
echo ""
echo ""

echo "=== 测试完成 ==="
echo ""
echo "如果所有测试通过，说明代理的工具调用功能正常"
echo "Cursor 应该能够正常使用工具调用功能"
echo ""
