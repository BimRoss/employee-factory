package slackbot

import (
	"testing"

	"github.com/bimross/employee-factory/internal/opsproxy"
)

func TestResolveRossOpsAction_Extractor(t *testing.T) {
	extract := rossOpsActionExtract{
		Intent:    "ops_query",
		Operation: string(opsproxy.OperationK8sStatus),
		Namespace: "employee-factory",
		Target:    "deployment/employee-factory-ross",
		Limit:     5,
	}
	action, matched, source := resolveRossOpsAction("check deployed", extract, nil)
	if !matched {
		t.Fatalf("expected matched extractor action")
	}
	if source != "extractor" {
		t.Fatalf("source mismatch: %q", source)
	}
	if action.Operation != opsproxy.OperationK8sStatus {
		t.Fatalf("operation mismatch: %q", action.Operation)
	}
}

func TestResolveRossOpsAction_ParserFallback(t *testing.T) {
	action, matched, source := resolveRossOpsAction("check kubernetes deployment/employee-factory-ross", rossOpsActionExtract{}, assertErrSentinel{})
	if !matched {
		t.Fatalf("expected parser fallback match")
	}
	if source != "parser" {
		t.Fatalf("source mismatch: %q", source)
	}
	if action.Operation != opsproxy.OperationK8sStatus {
		t.Fatalf("operation mismatch: %q", action.Operation)
	}
}

func TestResolveRossOpsAction_WaitlistOverride(t *testing.T) {
	extract := rossOpsActionExtract{
		Intent:    "ops_query",
		Operation: string(opsproxy.OperationK8sStatus),
	}
	action, matched, source := resolveRossOpsAction("emails of people on waitlist on the server", extract, nil)
	if !matched {
		t.Fatalf("expected override match")
	}
	if source != "extractor_waitlist_override" {
		t.Fatalf("source mismatch: %q", source)
	}
	if action.Operation != opsproxy.OperationWaitlistEmails {
		t.Fatalf("operation mismatch: %q", action.Operation)
	}
}

func TestParseRossOpsAction_RedisDefaultPrefix(t *testing.T) {
	action, matched := parseRossOpsAction("check redis cache")
	if !matched {
		t.Fatalf("expected redis match")
	}
	if action.Operation != opsproxy.OperationRedisRead {
		t.Fatalf("operation mismatch: %q", action.Operation)
	}
	if action.RedisPrefix == "" {
		t.Fatalf("expected default redis prefix")
	}
}

func TestParseRossOpsAction_WaitlistEmails(t *testing.T) {
	action, matched := parseRossOpsAction("can you give me full waitlist emails from redis")
	if !matched {
		t.Fatalf("expected waitlist match")
	}
	if action.Operation != opsproxy.OperationWaitlistEmails {
		t.Fatalf("operation mismatch: %q", action.Operation)
	}
	if !action.RevealFull {
		t.Fatalf("expected reveal_full to be true")
	}
}

func TestParseRossOpsAction_WaitlistEmailsWithoutRedisKeyword(t *testing.T) {
	action, matched := parseRossOpsAction("show waitlist emails on the server")
	if !matched {
		t.Fatalf("expected waitlist match")
	}
	if action.Operation != opsproxy.OperationWaitlistEmails {
		t.Fatalf("operation mismatch: %q", action.Operation)
	}
}
