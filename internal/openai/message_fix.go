package openai

import (
	"encoding/json"
	"fmt"
	"strings"
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

		// Get tool_call IDs.
		toolCallIDs := make(map[string]struct{}, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			if id := strings.TrimSpace(tc.ID); id != "" {
				toolCallIDs[id] = struct{}{}
			}
		}

		if len(toolCallIDs) == 0 {
			continue
		}

		// Scan subsequent non-assistant messages for tool responses.
		for j := i + 1; j < len(messages) && len(toolCallIDs) > 0; j++ {
			nextMsg := messages[j]
			if strings.EqualFold(strings.TrimSpace(nextMsg.Role), "assistant") {
				break
			}
			for _, toolCallID := range extractToolResponseIDs(nextMsg) {
				delete(toolCallIDs, toolCallID)
			}
		}

		// If some tool responses are missing, insert empty responses for the missing IDs only.
		if len(toolCallIDs) > 0 {
			for _, tc := range msg.ToolCalls {
				toolCallID := strings.TrimSpace(tc.ID)
				if toolCallID == "" {
					continue
				}
				if _, exists := toolCallIDs[toolCallID]; !exists {
					continue
				}
				newMessages = append(newMessages, ChatMessage{
					Role:       "tool",
					ToolCallID: toolCallID,
					Content:    []byte(`""`), // Empty string as JSON
				})
			}
		}
	}

	return newMessages
}

func extractToolResponseIDs(message ChatMessage) []string {
	ids := make([]string, 0, 2)

	if strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
		if toolCallID := strings.TrimSpace(message.ToolCallID); toolCallID != "" {
			ids = append(ids, toolCallID)
		}
	}

	return appendUniqueToolCallIDs(ids, extractInlineToolResultIDs(message.Content))
}

func extractInlineToolResultIDs(raw json.RawMessage) []string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}

	switch trimmed[0] {
	case '[':
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil
		}
		ids := make([]string, 0, len(entries))
		for _, entry := range entries {
			ids = appendUniqueToolCallIDs(ids, extractInlineToolResultIDs(entry))
		}
		return ids
	case '{':
		var item map[string]json.RawMessage
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil
		}
		itemType := strings.ToLower(strings.TrimSpace(jsonString(item["type"])))
		if itemType != "tool_result" && itemType != "function_call_output" && itemType != "function_result" {
			return nil
		}

		toolCallID := strings.TrimSpace(jsonString(item["tool_use_id"]))
		if toolCallID == "" {
			toolCallID = strings.TrimSpace(jsonString(item["tool_call_id"]))
		}
		if toolCallID == "" {
			toolCallID = strings.TrimSpace(jsonString(item["call_id"]))
		}
		if toolCallID == "" {
			toolCallID = strings.TrimSpace(jsonString(item["id"]))
		}
		if toolCallID == "" {
			return nil
		}
		return []string{toolCallID}
	default:
		return nil
	}
}

func appendUniqueToolCallIDs(dst []string, ids []string) []string {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		duplicate := false
		for _, existing := range dst {
			if existing == id {
				duplicate = true
				break
			}
		}
		if !duplicate {
			dst = append(dst, id)
		}
	}
	return dst
}
