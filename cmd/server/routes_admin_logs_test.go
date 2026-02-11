package main

import (
	"path/filepath"
	"testing"
)

func TestIsDebugLogFilename(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{name: "a.txt", ok: true},
		{name: "a.json", ok: true},
		{name: "a.log", ok: true},
		{name: "a.csv", ok: false},
		{name: "../a.txt", ok: false},
		{name: `..\\a.txt`, ok: false},
		{name: "dir/a.txt", ok: false},
		{name: "", ok: false},
	}

	for _, tc := range cases {
		got := isDebugLogFilename(tc.name)
		if got != tc.ok {
			t.Fatalf("isDebugLogFilename(%q) = %v, want %v", tc.name, got, tc.ok)
		}
	}
}

func TestResolveDebugLogFilePath(t *testing.T) {
	logDir := t.TempDir()

	validPath, err := resolveDebugLogFilePath(logDir, "x_response.txt")
	if err != nil {
		t.Fatalf("resolveDebugLogFilePath(valid) returned error: %v", err)
	}
	want := filepath.Join(logDir, "x_response.txt")
	if validPath != want {
		t.Fatalf("unexpected path: got=%q want=%q", validPath, want)
	}

	if _, err := resolveDebugLogFilePath(logDir, "../x_response.txt"); err == nil {
		t.Fatalf("expected error for path traversal")
	}
	if _, err := resolveDebugLogFilePath(logDir, "x_response.csv"); err == nil {
		t.Fatalf("expected error for unsupported extension")
	}
}
