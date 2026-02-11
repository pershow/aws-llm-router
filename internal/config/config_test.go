package config

import "testing"

func TestLoadReadsMinToolMaxOutputTokens(t *testing.T) {
	t.Setenv("MIN_TOOL_MAX_OUTPUT_TOKENS", "9000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.MinToolMaxOutputToken != 9000 {
		t.Fatalf("unexpected MinToolMaxOutputToken: %d", cfg.MinToolMaxOutputToken)
	}
}

func TestLoadRejectsNegativeMinToolMaxOutputTokens(t *testing.T) {
	t.Setenv("MIN_TOOL_MAX_OUTPUT_TOKENS", "-1")

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error for negative MIN_TOOL_MAX_OUTPUT_TOKENS")
	}
}
