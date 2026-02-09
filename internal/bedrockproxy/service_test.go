package bedrockproxy

import "testing"

func TestResolveModelDirect(t *testing.T) {
	service := NewService(nil, "anthropic.default", map[string]string{
		"gpt-4o": "anthropic.claude-3-7-sonnet-20250219-v1:0",
	}, 2048)

	requested, bedrockID, err := service.ResolveModel("gpt-4o")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if requested != "gpt-4o" {
		t.Fatalf("unexpected requested: %s", requested)
	}
	if bedrockID != "gpt-4o" {
		t.Fatalf("unexpected bedrock id: %s", bedrockID)
	}
}

func TestResolveModelDefault(t *testing.T) {
	service := NewService(nil, "anthropic.default", nil, 2048)

	requested, bedrockID, err := service.ResolveModel("")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if requested != "default" || bedrockID != "anthropic.default" {
		t.Fatalf("unexpected model resolution: %s %s", requested, bedrockID)
	}
}
