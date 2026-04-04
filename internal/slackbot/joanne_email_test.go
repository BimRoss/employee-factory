package slackbot

import (
	"context"
	"testing"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/emailaction"
)

func TestNormalizeEmailAddress(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"grant@bimross.com", "grant@bimross.com"},
		{"<mailto:grant@bimross.com|grant@bimross.com>", "grant@bimross.com"},
		{"to: grant@bimross.com;", "grant@bimross.com"},
		{" grant@bimross.com, ", "grant@bimross.com"},
	}
	for _, tc := range tests {
		if got := normalizeEmailAddress(tc.in); got != tc.want {
			t.Fatalf("normalizeEmailAddress(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveJoanneEmailAction_ExtractorSendIntent(t *testing.T) {
	raw := "please email me, title: Reminder, body: Hello there."
	extract := joanneEmailActionExtract{
		Intent:   emailaction.IntentSendEmail,
		Subject:  "Reminder",
		BodyText: "Hello there.",
	}
	got, matched, err, source := resolveJoanneEmailAction(raw, extract, nil)
	if !matched {
		t.Fatalf("expected matched")
	}
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if source != "extractor" {
		t.Fatalf("source mismatch: %q", source)
	}
	if got.Subject != "Reminder" || got.BodyText != "Hello there." {
		t.Fatalf("unexpected mapped action: %+v", got)
	}
}

func TestResolveJoanneEmailAction_FallsBackToParser(t *testing.T) {
	raw := "send email; to: grant@bimross.com; body: hello"
	got, matched, err, source := resolveJoanneEmailAction(raw, joanneEmailActionExtract{}, assertErrSentinel{})
	if !matched {
		t.Fatalf("expected parser fallback match")
	}
	if err != nil {
		t.Fatalf("unexpected parser err: %v", err)
	}
	if source != "parser" {
		t.Fatalf("source mismatch: %q", source)
	}
	if got.To != "grant@bimross.com" {
		t.Fatalf("to mismatch: %q", got.To)
	}
}

type assertErrSentinel struct{}

func (assertErrSentinel) Error() string { return "forced extract failure" }

func TestShouldUseGrantRecipientFallback(t *testing.T) {
	b := &Bot{cfg: &config.Config{ChatAllowedUserID: "UCEO"}}
	if !b.shouldUseGrantRecipientFallback("UCEO", "me") {
		t.Fatalf("expected explicit me alias fallback")
	}
	if !b.shouldUseGrantRecipientFallback("UCEO", "") {
		t.Fatalf("expected implicit grant fallback for chat allowed user")
	}
	if b.shouldUseGrantRecipientFallback("UOTHER", "me") {
		t.Fatalf("did not expect fallback for non-ceo user id")
	}
}

func TestResolveJoanneEmailRecipient_GrantFallback(t *testing.T) {
	b := &Bot{cfg: &config.Config{ChatAllowedUserID: "UCEO"}}
	got, source, err := b.resolveJoanneEmailRecipient(context.TODO(), "", "UCEO")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != grantFallbackRecipientEmail {
		t.Fatalf("recipient mismatch: %q", got)
	}
	if source != "grant_user_fallback" {
		t.Fatalf("source mismatch: %q", source)
	}
}
