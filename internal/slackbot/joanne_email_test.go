package slackbot

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestResolveJoanneEmailRecipient_GrantFallbackWhenExplicitInvalid(t *testing.T) {
	b := &Bot{cfg: &config.Config{ChatAllowedUserID: "UCEO"}}
	got, source, err := b.resolveJoanneEmailRecipient(context.TODO(), "Erika", "UCEO")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != grantFallbackRecipientEmail {
		t.Fatalf("recipient mismatch: %q", got)
	}
	if source != "grant_user_fallback_invalid_explicit" {
		t.Fatalf("source mismatch: %q", source)
	}
}

func TestBuildJoanneEmailMissingInfoPrompt(t *testing.T) {
	b := &Bot{}
	gotBoth := b.buildJoanneEmailMissingInfoPrompt(true, true)
	if !strings.Contains(strings.ToLower(gotBoth), "who should receive") {
		t.Fatalf("expected recipient guidance, got %q", gotBoth)
	}
	if !strings.Contains(strings.ToLower(gotBoth), "outcome") {
		t.Fatalf("expected goal guidance, got %q", gotBoth)
	}

	gotRecipient := b.buildJoanneEmailMissingInfoPrompt(true, false)
	if !strings.Contains(strings.ToLower(gotRecipient), "recipient") {
		t.Fatalf("expected recipient-only prompt, got %q", gotRecipient)
	}

	gotGoal := b.buildJoanneEmailMissingInfoPrompt(false, true)
	if !strings.Contains(strings.ToLower(gotGoal), "goal") {
		t.Fatalf("expected goal-only prompt, got %q", gotGoal)
	}
}

func TestBuildJoanneEmailConfirmationPrompt(t *testing.T) {
	got := buildJoanneEmailConfirmationPrompt("grant@bimross.com", "Quick follow-up", "Confirm next-step timing")
	if !strings.Contains(got, "grant@bimross.com") {
		t.Fatalf("expected recipient in summary: %q", got)
	}
	if !strings.Contains(got, "Confirm next-step timing") {
		t.Fatalf("expected goal in summary: %q", got)
	}
	if !strings.Contains(got, "confirm send") {
		t.Fatalf("expected confirm instruction in summary: %q", got)
	}
}

func TestDeriveJoanneEmailGoal(t *testing.T) {
	goal := deriveJoanneEmailGoal(emailaction.SendEmailAction{
		BodyInstruction: "Ask for final approval by EOD.",
	}, "")
	if goal != "Ask for final approval by EOD." {
		t.Fatalf("unexpected goal from instruction: %q", goal)
	}

	bodyGoal := deriveJoanneEmailGoal(emailaction.SendEmailAction{
		BodyText: "Hi team, please confirm the rollout plan by 3pm. Thanks.",
	}, "")
	if !strings.Contains(bodyGoal, "rollout plan") {
		t.Fatalf("unexpected goal from body text: %q", bodyGoal)
	}
}

func TestJoanneEmailConfirmCancelText(t *testing.T) {
	if !isJoanneEmailConfirmText("confirm send") {
		t.Fatalf("expected confirm send to match")
	}
	if isJoanneEmailConfirmText("please send this now to bob") {
		t.Fatalf("did not expect broad sentence to match strict confirmation")
	}
	if !isJoanneEmailCancelText("cancel") {
		t.Fatalf("expected cancel to match")
	}
}

func TestJoannePendingEmailLifecycle(t *testing.T) {
	b := &Bot{
		joanneEmailPending: map[string]joannePendingEmail{},
	}
	b.setJoannePendingEmail("C1", "U1", "T1", joannePendingEmail{
		To:        "grant@bimross.com",
		Subject:   "Hello",
		Body:      "Body",
		Goal:      "Confirm timing",
		ThreadTS:  "T1",
		CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	})
	got, ok := b.getJoannePendingEmail("C1", "U1", "T1")
	if !ok {
		t.Fatalf("expected pending email")
	}
	if got.To != "grant@bimross.com" {
		t.Fatalf("pending recipient mismatch: %q", got.To)
	}

	b.clearJoannePendingEmail("C1", "U1", "T1")
	if _, ok := b.getJoannePendingEmail("C1", "U1", "T1"); ok {
		t.Fatalf("expected pending email to be cleared")
	}
}

func TestJoannePendingEmailExpires(t *testing.T) {
	b := &Bot{
		joanneEmailPending: map[string]joannePendingEmail{},
	}
	b.setJoannePendingEmail("C1", "U1", "", joannePendingEmail{
		To:        "grant@bimross.com",
		ExpiresAt: time.Now().UTC().Add(-1 * time.Minute),
	})
	if _, ok := b.getJoannePendingEmail("C1", "U1", ""); ok {
		t.Fatalf("expected expired pending email to be removed")
	}
}

func TestNormalizeJoannePlainText(t *testing.T) {
	raw := "```Hello team,\r\n\r\nThis is a draft.\r\n\r\n\r\nThanks,\r\nJoanne```"
	got := normalizeJoannePlainText(raw)
	want := "Hello team,\n\nThis is a draft.\n\nThanks,\nJoanne"
	if got != want {
		t.Fatalf("normalized body mismatch:\nwant=%q\ngot =%q", want, got)
	}
}
