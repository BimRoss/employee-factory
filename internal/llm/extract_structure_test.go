package llm

import "testing"

func TestExtractJSONPayload_FromMarkdownFence(t *testing.T) {
	raw := "```json\n{\"intent\":\"send_email\",\"subject\":\"Reminder\"}\n```"
	got := extractJSONPayload(raw)
	if got == "" {
		t.Fatalf("expected payload from fenced block")
	}
}

func TestExtractJSONPayload_FromMixedText(t *testing.T) {
	raw := "Here is the output: {\"intent\":\"send_email\",\"body_text\":\"hello\"} thanks"
	got := extractJSONPayload(raw)
	want := "{\"intent\":\"send_email\",\"body_text\":\"hello\"}"
	if got != want {
		t.Fatalf("extractJSONPayload()=%q want %q", got, want)
	}
}
