package slackbot

import (
	"strings"
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
