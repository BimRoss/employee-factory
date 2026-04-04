package slackbot

import (
	"strings"
	"testing"

	"github.com/bimross/employee-factory/internal/opsproxy"
)

func TestInferWaitlistQuestionType(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want waitlistQuestionType
	}{
		{
			name: "latest signup ask",
			in:   "who was the last person to sign up on the waitlist?",
			want: waitlistQuestionLatest,
		},
		{
			name: "count ask",
			in:   "how many waitlist emails do we have?",
			want: waitlistQuestionCount,
		},
		{
			name: "list ask default",
			in:   "show the waitlist emails",
			want: waitlistQuestionList,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inferWaitlistQuestionType(tc.in)
			if got != tc.want {
				t.Fatalf("question type mismatch: got=%s want=%s", got, tc.want)
			}
		})
	}
}

func TestResolveRossOpsAction_ExtractorWaitlistOverrideSetsQuestionType(t *testing.T) {
	extract := rossOpsActionExtract{
		Intent:       "ops_query",
		Operation:    string(opsproxy.OperationK8sStatus),
		QuestionType: "",
	}
	action, matched, source := resolveRossOpsAction("who was the latest waitlist email signup?", extract, nil)
	if !matched {
		t.Fatal("expected waitlist prompt to match")
	}
	if source != "extractor_waitlist_override" {
		t.Fatalf("source mismatch: got=%q", source)
	}
	if action.Operation != opsproxy.OperationWaitlistEmails {
		t.Fatalf("operation mismatch: got=%s", action.Operation)
	}
	if action.QuestionType != waitlistQuestionLatest {
		t.Fatalf("question type mismatch: got=%s", action.QuestionType)
	}
}

func TestBuildWaitlistAnswer_LatestSignupSingleLine(t *testing.T) {
	resp := opsproxy.WaitlistEmailsResponse{
		Emails: []opsproxy.WaitlistEmail{
			{
				Email:     "a***@gmail.com",
				UpdatedAt: "2026-04-04T12:00:00Z",
				SourceKey: "makeacompany:waitlist:a@gmail.com",
			},
			{
				Email:     "b***@gmail.com",
				UpdatedAt: "2026-04-04T11:00:00Z",
				SourceKey: "makeacompany:waitlist:b@gmail.com",
			},
		},
	}
	answer, mode, ordering := buildWaitlistAnswer(waitlistQuestionLatest, "latest waitlist signup", resp, false)
	if mode != "deterministic" {
		t.Fatalf("mode mismatch: got=%q", mode)
	}
	if ordering != "timestamp_sorted" {
		t.Fatalf("ordering mismatch: got=%q", ordering)
	}
	if !strings.Contains(answer, "latest waitlist signup") || !strings.Contains(answer, "`a***@gmail.com`") {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if strings.Contains(answer, "\n- `") {
		t.Fatalf("expected direct answer, got list output: %q", answer)
	}
}

func TestBuildWaitlistAnswer_LatestSignupWithoutTimestamp(t *testing.T) {
	resp := opsproxy.WaitlistEmailsResponse{
		Emails: []opsproxy.WaitlistEmail{
			{
				Email: "a***@gmail.com",
			},
		},
	}
	answer, _, ordering := buildWaitlistAnswer(waitlistQuestionLatest, "latest waitlist signup", resp, false)
	if ordering != "timestamp_missing" {
		t.Fatalf("ordering mismatch: got=%q", ordering)
	}
	if !strings.Contains(strings.ToLower(answer), "cannot confirm") {
		t.Fatalf("expected limitation message, got %q", answer)
	}
}

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

func TestResolveRossOpsAction_ExtractorMetrics(t *testing.T) {
	extract := rossOpsActionExtract{
		Intent:    "ops_query",
		Operation: string(opsproxy.OperationK8sMetrics),
		Namespace: "employee-factory",
		Limit:     4,
	}
	action, matched, source := resolveRossOpsAction("cpu ram capacity metrics for kubernetes", extract, nil)
	if !matched {
		t.Fatalf("expected matched extractor action")
	}
	if source != "extractor" {
		t.Fatalf("source mismatch: %q", source)
	}
	if action.Operation != opsproxy.OperationK8sMetrics {
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

func TestParseRossOpsAction_K8sMetrics(t *testing.T) {
	action, matched := parseRossOpsAction("give me kubernetes cpu ram capacity metrics for admin cluster")
	if !matched {
		t.Fatalf("expected k8s metrics match")
	}
	if action.Operation != opsproxy.OperationK8sMetrics {
		t.Fatalf("operation mismatch: %q", action.Operation)
	}
}

func TestFormatRossMetrics_LiveUnavailable(t *testing.T) {
	text := formatRossMetrics(opsproxy.MetricsResponse{
		Namespace:            "employee-factory",
		LiveMetricsAvailable: false,
		LiveMetricsReason:    "metrics API unavailable",
		Cluster: opsproxy.ClusterResourceTotals{
			CPUCapacityMilli:       8000,
			CPUAllocatableMilli:    7600,
			CPURequestedMilli:      2400,
			MemoryCapacityBytes:    16 * 1024 * 1024 * 1024,
			MemoryAllocatableBytes: 15 * 1024 * 1024 * 1024,
			MemoryRequestedBytes:   4 * 1024 * 1024 * 1024,
		},
	})
	if !strings.Contains(text, "live usage unavailable") {
		t.Fatalf("expected unavailable marker in output: %q", text)
	}
	if !strings.Contains(text, "cluster cpu cap=") {
		t.Fatalf("expected cluster cpu summary in output: %q", text)
	}
}

func TestFormatRossWaitlistEmails_EmptyIncludesSearchedPrefixes(t *testing.T) {
	text := formatRossWaitlistEmails(opsproxy.WaitlistEmailsResponse{
		SearchedPrefixes: []string{"makeacompany:waitlist:", "waitlist:"},
	}, false)
	if !strings.Contains(text, "makeacompany:waitlist:") {
		t.Fatalf("expected searched prefixes in output: %q", text)
	}
}
