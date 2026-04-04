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
	QuestionType string  `json:"question_type,omitempty"`
	Namespace    string  `json:"namespace,omitempty"`
	Target       string  `json:"target,omitempty"`
	Container    string  `json:"container,omitempty"`
	RedisKey     string  `json:"redis_key,omitempty"`
	RedisPrefix  string  `json:"redis_prefix,omitempty"`
	TailLines    int64   `json:"tail_lines,omitempty"`
	SinceSeconds int64   `json:"since_seconds,omitempty"`
	Limit        int     `json:"limit,omitempty"`
	RevealFull   bool    `json:"reveal_full,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

type rossOpsAction struct {
	Operation    opsproxy.Operation
	QuestionType waitlistQuestionType
	Namespace    string
	Target       string
	Container    string
	RedisKey     string
	RedisPrefix  string
	TailLines    int64
	SinceSeconds int64
	Limit        int
	RevealFull   bool
	RawPrompt    string
}

type waitlistQuestionType string

const (
	waitlistQuestionNone   waitlistQuestionType = "none"
	waitlistQuestionLatest waitlistQuestionType = "latest_signup"
	waitlistQuestionList   waitlistQuestionType = "list"
	waitlistQuestionCount  waitlistQuestionType = "count"
)

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
				Enum:        []string{string(opsproxy.OperationK8sStatus), string(opsproxy.OperationK8sMetrics), string(opsproxy.OperationK8sLogs), string(opsproxy.OperationRedisRead), string(opsproxy.OperationWaitlistEmails)},
				Description: "Desired read-only operation.",
			},
			"question_type": {
				Type:        jsonschema.String,
				Enum:        []string{string(waitlistQuestionLatest), string(waitlistQuestionList), string(waitlistQuestionCount), string(waitlistQuestionNone)},
				Description: "When operation=waitlist_emails, classify the ask intent.",
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
			"reveal_full": {
				Type:        jsonschema.Boolean,
				Description: "Whether full emails should be returned (Grant-only).",
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

func (b *Bot) tryHandleRossOps(ctx context.Context, channel, rawText, requestUserID, messageTS, threadTS string) bool {
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
	action.RawPrompt = cmd
	log.Printf(
		"ross_ops: accepted message_ts=%s source=%s operation=%s question_type=%s confidence=%.2f extract_err=%t reason=%q",
		strings.TrimSpace(messageTS),
		source,
		action.Operation,
		action.QuestionType,
		extract.Confidence,
		extractErr != nil,
		strings.TrimSpace(extract.Reason),
	)
	if b.cfg.RossOpsLogOnly {
		log.Printf("ross_ops: log_only=true message_ts=%s source=%s operation=%s", strings.TrimSpace(messageTS), source, action.Operation)
		return false
	}
	go b.handleRossOpsSafely(ctx, channel, requestUserID, threadTS, messageTS, action)
	return true
}

func resolveRossOpsAction(raw string, extract rossOpsActionExtract, extractErr error) (rossOpsAction, bool, string) {
	waitlistPrompt := isWaitlistEmailPrompt(raw)
	if extractErr == nil && strings.EqualFold(strings.TrimSpace(extract.Intent), "ops_query") {
		questionType := parseWaitlistQuestionType(extract.QuestionType)
		action := rossOpsAction{
			Operation:    opsproxy.Operation(strings.TrimSpace(extract.Operation)),
			QuestionType: questionType,
			Namespace:    strings.TrimSpace(extract.Namespace),
			Target:       strings.TrimSpace(extract.Target),
			Container:    strings.TrimSpace(extract.Container),
			RedisKey:     strings.TrimSpace(extract.RedisKey),
			RedisPrefix:  strings.TrimSpace(extract.RedisPrefix),
			TailLines:    extract.TailLines,
			SinceSeconds: extract.SinceSeconds,
			Limit:        extract.Limit,
			RevealFull:   extract.RevealFull,
		}
		// Guardrail: waitlist-email asks should never fall through as k8s status.
		if waitlistPrompt {
			action.Operation = opsproxy.OperationWaitlistEmails
			if action.Limit <= 0 {
				action.Limit = 100
			}
			if action.QuestionType == waitlistQuestionNone {
				action.QuestionType = inferWaitlistQuestionType(raw)
			}
			return action, true, "extractor_waitlist_override"
		}
		if action.Operation == opsproxy.OperationWaitlistEmails && action.QuestionType == waitlistQuestionNone {
			action.QuestionType = inferWaitlistQuestionType(raw)
		}
		if action.Operation == opsproxy.OperationK8sStatus || action.Operation == opsproxy.OperationK8sMetrics || action.Operation == opsproxy.OperationK8sLogs || action.Operation == opsproxy.OperationRedisRead || action.Operation == opsproxy.OperationWaitlistEmails {
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
	if isWaitlistEmailPrompt(text) {
		action.Operation = opsproxy.OperationWaitlistEmails
		action.QuestionType = inferWaitlistQuestionType(text)
		action.Limit = 100
		action.RevealFull = strings.Contains(text, "full") || strings.Contains(text, "unmask")
		return action, true
	}
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
	if isK8sMetricsPrompt(text) {
		action.Operation = opsproxy.OperationK8sMetrics
		action.Limit = 8
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

func isWaitlistEmailPrompt(raw string) bool {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return false
	}
	return strings.Contains(text, "waitlist") && (strings.Contains(text, "email") || strings.Contains(text, "emails"))
}

func parseWaitlistQuestionType(raw string) waitlistQuestionType {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(waitlistQuestionLatest):
		return waitlistQuestionLatest
	case string(waitlistQuestionList):
		return waitlistQuestionList
	case string(waitlistQuestionCount):
		return waitlistQuestionCount
	default:
		return waitlistQuestionNone
	}
}

func inferWaitlistQuestionType(raw string) waitlistQuestionType {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return waitlistQuestionList
	}
	if containsAny(text, "how many", "count", "total", "number of") {
		return waitlistQuestionCount
	}
	if containsAny(text, "last", "latest", "most recent", "newest") {
		return waitlistQuestionLatest
	}
	return waitlistQuestionList
}

func containsAny(s string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(s, term) {
			return true
		}
	}
	return false
}

func isK8sMetricsPrompt(raw string) bool {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return false
	}
	hasResourceSignal := strings.Contains(text, "cpu") || strings.Contains(text, "ram") || strings.Contains(text, "memory")
	hasInfraSignal := strings.Contains(text, "kubernetes") || strings.Contains(text, "cluster") || strings.Contains(text, "node") || strings.Contains(text, "capacity") || strings.Contains(text, "usage") || strings.Contains(text, "metric")
	return hasResourceSignal && hasInfraSignal
}

func (b *Bot) extractRossOpsAction(ctx context.Context, cmdText string) (rossOpsActionExtract, error) {
	var out rossOpsActionExtract
	extractCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	systemPrompt := "You classify whether a Slack message is a Ross operations data request. Respond only with schema-compliant JSON."
	userPrompt := "Message:\n" + strings.TrimSpace(cmdText) + "\n\nRules:\n- intent=ops_query only for explicit requests to inspect infrastructure/cache state.\n- operation must be one of k8s_status, k8s_metrics, k8s_logs, redis_read, waitlist_emails.\n- If user asks for CPU, RAM, or memory capacity/usage in Kubernetes, use k8s_metrics.\n- If user asks for waitlist emails, use waitlist_emails and classify question_type:\n  - latest_signup for asks like last/latest/most recent signup.\n  - count for asks like how many/total/count.\n  - list for asks requesting list/show emails.\n  - none when operation is not waitlist_emails.\n- If unsure, use intent=none.\n- Never invent namespaces, targets, keys, or logs."
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
	out.QuestionType = string(parseWaitlistQuestionType(out.QuestionType))
	out.Reason = strings.TrimSpace(out.Reason)
	if out.Confidence < 0 {
		out.Confidence = 0
	}
	if out.Confidence > 1 {
		out.Confidence = 1
	}
	return out, nil
}

func (b *Bot) handleRossOpsSafely(parent context.Context, channel, requestUserID, threadTS, messageTS string, action rossOpsAction) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("ross_ops: panic recovered message_ts=%s panic=%v", strings.TrimSpace(messageTS), r)
		}
	}()
	b.handleRossOps(parent, channel, requestUserID, threadTS, action)
}

func (b *Bot) handleRossOps(parent context.Context, channel, requestUserID, threadTS string, action rossOpsAction) {
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
		log.Printf("ross_ops: execute operation=%s namespace=%s target=%s", action.Operation, namespace, strings.TrimSpace(action.Target))
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
	case opsproxy.OperationK8sMetrics:
		log.Printf("ross_ops: execute operation=%s namespace=%s limit=%d", action.Operation, namespace, action.Limit)
		resp, err := b.opsProxyClient.Metrics(ctx, opsproxy.MetricsRequest{
			Namespace: namespace,
			Limit:     action.Limit,
		})
		if err != nil {
			b.postRossOpsStatus(ctx, channel, threadTS, fmt.Sprintf("I could not fetch Kubernetes CPU/RAM metrics from the ops proxy (%s).", compactOpsErr(err)))
			return
		}
		b.postRossOpsStatus(ctx, channel, threadTS, formatRossMetrics(resp))
	case opsproxy.OperationK8sLogs:
		log.Printf("ross_ops: execute operation=%s namespace=%s target=%s", action.Operation, namespace, strings.TrimSpace(action.Target))
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
		log.Printf("ross_ops: execute operation=%s key=%s prefix=%s", action.Operation, strings.TrimSpace(action.RedisKey), strings.TrimSpace(action.RedisPrefix))
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
	case opsproxy.OperationWaitlistEmails:
		log.Printf("ross_ops: execute operation=%s requester=%s question_type=%s limit=%d reveal_full=%t", action.Operation, strings.TrimSpace(requestUserID), action.QuestionType, action.Limit, action.RevealFull)
		if strings.TrimSpace(requestUserID) != strings.TrimSpace(b.cfg.ChatAllowedUserID) {
			b.postRossOpsStatus(ctx, channel, threadTS, "I can only return waitlist emails for the authorized operator.")
			return
		}
		resp, err := b.opsProxyClient.WaitlistEmails(ctx, opsproxy.WaitlistEmailsRequest{
			Limit:      action.Limit,
			RevealFull: action.RevealFull,
		})
		if err != nil {
			b.postRossOpsStatus(ctx, channel, threadTS, "I could not fetch waitlist emails from Redis.")
			return
		}
		answer, synthesisMode, orderingConfidence := buildWaitlistAnswer(action.QuestionType, action.RawPrompt, resp, action.RevealFull)
		log.Printf("ross_ops: waitlist synthesis mode=%s ordering_confidence=%s question_type=%s emails=%d", synthesisMode, orderingConfidence, action.QuestionType, len(resp.Emails))
		b.postRossOpsStatus(ctx, channel, threadTS, answer)
	default:
		b.postRossOpsStatus(ctx, channel, threadTS, "I can run `k8s_status`, `k8s_metrics`, `k8s_logs`, `redis_read`, or `waitlist_emails` once the request is specific.")
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

func formatRossWaitlistEmails(resp opsproxy.WaitlistEmailsResponse, revealFull bool) string {
	if len(resp.Emails) == 0 {
		if len(resp.SearchedPrefixes) > 0 {
			return fmt.Sprintf("No waitlist emails found. I scanned prefixes: `%s`.", strings.Join(resp.SearchedPrefixes, "`, `"))
		}
		return "No waitlist emails found in the allowed Redis prefixes."
	}
	header := "Waitlist emails (masked):"
	if revealFull {
		header = "Waitlist emails:"
	}
	lines := []string{header}
	for _, item := range resp.Emails {
		lines = append(lines, fmt.Sprintf("- `%s`", strings.TrimSpace(item.Email)))
	}
	return strings.Join(lines, "\n")
}

func buildWaitlistAnswer(questionType waitlistQuestionType, rawPrompt string, resp opsproxy.WaitlistEmailsResponse, revealFull bool) (string, string, string) {
	if questionType == waitlistQuestionNone {
		questionType = inferWaitlistQuestionType(rawPrompt)
	}
	if len(resp.Emails) == 0 {
		return formatRossWaitlistEmails(resp, revealFull), "deterministic", "timestamp_missing"
	}
	switch questionType {
	case waitlistQuestionLatest:
		latest := resp.Emails[0]
		email := strings.TrimSpace(latest.Email)
		updatedAt := strings.TrimSpace(latest.UpdatedAt)
		if email == "" {
			return "I found waitlist records, but the latest email field was empty.", "deterministic", "timestamp_missing"
		}
		if updatedAt == "" {
			return fmt.Sprintf("I found `%s`, but I cannot confirm it is the latest signup because timestamp metadata is missing.", email), "deterministic", "timestamp_missing"
		}
		return fmt.Sprintf("The latest waitlist signup is `%s` (updated `%s`).", email, updatedAt), "deterministic", "timestamp_sorted"
	case waitlistQuestionCount:
		prefixes := "configured waitlist prefixes"
		if len(resp.SearchedPrefixes) > 0 {
			prefixes = fmt.Sprintf("`%s`", strings.Join(resp.SearchedPrefixes, "`, `"))
		}
		return fmt.Sprintf("I found %d waitlist emails across %s.", len(resp.Emails), prefixes), "deterministic", "timestamp_sorted"
	case waitlistQuestionList:
		fallthrough
	default:
		return formatRossWaitlistEmails(resp, revealFull), "deterministic", "timestamp_sorted"
	}
}

func formatRossMetrics(resp opsproxy.MetricsResponse) string {
	lines := []string{fmt.Sprintf("Kubernetes CPU/RAM metrics `%s`", strings.TrimSpace(resp.Namespace))}
	lines = append(lines, fmt.Sprintf("- cluster cpu cap=%.2f cores alloc=%.2f req=%.2f lim=%.2f usage=%.2f", milliToCores(resp.Cluster.CPUCapacityMilli), milliToCores(resp.Cluster.CPUAllocatableMilli), milliToCores(resp.Cluster.CPURequestedMilli), milliToCores(resp.Cluster.CPULimitsMilli), milliToCores(resp.Cluster.CPUUsageMilli)))
	lines = append(lines, fmt.Sprintf("- cluster mem cap=%.2f Gi alloc=%.2f Gi req=%.2f Gi lim=%.2f Gi usage=%.2f Gi", bytesToGi(resp.Cluster.MemoryCapacityBytes), bytesToGi(resp.Cluster.MemoryAllocatableBytes), bytesToGi(resp.Cluster.MemoryRequestedBytes), bytesToGi(resp.Cluster.MemoryLimitsBytes), bytesToGi(resp.Cluster.MemoryUsageBytes)))
	if resp.LiveMetricsAvailable {
		lines = append(lines, "- live usage source=metrics.k8s.io")
	} else {
		reason := strings.TrimSpace(resp.LiveMetricsReason)
		if reason == "" {
			reason = "metrics API unavailable"
		}
		lines = append(lines, fmt.Sprintf("- live usage unavailable: %s", reason))
	}
	for _, node := range resp.Nodes {
		lines = append(lines, fmt.Sprintf("- node/%s cpu alloc=%.2f req=%.2f usage=%.2f mem alloc=%.2fGi req=%.2fGi usage=%.2fGi", node.NodeName, milliToCores(node.CPUAllocatableMilli), milliToCores(node.CPURequestedMilli), milliToCores(node.CPUUsageMilli), bytesToGi(node.MemoryAllocatableBytes), bytesToGi(node.MemoryRequestedBytes), bytesToGi(node.MemoryUsageBytes)))
	}
	return strings.Join(lines, "\n")
}

func milliToCores(v int64) float64 {
	return float64(v) / 1000.0
}

func bytesToGi(v int64) float64 {
	return float64(v) / (1024.0 * 1024.0 * 1024.0)
}

func compactOpsErr(err error) string {
	if err == nil {
		return "unknown error"
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "unknown error"
	}
	if len(msg) > 120 {
		msg = strings.TrimSpace(msg[:120]) + "..."
	}
	return msg
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
