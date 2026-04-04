package emailaction

import "testing"

func TestParseSendEmailAction_NoMatch(t *testing.T) {
	_, matched, err := ParseSendEmailAction("what should we prioritize this week?")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if matched {
		t.Fatalf("expected no send-email match")
	}
}

func TestParseSendEmailAction_KeyValueFields(t *testing.T) {
	raw := "send email; to: grant@bimross.com; subject: Quick update; instruction: let him know we shipped phase one"
	got, matched, err := ParseSendEmailAction(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !matched {
		t.Fatalf("expected match")
	}
	if got.Intent != IntentSendEmail {
		t.Fatalf("intent mismatch: %s", got.Intent)
	}
	if got.To != "grant@bimross.com" {
		t.Fatalf("to mismatch: %s", got.To)
	}
	if got.Subject != "Quick update" {
		t.Fatalf("subject mismatch: %s", got.Subject)
	}
	if got.BodyInstruction == "" {
		t.Fatalf("expected body instruction")
	}
}

func TestParseSendEmailAction_InfersEmailAndBodyText(t *testing.T) {
	raw := "send an email to grant@bimross.com; body: Hi Grant, shipping now."
	got, matched, err := ParseSendEmailAction(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !matched {
		t.Fatalf("expected match")
	}
	if got.To != "grant@bimross.com" {
		t.Fatalf("expected inferred email, got %q", got.To)
	}
	if got.BodyText != "Hi Grant, shipping now." {
		t.Fatalf("body text mismatch: %q", got.BodyText)
	}
}

func TestParseSendEmailAction_RequiresBody(t *testing.T) {
	_, matched, err := ParseSendEmailAction("send email to: grant@bimross.com")
	if !matched {
		t.Fatalf("expected match")
	}
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestParseSendEmailAction_EmailMeWithTitleAndBody(t *testing.T) {
	raw := "please email me, title: Reminder, body: Tell your wife you love her."
	got, matched, err := ParseSendEmailAction(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !matched {
		t.Fatalf("expected match")
	}
	if got.Subject != "Reminder" {
		t.Fatalf("title alias should map to subject, got %q", got.Subject)
	}
	if got.BodyText != "Tell your wife you love her." {
		t.Fatalf("body text mismatch: %q", got.BodyText)
	}
}

func TestParseSendEmailAction_NewlineThreadStyle(t *testing.T) {
	raw := "email me\nsubject: Reminder\nbody: Please remember this."
	got, matched, err := ParseSendEmailAction(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !matched {
		t.Fatalf("expected match")
	}
	if got.Subject != "Reminder" {
		t.Fatalf("subject mismatch: %q", got.Subject)
	}
	if got.BodyText != "Please remember this." {
		t.Fatalf("body mismatch: %q", got.BodyText)
	}
}

func TestParseSendEmailAction_PleaseEmailWithExplicitRecipient(t *testing.T) {
	raw := "please email grant@bimross.com, title: Reminder, body: You got this."
	got, matched, err := ParseSendEmailAction(raw)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !matched {
		t.Fatalf("expected match")
	}
	if got.To != "grant@bimross.com" {
		t.Fatalf("to mismatch: %q", got.To)
	}
	if got.Subject != "Reminder" {
		t.Fatalf("subject mismatch: %q", got.Subject)
	}
	if got.BodyText != "You got this." {
		t.Fatalf("body mismatch: %q", got.BodyText)
	}
}
