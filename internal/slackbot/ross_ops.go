package slackbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bimross/employee-factory/internal/opsproxy"
	"github.com/sashabaranov/go-openai/jsonschema"
	"github.com/slack-go/slack"
)

type rossOpsActionExtract struct {
	Intent       string  `json:"intent"`
	Operation    string  `json:"operation,omitempty"`
	Namespace    string  `json:"namespace,omitempty"`
	Target       string  `json:"target,omitempty"`
	Container    string  `json:"container,omitempty"`
	RedisKey     string  `json:"redis_key,omitempty"`
	RedisPrefix  string  `json:"redis_prefix,omitempty"`
	TailLines    int64   `json:"tail_lines,omitempty"`
	SinceSeconds int64   `json:"since_seconds,omitempty"`
	Limit        int     `json:"limit,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

type rossOpsAction struct {
	Operation    opsproxy.Operation
	Namespace    string
	Target       string
	Container    string
	RedisKey     string
	RedisPrefix  string
	TailLines    int64
	SinceSeconds int64
	Limit        int
}

func rossOpsActionSchema() jsonschema.Definition {
	return jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"intent": {
				Type:        jsonschema.String,
				Enum:        []string{"ops_query", "none"},
				Description: "Set ops_query only when user asks for cluster, server, deploy, logs, cache, or redis data.",
			},
			"operation": {
				Type:        jsonschema.String,
				Enum:        []string{string(opsproxy.OperationK8sStatus), string(opsproxy.OperationK8sLogs), string(opsproxy.OperationRedisRead)},
				Description: "Desired read-only operation.",
			},
			"namespace": {
				Type:        jsonschema.String,
				Description: "Kubernetes namespace if specified by the user.",
			},
			"target": {
				Type:        jsonschema.String,
				Description: "Kubernetes target like deployment/<name> or pod/<name>.",
			},
			"container": {
				Type:        jsonschema.String,
				Description: "Container name for pod log reads.",
			},
			"redis_key": {
				Type:        jsonschema.String,
				Description: "Exact redis key requested.",
			},
			"redis_prefix": {
				Type:        jsonschema.String,
				Description: "Redis key prefix requested.",
			},
			"tail_lines": {
				Type:        jsonschema.Number,
				Description: "Requested log tail line count.",
			},
			"since_seconds": {
				Type:        jsonschema.Number,
				Description: "Optional log lookback in seconds.",
			},
			"limit": {
				Type:        jsonschema.Number,
				Description: "Result size limit.",
			},
			"confidence": {
				Type:        jsonschema.Number,
				Description: "Confidence score 0.0 to 1.0.",
			},
			"reason": {
				Type:        jsonschema.String,
				Description: "Brief reason for classification.",
			},
		},
		Required: []string{"intent"},
	}
}

func (b *Bot) tryHandleRossOps(ctx context.Context, channel, rawText, messageTS, threadTS string) bool {
	if b == nil || b.cfg == nil {
		return false
	}
	if !b.cfg.RossOpsEnabled || !strings.EqualFold(strings.TrimSpace(b.cfg.EmployeeID), "ross") {
		return false
	}
	cmd := strings.TrimSpace(rawText)
	if b.botUserID != "" {
		cmd = strings.TrimSpace(strings.ReplaceAll(cmd, "<@"+b.botUserID+">", ""))
	}
	extract, extractErr := b.extractRossOpsAction(ctx, cmd)
	action, matched, source := resolveRossOpsAction(cmd, extract, extractErr)
	if !matched {
		return false
	}
	log.Printf(
		"ross_ops: accepted message_ts=%s source=%s operation=%s confidence=%.2f extract_err=%t reason=%q",
		strings.TrimSpace(messageTS),
		source,
		action.Operation,
		extract.Confidence,
		extractErr != nil,
		strings.TrimSpace(extract.Reason),
	)
	if b.cfg.RossOpsLogOnly {
		log.Printf("ross_ops: log_only=true message_ts=%s source=%s operation=%s", strings.TrimSpace(messageTS), source, action.Operation)
		return false
	}
	go b.handleRossOpsSafely(ctx, channel, threadTS, messageTS, action)
	return true
}

func resolveRossOpsAction(raw string, extract rossOpsActionExtract, extractErr error) (rossOpsAction, bool, string) {
	if extractErr == nil && strings.EqualFold(strings.TrimSpace(extract.Intent), "ops_query") {
		action := rossOpsAction{
			Operation:    opsproxy.Operation(strings.TrimSpace(extract.Operation)),
			Namespace:    strings.TrimSpace(extract.Namespace),
			Target:       strings.TrimSpace(extract.Target),
			Container:    strings.TrimSpace(extract.Container),
			RedisKey:     strings.TrimSpace(extract.RedisKey),
			RedisPrefix:  strings.TrimSpace(extract.RedisPrefix),
			TailLines:    extract.TailLines,
			SinceSeconds: extract.SinceSeconds,
			Limit:        extract.Limit,
		}
		if action.Operation == opsproxy.OperationK8sStatus || action.Operation == opsproxy.OperationK8sLogs || action.Operation == opsproxy.OperationRedisRead {
			return action, true, "extractor"
		}
	}
	if action, ok := parseRossOpsAction(raw); ok {
		return action, true, "parser"
	}
	return rossOpsAction{}, false, "none"
}

func parseRossOpsAction(raw string) (rossOpsAction, bool) {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return rossOpsAction{}, false
	}
	action := rossOpsAction{}
	if strings.Contains(text, "redis") || strings.Contains(text, "cache key") || strings.Contains(text, "cache") {
		action.Operation = opsproxy.OperationRedisRead
		if idx := strings.Index(text, "key "); idx >= 0 {
			action.RedisKey = strings.TrimSpace(raw[idx+len("key "):])
		}
		if action.RedisKey == "" {
			action.RedisPrefix = "thread_owner:"
		}
		return action, true
	}
	if strings.Contains(text, "log") {
		action.Operation = opsproxy.OperationK8sLogs
		action.Target = parseTargetHint(text)
		action.TailLines = 200
		return action, true
	}
	if strings.Contains(text, "kubernetes") || strings.Contains(text, "server") || strings.Contains(text, "deployed") || strings.Contains(text, "deployment") || strings.Contains(text, "pods") {
		action.Operation = opsproxy.OperationK8sStatus
		action.Target = parseTargetHint(text)
		action.Limit = 8
		return action, true
	}
	return rossOpsAction{}, false
}

func parseTargetHint(text string) string {
	t := strings.TrimSpace(text)
	for _, marker := range []string{"deployment/", "pod/"} {
		if idx := strings.Index(t, marker); idx >= 0 {
			rest := t[idx:]
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				return strings.TrimSpace(fields[0])
			}
		}
	}
	return ""
}

func (b *Bot) extractRossOpsAction(ctx context.Context, cmdText string) (rossOpsActionExtract, error) {
	var out rossOpsActionExtract
	extractCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	systemPrompt := "You classify whether a Slack message is a Ross operations data request. Respond only with schema-compliant JSON."
	userPrompt := "Message:\n" + strings.TrimSpace(cmdText) + "\n\nRules:\n- intent=ops_query only for explicit requests to inspect infrastructure/cache state.\n- operation must be one of k8s_status, k8s_logs, redis_read.\n- If unsure, use intent=none.\n- Never invent namespaces, targets, keys, or logs."
	err := b.llm.ExtractStructured(extractCtx, systemPrompt, userPrompt, rossOpsActionSchema(), &out, "ross_ops_query")
	if err != nil {
		return rossOpsActionExtract{}, err
	}
	out.Intent = strings.ToLower(strings.TrimSpace(out.Intent))
	out.Operation = strings.TrimSpace(out.Operation)
	out.Namespace = strings.TrimSpace(out.Namespace)
	out.Target = strings.TrimSpace(out.Target)
	out.Container = strings.TrimSpace(out.Container)
	out.RedisKey = strings.TrimSpace(out.RedisKey)
	out.RedisPrefix = strings.TrimSpace(out.RedisPrefix)
	out.Reason = strings.TrimSpace(out.Reason)
	if out.Confidence < 0 {
		out.Confidence = 0
	}
	if out.Confidence > 1 {
		out.Confidence = 1
	}
	return out, nil
}

func (b *Bot) handleRossOpsSafely(parent context.Context, channel, threadTS, messageTS string, action rossOpsAction) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ross_ops: panic recovered message_ts=%s panic=%v", strings.TrimSpace(messageTS), r)
		}
	}()
	b.handleRossOps(parent, channel, threadTS, action)
}

func (b *Bot) handleRossOps(parent context.Context, channel, threadTS string, action rossOpsAction) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	if b.opsProxyClient == nil {
		b.postRossOpsStatus(ctx, channel, threadTS, "Ops tooling is enabled but proxy client is not configured yet.")
		return
	}
	namespace := strings.TrimSpace(action.Namespace)
	if namespace == "" {
		namespace = b.cfg.RossOpsDefaultNamespace
	}
	if action.Operation != opsproxy.OperationRedisRead && namespace != "" && !containsValue(b.cfg.RossOpsAllowedNamespaces, namespace) {
		b.postRossOpsStatus(ctx, channel, threadTS, fmt.Sprintf("Namespace `%s` is outside the Ross ops allowlist.", namespace))
		return
	}
	switch action.Operation {
	case opsproxy.OperationK8sStatus:
		resp, err := b.opsProxyClient.Status(ctx, opsproxy.StatusRequest{
			Namespace: namespace,
			Target:    action.Target,
			Limit:     action.Limit,
		})
		if err != nil {
			b.postRossOpsStatus(ctx, channel, threadTS, "I could not fetch Kubernetes status from the ops proxy.")
			return
		}
		b.postRossOpsStatus(ctx, channel, threadTS, formatRossStatus(resp))
	case opsproxy.OperationK8sLogs:
		resp, err := b.opsProxyClient.Logs(ctx, opsproxy.LogsRequest{
			Namespace:    namespace,
			Target:       action.Target,
			Container:    action.Container,
			TailLines:    action.TailLines,
			SinceSeconds: action.SinceSeconds,
		})
		if err != nil {
			b.postRossOpsStatus(ctx, channel, threadTS, "I could not fetch logs from the ops proxy.")
			return
		}
		text := strings.TrimSpace(resp.Lines)
		if text == "" {
			text = "(no log lines returned)"
		}
		if resp.Truncated {
			text += "\n…truncated"
		}
		b.postRossOpsStatus(ctx, channel, threadTS, fmt.Sprintf("Logs `%s` in `%s`:\n```%s```", resp.Target, resp.Namespace, text))
	case opsproxy.OperationRedisRead:
		if action.RedisKey != "" && !prefixAllowed(action.RedisKey, b.cfg.RossOpsAllowedRedisPrefixes) {
			b.postRossOpsStatus(ctx, channel, threadTS, "That redis key is outside the Ross ops allowlist.")
			return
		}
		if action.RedisPrefix != "" && !prefixAllowed(action.RedisPrefix, b.cfg.RossOpsAllowedRedisPrefixes) {
			b.postRossOpsStatus(ctx, channel, threadTS, "That redis prefix is outside the Ross ops allowlist.")
			return
		}
		resp, err := b.opsProxyClient.RedisRead(ctx, opsproxy.RedisReadRequest{
			Key:    action.RedisKey,
			Prefix: action.RedisPrefix,
			Limit:  action.Limit,
		})
		if err != nil {
			b.postRossOpsStatus(ctx, channel, threadTS, "I could not read Redis from the ops proxy.")
			return
		}
		b.postRossOpsStatus(ctx, channel, threadTS, formatRossRedis(resp))
	default:
		b.postRossOpsStatus(ctx, channel, threadTS, "I can run `k8s_status`, `k8s_logs`, or `redis_read` once the request is specific.")
	}
}

func formatRossStatus(resp opsproxy.StatusResponse) string {
	lines := []string{fmt.Sprintf("Kubernetes status `%s`", resp.Namespace)}
	for _, dep := range resp.Deployments {
		lines = append(lines, fmt.Sprintf("- deploy/%s ready %d/%d image=%s", dep.Name, dep.ReadyReplicas, dep.Replicas, strings.Join(dep.Images, ",")))
	}
	for _, pod := range resp.Pods {
		ready := "not-ready"
		if pod.Ready {
			ready = "ready"
		}
		lines = append(lines, fmt.Sprintf("- pod/%s phase=%s %s restarts=%d", pod.Name, pod.Phase, ready, pod.Restarts))
	}
	if len(lines) == 1 {
		lines = append(lines, "- no matching resources")
	}
	return strings.Join(lines, "\n")
}

func formatRossRedis(resp opsproxy.RedisReadResponse) string {
	if len(resp.Items) == 0 {
		return "Redis read returned no matching keys."
	}
	lines := []string{"Redis results:"}
	for _, item := range resp.Items {
		lines = append(lines, fmt.Sprintf("- `%s` (%s): %s", item.Key, item.Type, item.Value))
	}
	return strings.Join(lines, "\n")
}

func (b *Bot) postRossOpsStatus(ctx context.Context, channel, threadTS, text string) {
	postCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	opts := []slack.MsgOption{slack.MsgOptionText(strings.TrimSpace(text), false)}
	if strings.TrimSpace(threadTS) != "" {
		opts = append(opts, slack.MsgOptionTS(strings.TrimSpace(threadTS)))
	}
	if _, _, err := b.api.PostMessageContext(postCtx, channel, opts...); err != nil {
		log.Printf("ross_ops: slack status post failed: %v", err)
	}
}

func containsValue(values []string, want string) bool {
	if strings.TrimSpace(want) == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func prefixAllowed(value string, prefixes []string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	for _, prefix := range prefixes {
		p := strings.TrimSpace(prefix)
		if p == "" {
			continue
		}
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}
