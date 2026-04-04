package slackbot

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/opsproxy"
	"github.com/slack-go/slack"
)

type OpsWaitlistPresentation struct {
	QuestionType waitlistQuestionType
	Response     opsproxy.WaitlistEmailsResponse
	RevealFull   bool
}

type OpsMetricsPresentation struct {
	Response opsproxy.MetricsResponse
}

func RenderPlainText(body string, cfg *config.Config, selfSlackUserID string) string {
	return normalizeSlackReply(body, cfg, selfSlackUserID)
}

func RenderFencedJSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return "```json\n" + string(b) + "\n```", nil
}

func RenderBlocks(kind ResponseKind, payload any, opts PresentationOptions) ([]slack.Block, string, bool) {
	switch kind {
	case ResponseKindOpsWaitlist:
		p, ok := payload.(OpsWaitlistPresentation)
		if !ok {
			return nil, "", false
		}
		return renderWaitlistBlocks(p, opts)
	case ResponseKindOpsMetrics:
		p, ok := payload.(OpsMetricsPresentation)
		if !ok {
			return nil, "", false
		}
		return renderMetricsBlocks(p, opts)
	default:
		return nil, "", false
	}
}

func renderWaitlistBlocks(p OpsWaitlistPresentation, opts PresentationOptions) ([]slack.Block, string, bool) {
	emails := p.Response.Emails
	if len(emails) == 0 {
		fallback := formatRossWaitlistEmails(p.Response, p.RevealFull)
		return nil, fallback, false
	}
	maxItems := opts.MaxBlockItems
	if maxItems <= 0 {
		maxItems = 8
	}
	if len(emails) > maxItems {
		emails = emails[:maxItems]
	}

	fallback, _, _ := buildWaitlistAnswer(p.QuestionType, "", p.Response, p.RevealFull)
	header := "*Waitlist snapshot*"
	summary := fmt.Sprintf("records: *%d*", len(p.Response.Emails))

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", header, false, false),
			nil,
			nil,
		),
		slack.NewContextBlock("waitlist-summary",
			slack.NewTextBlockObject("mrkdwn", summary, false, false),
		),
	}

	switch p.QuestionType {
	case waitlistQuestionLatest:
		first := p.Response.Emails[0]
		line := fmt.Sprintf("*Latest:* `%s`", strings.TrimSpace(first.Email))
		if ts := strings.TrimSpace(first.UpdatedAt); ts != "" {
			line += fmt.Sprintf(" (updated `%s`)", ts)
		}
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", line, false, false),
			nil,
			nil,
		))
	default:
		lines := make([]string, 0, len(emails))
		for _, row := range emails {
			line := "- `" + strings.TrimSpace(row.Email) + "`"
			if ts := strings.TrimSpace(row.UpdatedAt); ts != "" {
				line += " (`" + ts + "`)"
			}
			lines = append(lines, line)
		}
		body := strings.Join(lines, "\n")
		if len(body) > 2900 {
			body = body[:2900] + "..."
		}
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", body, false, false),
			nil,
			nil,
		))
	}
	return blocks, fallback, true
}

func renderMetricsBlocks(p OpsMetricsPresentation, opts PresentationOptions) ([]slack.Block, string, bool) {
	resp := p.Response
	if len(resp.Nodes) == 0 {
		return nil, formatRossMetrics(resp), false
	}
	maxItems := opts.MaxBlockItems
	if maxItems <= 0 {
		maxItems = 8
	}
	nodes := resp.Nodes
	if len(nodes) > maxItems {
		nodes = nodes[:maxItems]
	}

	fallback := formatRossMetrics(resp)
	live := "unavailable"
	if resp.LiveMetricsAvailable {
		live = "metrics.k8s.io"
	}

	blocks := []slack.Block{
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("*Kubernetes CPU/RAM metrics* `%s`", strings.TrimSpace(resp.Namespace)),
				false, false),
			nil,
			nil,
		),
		slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("*Cluster* cpu usage=%.2f cores, mem usage=%.2f Gi (live: %s)",
					milliToCores(resp.Cluster.CPUUsageMilli),
					bytesToGi(resp.Cluster.MemoryUsageBytes),
					live,
				),
				false, false),
			nil,
			nil,
		),
	}

	lines := make([]string, 0, len(nodes))
	for _, node := range nodes {
		lines = append(lines, fmt.Sprintf("- *%s* cpu=%.2f mem=%.2fGi",
			node.NodeName,
			milliToCores(node.CPUUsageMilli),
			bytesToGi(node.MemoryUsageBytes),
		))
	}
	body := strings.Join(lines, "\n")
	if len(body) > 2900 {
		body = body[:2900] + "..."
	}
	blocks = append(blocks, slack.NewSectionBlock(
		slack.NewTextBlockObject("mrkdwn", body, false, false),
		nil,
		nil,
	))
	return blocks, fallback, true
}
