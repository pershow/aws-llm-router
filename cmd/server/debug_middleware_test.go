package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeSSEDebugCollectsToolArguments(t *testing.T) {
	raw := strings.Join([]string{
		`data: {"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Write"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"README.md\", \"content\":\"hello"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":" world\"}"}}]},"finish_reason":null}]}`,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"length"}],"usage":{"prompt_tokens":10,"completion_tokens":4096,"total_tokens":4106}}`,
		`data: [DONE]`,
		"",
	}, "\n")

	analysis := analyzeSSEDebug(raw)
	if !analysis.hasToolCalls {
		t.Fatalf("expected hasToolCalls=true")
	}
	if analysis.finishReason != "length" {
		t.Fatalf("unexpected finishReason: %q", analysis.finishReason)
	}
	if analysis.completionTokens != 4096 {
		t.Fatalf("unexpected completionTokens: %d", analysis.completionTokens)
	}
	if len(analysis.toolCallNames) != 1 || analysis.toolCallNames[0] != "Write" {
		t.Fatalf("unexpected toolCallNames: %#v", analysis.toolCallNames)
	}
	gotArgs := analysis.toolCallArguments[0]
	wantArgs := `{"path":"README.md", "content":"hello world"}`
	if gotArgs != wantArgs {
		t.Fatalf("unexpected tool args: got=%q want=%q", gotArgs, wantArgs)
	}
}

func TestMaybeWriteToolTruncationWarningWritesFile(t *testing.T) {
	logDir := t.TempDir()
	var logBuffer bytes.Buffer
	logger := log.New(&logBuffer, "", 0)

	analysis := sseDebugAnalysis{
		hasToolCalls:      true,
		finishReason:      "length",
		toolCallNames:     []string{"Write"},
		completionTokens:  4096,
		toolCallArguments: map[int]string{0: `{"path":"README.md","content":"incomplete"`},
	}

	maybeWriteToolTruncationWarning(logger, logDir, "req_1", analysis, 4096, 0)

	warningFile := filepath.Join(logDir, "req_1_warning.log")
	contentBytes, err := os.ReadFile(warningFile)
	if err != nil {
		t.Fatalf("expected warning file to be written: %v", err)
	}
	content := string(contentBytes)
	if !strings.Contains(content, "tool-call truncation warning") {
		t.Fatalf("warning file missing header: %q", content)
	}
	if !strings.Contains(content, "request.max_tokens: 4096") {
		t.Fatalf("warning file missing request max tokens: %q", content)
	}
	if !strings.Contains(content, "invalid JSON") {
		t.Fatalf("warning file should include invalid JSON diagnostics: %q", content)
	}
}
