package gmailsender

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBuildRawMessage_WithSenderName(t *testing.T) {
	raw := buildRawMessage("Joanne", "joanne@bimross.com", "grant@bimross.com", "Test", "Hello")
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	msg := string(decoded)
	if !strings.Contains(msg, "From: \"Joanne\" <joanne@bimross.com>\r\n") {
		t.Fatalf("missing named From header: %q", msg)
	}
}

func TestBuildRawMessage_NoSenderName(t *testing.T) {
	raw := buildRawMessage("", "joanne@bimross.com", "grant@bimross.com", "Test", "Hello")
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	msg := string(decoded)
	if !strings.Contains(msg, "From: joanne@bimross.com\r\n") {
		t.Fatalf("missing plain From header: %q", msg)
	}
}

func TestParseSendResultJSON(t *testing.T) {
	got := parseSendResultJSON([]byte(`{"id":"abc123","threadId":"th999"}`))
	if got.MessageID != "abc123" {
		t.Fatalf("message id mismatch: %q", got.MessageID)
	}
	if got.ThreadID != "th999" {
		t.Fatalf("thread id mismatch: %q", got.ThreadID)
	}
}
