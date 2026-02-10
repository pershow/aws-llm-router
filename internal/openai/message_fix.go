package openai

import (
	"fmt"
	"time"
)

// EnsureToolCallIDs ensures all tool_calls have valid IDs
func EnsureToolCallIDs(messages []ChatMessage) []ChatMessage {
	for i := range messages {
		msg := &messages[i]
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			for j := range msg.ToolCalls {
				tc := &msg.ToolCalls[j]
				if tc.ID == "" {
					// Generate unique ID
					tc.ID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), j)
				}
				if tc.Type == "" {
					tc.Type = "function"
				}
			}
		}
	}
	return messages
}

// FixMissingToolResponses inserts empty tool responses if assistant has tool_calls but no corresponding tool response
func FixMissingToolResponses(messages []ChatMessage) []ChatMessage {
	if len(messages) == 0 {
		return messages
	}

	newMessages := make([]ChatMessage, 0, len(messages))

	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		newMessages = append(newMessages, msg)

		// Check if this is assistant with tool_calls
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}

		// Get tool_call IDs
		toolCallIDs := make(map[string]bool)
		for _, tc := range msg.ToolCalls {
			if tc.ID != "" {
				toolCallIDs[tc.ID] = true
			}
		}

		if len(toolCallIDs) == 0 {
			continue
		}

		// Check if next message has tool responses
		hasToolResponse := false
		if i+1 < len(messages) {
			nextMsg := messages[i+1]
			if nextMsg.Role == "tool" && nextMsg.ToolCallID != "" {
				if toolCallIDs[nextMsg.ToolCallID] {
					hasToolResponse = true
				}
			}
		}

		// If no tool response found, insert empty responses
		if !hasToolResponse {
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" {
					newMessages = append(newMessages, ChatMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    []byte(`""`), // Empty string as JSON
					})
				}
			}
		}
	}

	return newMessages
}
