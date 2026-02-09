package openai

import "testing"

func TestDecodeContentAsText_String(t *testing.T) {
	value, err := DecodeContentAsText([]byte(`"hello"`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if value != "hello" {
		t.Fatalf("unexpected value: %q", value)
	}
}

func TestDecodeContentAsText_Array(t *testing.T) {
	value, err := DecodeContentAsText([]byte(`[{"type":"text","text":"hi "},{"type":"text","text":"there"}]`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if value != "hi there" {
		t.Fatalf("unexpected value: %q", value)
	}
}
