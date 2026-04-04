package slackbot

import (
	"strings"
	"testing"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/opsproxy"
)

func TestRenderPlainText_UsesNormalizer(t *testing.T) {
	cfg := &config.Config{}
	got := RenderPlainText("**Bold**", cfg, "")
	if !strings.Contains(got, "*Bold*") {
		t.Fatalf("expected Slack single-asterisk bold, got %q", got)
	}
}

func TestRenderFencedJSON(t *testing.T) {
	out, err := RenderFencedJSON(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("render fenced json: %v", err)
	}
	if !strings.HasPrefix(out, "```json\n") {
		t.Fatalf("unexpected fenced prefix: %q", out)
	}
}

func TestRenderBlocks_WaitlistLatest(t *testing.T) {
	blocks, fallback, ok := RenderBlocks(ResponseKindOpsWaitlist, OpsWaitlistPresentation{
		QuestionType: waitlistQuestionLatest,
		Response: opsproxy.WaitlistEmailsResponse{
			Emails: []opsproxy.WaitlistEmail{
				{
					Email:     "a***@gmail.com",
					UpdatedAt: "2026-04-04T12:00:00Z",
				},
			},
		},
	}, PresentationOptions{MaxBlockItems: 8})
	if !ok {
		t.Fatal("expected waitlist blocks render to succeed")
	}
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
	if strings.TrimSpace(fallback) == "" {
		t.Fatal("expected fallback text")
	}
}

func TestRenderBlocks_Metrics(t *testing.T) {
	blocks, fallback, ok := RenderBlocks(ResponseKindOpsMetrics, OpsMetricsPresentation{
		Response: opsproxy.MetricsResponse{
			Namespace: "employee-factory",
			Cluster: opsproxy.ClusterResourceTotals{
				CPUUsageMilli:    500,
				MemoryUsageBytes: 2 * 1024 * 1024 * 1024,
			},
			Nodes: []opsproxy.NodeResourceMetrics{
				{NodeName: "admin", CPUUsageMilli: 240, MemoryUsageBytes: 1024 * 1024 * 1024},
			},
			LiveMetricsAvailable: true,
		},
	}, PresentationOptions{MaxBlockItems: 8})
	if !ok {
		t.Fatal("expected metrics blocks render to succeed")
	}
	if len(blocks) == 0 {
		t.Fatal("expected blocks")
	}
	if strings.TrimSpace(fallback) == "" {
		t.Fatal("expected fallback text")
	}
}
