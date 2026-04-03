package slackbot

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/router"
	"github.com/slack-go/slack"
)

var (
	reSlackUserMention = regexp.MustCompile(`<@(U[A-Za-z0-9]+)>`)
	reBroadcastAlias   = regexp.MustCompile(`(?i)(?:^|\s)@(everyone|channel)\b`)
)

// mentionedSquadKeys returns squad employee keys mentioned in raw Slack text, in MULTIAGENT_ORDER.
func mentionedSquadKeys(rawText string, cfg *config.Config) []string {
	if cfg == nil || len(cfg.MultiagentBotUserIDs) == 0 {
		return nil
	}
	idToKey := make(map[string]string, len(cfg.MultiagentBotUserIDs))
	for k, uid := range cfg.MultiagentBotUserIDs {
		idToKey[uid] = k
	}
	seen := make(map[string]bool)
	for _, id := range parseMentionedUserIDs(rawText) {
		if key, ok := idToKey[id]; ok {
			seen[key] = true
		}
	}
	var out []string
	for _, key := range cfg.MultiagentOrder {
		if seen[key] {
			out = append(out, key)
		}
	}
	return out
}

func parseMentionedUserIDs(text string) []string {
	matches := reSlackUserMention.FindAllStringSubmatch(text, -1)
	var out []string
	seen := make(map[string]bool)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		id := m[1]
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// multiagentSquadPasses is how many full ordered passes run for explicit multi-bot @mentions (not
// channel-wide broadcast). One pass = each participant posts once in MULTIAGENT_ORDER.
const multiagentSquadPasses = 1

// shuffleBroadcastParticipants returns a pseudorandom permutation of order for this trigger.
// All squad pods compute the same sequence from anchorTS + order + optional secret (SHA-256 seed).
func shuffleBroadcastParticipants(anchorTS string, order []string, secret string) []string {
	if len(order) == 0 {
		return nil
	}
	out := make([]string, len(order))
	copy(out, order)
	if len(out) <= 1 {
		return out
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(anchorTS))
	b.WriteByte(0)
	b.WriteString(strings.Join(order, ","))
	b.WriteByte(0)
	b.WriteString(secret)
	sum := sha256.Sum256([]byte(b.String()))
	seed := int64(binary.BigEndian.Uint64(sum[:8]))
	rng := rand.New(rand.NewSource(seed))
	for i := len(out) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// shouldUseBroadcastBranchMode deterministically selects branch mode for a broadcast trigger.
// All pods compute the same result from anchorTS + order + optional secret.
func shouldUseBroadcastBranchMode(anchorTS string, order []string, secret string, probability float64) bool {
	if probability <= 0 {
		return false
	}
	if probability >= 1 {
		return true
	}
	var b strings.Builder
	b.WriteString("broadcast-branch")
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(anchorTS))
	b.WriteByte(0)
	b.WriteString(strings.Join(order, ","))
	b.WriteByte(0)
	b.WriteString(secret)
	sum := sha256.Sum256([]byte(b.String()))
	x := binary.BigEndian.Uint64(sum[8:16])
	u := float64(x) / float64(^uint64(0))
	return u < probability
}

// shouldTriggerGeneralAutoReply deterministically gates plain-message auto-replies in #general.
// All squad pods compute the same result from anchorTS + order + optional secret.
func shouldTriggerGeneralAutoReply(anchorTS string, order []string, secret string, probability float64) bool {
	if probability <= 0 {
		return false
	}
	if probability >= 1 {
		return true
	}
	var b strings.Builder
	b.WriteString("general-auto-reply")
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(anchorTS))
	b.WriteByte(0)
	b.WriteString(strings.Join(order, ","))
	b.WriteByte(0)
	b.WriteString(secret)
	sum := sha256.Sum256([]byte(b.String()))
	x := binary.BigEndian.Uint64(sum[8:16])
	u := float64(x) / float64(^uint64(0))
	return u < probability
}

// selectSingleGeneralParticipant deterministically picks one employee key from order.
// All squad pods compute the same winner from anchorTS + order + optional secret.
func selectSingleGeneralParticipant(anchorTS string, order []string, secret string) string {
	if len(order) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("general-auto-reply-winner")
	b.WriteByte(0)
	b.WriteString(strings.TrimSpace(anchorTS))
	b.WriteByte(0)
	b.WriteString(strings.Join(order, ","))
	b.WriteByte(0)
	b.WriteString(secret)
	sum := sha256.Sum256([]byte(b.String()))
	idx := int(binary.BigEndian.Uint64(sum[:8]) % uint64(len(order)))
	return order[idx]
}

// broadcastMultiagentTrigger is true for Slack channel-wide broadcast forms:
// <!everyone>, <!channel>, and plain @everyone/@channel mentions.
// Used when no bot is @mentioned — each squad bot starts runMultiagentSession; each posts only its own slots.
func broadcastMultiagentTrigger(rawText string) bool {
	lower := strings.ToLower(rawText)
	if strings.Contains(lower, "<!everyone") || strings.Contains(lower, "<!channel") {
		return true
	}
	return reBroadcastAlias.MatchString(rawText)
}

// buildSlots repeats ordered participant keys for each round; returns Slack user IDs per slot.
func buildSlots(participantKeys []string, rounds int, botIDs map[string]string) []string {
	if rounds < 1 {
		rounds = 1
	}
	var slots []string
	for r := 0; r < rounds; r++ {
		for _, k := range participantKeys {
			slots = append(slots, botIDs[k])
		}
	}
	return slots
}

func stripSquadUserMentions(text string, squadUserIDs map[string]bool) string {
	out := reSlackUserMention.ReplaceAllStringFunc(text, func(m string) string {
		sub := reSlackUserMention.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		if squadUserIDs[sub[1]] {
			return ""
		}
		return m
	})
	// Slack special groups
	out = strings.ReplaceAll(out, "<!everyone>", "")
	out = strings.ReplaceAll(out, "<!channel>", "")
	out = strings.TrimSpace(strings.ReplaceAll(out, "  ", " "))
	return strings.TrimSpace(out)
}

func squadUserIDSet(cfg *config.Config) map[string]bool {
	s := make(map[string]bool)
	if cfg == nil {
		return s
	}
	for _, uid := range cfg.MultiagentBotUserIDs {
		s[uid] = true
	}
	return s
}

func parseSlackTSToFloat(ts string) float64 {
	f, err := strconv.ParseFloat(ts, 64)
	if err != nil {
		return 0
	}
	return f
}

// prefixMatchesSquadSlots returns true if the first k squad messages match slots[0:k].
// countSquadMessagesInRun counts squad messages in msgs (oldest-first) in the run ending at
// throughIdx. The run starts after the last message before or at throughIdx whose User is not in
// squad (walking backward from throughIdx). Empty User is ignored for anchor detection.
func countSquadMessagesInRun(msgs []slack.Message, squad map[string]bool, throughIdx int) int {
	if throughIdx < 0 || throughIdx >= len(msgs) || len(squad) == 0 {
		return 0
	}
	anchor := -1
	for i := throughIdx; i >= 0; i-- {
		u := msgs[i].User
		if u == "" {
			continue
		}
		if !squad[u] {
			anchor = i
			break
		}
	}
	start := anchor + 1
	if anchor == -1 {
		start = 0
	}
	n := 0
	for i := start; i <= throughIdx; i++ {
		u := msgs[i].User
		if u != "" && squad[u] {
			n++
		}
	}
	return n
}

func prefixMatchesSquadSlots(squadMsgs []slack.Message, slots []string, k int) bool {
	if k == 0 {
		return true
	}
	if len(squadMsgs) < k {
		return false
	}
	for i := 0; i < k; i++ {
		if squadMsgs[i].User != slots[i] {
			return false
		}
	}
	return true
}

func formatPriorSquadTurns(slots []string, slotIndex int, squadMsgs []slack.Message, idToKey map[string]string, maxRunes int, decay float64, window int) string {
	if slotIndex <= 0 || len(squadMsgs) == 0 {
		return ""
	}
	n := slotIndex
	if n > len(squadMsgs) {
		n = len(squadMsgs)
	}
	type squadLine struct {
		role string
		text string
	}
	var entries []squadLine
	for i := 0; i < n; i++ {
		key := idToKey[squadMsgs[i].User]
		if key == "" {
			key = squadMsgs[i].User
		}
		text := strings.TrimSpace(squadMsgs[i].Text)
		if text == "" {
			continue
		}
		entries = append(entries, squadLine{role: key, text: text})
	}
	if len(entries) == 0 {
		return ""
	}
	lines := make([]string, 0, len(entries))
	for i, e := range entries {
		indexFromLatest := len(entries) - 1 - i
		lines = append(lines, formatWeightedContext(e.role, e.text, indexFromLatest, decay, window))
	}
	out := "Earlier responses in this multi-agent turn (in order):\n" + strings.Join(lines, "\n")
	r := []rune(out)
	if len(r) > maxRunes {
		out = "…[truncated]\n" + string(r[len(r)-maxRunes:])
	}
	return out
}

func roleLaneForKey(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "ross":
		return "Execution and technical risk (infra, implementation constraints, rollout reality)."
	case "alex":
		return "GTM and revenue (offer, distribution, pricing, conversion)."
	case "tim":
		return "Decision quality (tradeoffs, assumptions, low-risk experiments)."
	case "garth":
		return "Synthesis and checklisting (concise recap, owner, next action)."
	default:
		return "Add one distinct, non-duplicative angle tied to this user's decision."
	}
}

func buildMultiagentTurnPolicy(selfKey string, slotIndex int, totalSlots int) string {
	selfKey = strings.ToLower(strings.TrimSpace(selfKey))
	if selfKey == "" || totalSlots < 2 {
		return ""
	}
	lane := roleLaneForKey(selfKey)
	closer := slotIndex == totalSlots-1
	var b strings.Builder
	b.WriteString("Multi-agent turn policy (must follow):\n")
	b.WriteString("- Your lane: ")
	b.WriteString(lane)
	b.WriteString("\n")
	b.WriteString("- Novelty guard: do not restate prior bot lines; add exactly one new angle (risk, metric, tradeoff, or next step).\n")
	b.WriteString("- Brevity guard: max 2 short lines.\n")
	if closer {
		b.WriteString("- You are the closer for this turn: provide the final merged recommendation in one clear move.\n")
		b.WriteString("- Do not ask another bot for a handoff in this closing message.\n")
	} else {
		b.WriteString("- You are not the closer: do not provide the final answer or full recap.\n")
		b.WriteString("- End with one sharp question or handoff cue only if needed.\n")
		b.WriteString("- If you hand off, choose the next agent by unresolved lane; do not default to @ross unless implementation/code/infra execution is the blocker.\n")
	}
	return b.String()
}

// runMultiagentSession coordinates sequential replies on the channel timeline (no thread_ts).
// messageTS is the triggering message timestamp; squad coordination uses messages posted after it.
// participants is the ordered squad subset (explicit @mentions) or shuffled broadcast order for <!everyone>.
// handoffProbability is per-reply chance to nudge an @mention of another squad member (0–1).
func (b *Bot) runMultiagentSession(ctx context.Context, channel, rawText string, messageTS string, participants []string, rounds int, handoffProbability float64) {
	if !b.cfg.MultiagentConfigured() {
		return
	}
	if len(participants) < 2 {
		return
	}
	if rounds < 1 {
		rounds = 1
	}
	if handoffProbability < 0 {
		handoffProbability = 0
	}
	if handoffProbability > 1 {
		handoffProbability = 1
	}
	slots := buildSlots(participants, rounds, b.cfg.MultiagentBotUserIDs)
	if len(slots) == 0 {
		return
	}

	anchorTS := strings.TrimSpace(messageTS)
	if anchorTS == "" {
		return
	}

	log.Printf("multiagent: session start employee=%s slots=%d rounds=%d anchor=%s", b.cfg.EmployeeID, len(slots), rounds, anchorTS)

	squadSet := squadUserIDSet(b.cfg)
	idToKey := make(map[string]string)
	for k, uid := range b.cfg.MultiagentBotUserIDs {
		idToKey[uid] = k
	}

	userQuestion := strings.TrimSpace(stripSquadUserMentions(rawText, squadSet))
	if userQuestion == "" {
		userQuestion = "(no text besides mentions)"
	}

	poll := time.Duration(b.cfg.MultiagentPollInterval) * time.Millisecond
	deadline := time.Duration(b.cfg.MultiagentWaitTimeoutSec) * time.Second
	softDeadline := time.Duration(b.cfg.MultiagentSlotSoftTimeoutSec) * time.Second
	allowDegraded := b.cfg.MultiagentAllowDegradedStart

	for k, uid := range slots {
		if uid != b.botUserID {
			continue
		}
		waitCtx, cancel := context.WithTimeout(ctx, deadline)
		waitResult, err := b.waitUntilSlot(waitCtx, channel, anchorTS, slots, k, poll, softDeadline, allowDegraded)
		cancel()
		if err != nil {
			if strings.Contains(err.Error(), "missing_prior_slot") {
				log.Printf("multiagent: missing_prior_slot employee=%s anchor=%s slot=%d err=%v", b.cfg.EmployeeID, anchorTS, k, err)
			} else {
				log.Printf("multiagent: slot %d wait failed (employee=%s): %v", k, b.cfg.EmployeeID, err)
			}
			return
		}
		msgs := waitResult.Messages
		log.Printf("multiagent: slot wait employee=%s anchor=%s slot=%d wait_mode=%s wait_ms=%d polls=%d observed_prior=%d missing_expected_agent=%s degraded=%t reason=%s",
			b.cfg.EmployeeID,
			anchorTS,
			k,
			waitResult.Mode,
			waitResult.Wait.Milliseconds(),
			waitResult.Polls,
			waitResult.ObservedPrior,
			waitResult.ExpectedMissingAgent,
			waitResult.Mode == multiagentWaitModeDegraded,
			waitResult.Reason,
		)
		if waitResult.Mode == multiagentWaitModeDegraded && waitResult.ObservedPrior > k {
			log.Printf("multiagent: late_slot_post employee=%s anchor=%s slot=%d observed_prior=%d", b.cfg.EmployeeID, anchorTS, k, waitResult.ObservedPrior)
		}

		prior := formatPriorSquadTurns(
			slots,
			k,
			msgs,
			idToKey,
			b.cfg.LLMThreadMaxRunes,
			b.cfg.LLMContextWeightDecay,
			b.cfg.LLMContextWeightWindow,
		)
		userPayload := userQuestion
		if prior != "" {
			userPayload = prior + "\n\n" + userQuestion
		}
		selfKey := idToKey[b.botUserID]
		if policy := buildMultiagentTurnPolicy(selfKey, k, len(slots)); policy != "" {
			userPayload = policy + "\n\n" + userPayload
		}
		userPayload = prependHostilityCue(userPayload, userQuestion)
		if b.useAlexHints() && b.cfg.LLMAlexHints {
			userPayload = router.WrapAlexUserMessage(userPayload)
		}

		log.Printf("multiagent: generating employee=%s slot=%d user_payload_runes=%d (includes prior squad context when slot>0)",
			b.cfg.EmployeeID, k, utf8.RuneCountInString(userPayload))
		slotHandoffProbability := handoffProbability
		if k == len(slots)-1 {
			// Single closer: keep the final message self-contained to reduce ping-pong loops.
			slotHandoffProbability = 0
		}
		b.postMultiagentReply(ctx, channel, userPayload, slotHandoffProbability)
	}
}

const (
	multiagentWaitModeExact    = "exact_slot_ready"
	multiagentWaitModeDegraded = "degraded_start"
)

type multiagentSlotWaitResult struct {
	Messages             []slack.Message
	Mode                 string
	Wait                 time.Duration
	Polls                int
	ObservedPrior        int
	ExpectedMissingAgent string
	Reason               string
}

func evaluateMultiagentSlotState(
	k int,
	msgs []slack.Message,
	slots []string,
	elapsed time.Duration,
	softDeadline time.Duration,
	allowDegraded bool,
) (mode string, reason string, ok bool) {
	if len(msgs) == k && prefixMatchesSquadSlots(msgs, slots, k) {
		return multiagentWaitModeExact, "exact_prefix_ready", true
	}
	if allowDegraded && softDeadline > 0 && elapsed >= softDeadline {
		return multiagentWaitModeDegraded, "soft_timeout", true
	}
	return "", "", false
}

func expectedMissingAgentForSlot(slots []string, slotIndex int, observedPrior int) string {
	if slotIndex <= 0 || len(slots) == 0 {
		return ""
	}
	if observedPrior >= 0 && observedPrior < len(slots) {
		return slots[observedPrior]
	}
	prev := slotIndex - 1
	if prev >= 0 && prev < len(slots) {
		return slots[prev]
	}
	return ""
}

func (b *Bot) waitUntilSlot(
	ctx context.Context,
	channelID, parentTS string,
	slots []string,
	slotIndex int,
	poll time.Duration,
	softDeadline time.Duration,
	allowDegraded bool,
) (*multiagentSlotWaitResult, error) {
	k := slotIndex
	start := time.Now()
	attempts := 0
	lastSeen := -1
	noProgressPolls := 0
	prefixMismatchPolls := 0
	const maxNoProgressPolls = 45
	const maxPrefixMismatchPolls = 6
	for {
		attempts++
		msgs, err := b.squadMessagesInChannelAfter(ctx, channelID, parentTS)
		if err != nil {
			return nil, err
		}
		// Slot k is this bot's turn after exactly k prior squad messages in order (0-indexed).
		// We poll conversations.history until that prefix appears—so the previous bot has
		// finished PostMessage and Slack returns the full message before we call the LLM.
		elapsed := time.Since(start)
		if mode, reason, ok := evaluateMultiagentSlotState(k, msgs, slots, elapsed, softDeadline, allowDegraded); ok {
			return &multiagentSlotWaitResult{
				Messages:             msgs,
				Mode:                 mode,
				Wait:                 elapsed,
				Polls:                attempts,
				ObservedPrior:        len(msgs),
				ExpectedMissingAgent: expectedMissingAgentForSlot(slots, k, len(msgs)),
				Reason:               reason,
			}, nil
		}
		if len(msgs) == lastSeen {
			noProgressPolls++
		} else {
			noProgressPolls = 0
			lastSeen = len(msgs)
		}
		if len(msgs) >= k && !prefixMatchesSquadSlots(msgs, slots, k) {
			prefixMismatchPolls++
		} else {
			prefixMismatchPolls = 0
		}
		if prefixMismatchPolls >= maxPrefixMismatchPolls {
			expected := ""
			if k > 0 && k-1 < len(slots) {
				expected = slots[k-1]
			}
			if allowDegraded {
				continue
			}
			return nil, fmt.Errorf("missing_prior_slot anchor=%s slot=%d expected_agent=%s observed_prior=%d reason=prefix_mismatch polls=%d",
				parentTS, k, expected, len(msgs), attempts)
		}
		if len(msgs) < k && noProgressPolls >= maxNoProgressPolls {
			expected := ""
			next := len(msgs)
			if next >= 0 && next < len(slots) {
				expected = slots[next]
			}
			if allowDegraded {
				continue
			}
			return nil, fmt.Errorf("missing_prior_slot anchor=%s slot=%d expected_agent=%s observed_prior=%d reason=no_progress polls=%d",
				parentTS, k, expected, len(msgs), attempts)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for multi-agent slot %d: %w", k, ctx.Err())
		case <-time.After(poll):
		}
	}
}

// squadMessagesInChannelAfter returns squad-bot messages posted to the channel after parentTS (exclusive),
// oldest-first. Used instead of thread replies so #chat stays a single timeline.
func (b *Bot) squadMessagesInChannelAfter(ctx context.Context, channelID, parentTS string) ([]slack.Message, error) {
	squad := squadUserIDSet(b.cfg)
	limit := b.cfg.LLMThreadMaxMessages
	if limit < 50 {
		limit = 50
	}
	params := &slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Oldest:    parentTS,
		Inclusive: false,
		Limit:     limit,
	}
	resp, err := b.api.GetConversationHistoryContext(ctx, params)
	if err != nil {
		return nil, err
	}
	parentF := parseSlackTSToFloat(parentTS)
	var out []slack.Message
	for _, m := range resp.Messages {
		if parseSlackTSToFloat(m.Timestamp) <= parentF {
			continue
		}
		if m.User == "" || !squad[m.User] {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return parseSlackTSToFloat(out[i].Timestamp) < parseSlackTSToFloat(out[j].Timestamp)
	})
	return out, nil
}

func (b *Bot) postMultiagentReply(ctx context.Context, channel, userPayload string, handoffProbability float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.outbound != nil && !b.outbound.allow(now) {
		log.Printf("slack outbound rate limit: skipping multi-agent reply (employee=%s channel=%s)", b.cfg.EmployeeID, channel)
		return
	}

	persona := b.persona.String()
	if persona == "" {
		persona = "You are a helpful assistant."
	}

	suffix := slackReplySuffix
	handoff := false
	if b.cfg.MultiagentConfigured() {
		p := handoffProbability
		handoff, _ = shouldHandoff(
			p,
			b.cfg.MultiagentHandoffMinProbability,
			b.cfg.MultiagentHandoffMaxProbability,
		)
		if handoff {
			suffix += "\n\nHand-off cue for this turn: include one @mention of another squad member—not yourself (@ross/@tim/@alex/@garth) with a concrete question or next step. Choose based on unresolved lane; do not default to @ross unless implementation/code/infra execution is the blocker."
		} else {
			suffix += "\n\nHand-off cue for this turn: keep this reply self-contained; do not @mention squad members unless strictly necessary."
		}
	}

	llmCtx, cancelLLM := b.withLLMTimeout(ctx)
	startLLM := time.Now()
	reply, err := b.llm.Reply(llmCtx, persona, suffix, userPayload)
	cancelLLM()
	log.Printf("llm_call: path=multiagent employee=%s ms=%d err=%t payload_runes=%d",
		strings.TrimSpace(b.cfg.EmployeeID), time.Since(startLLM).Milliseconds(), err != nil, utf8.RuneCountInString(userPayload))
	if err != nil {
		log.Printf("llm reply error (employee=%s): %v", strings.TrimSpace(b.cfg.EmployeeID), err)
		opts := []slack.MsgOption{slack.MsgOptionText(llmErrorUserMessage(err), false)}
		_, _, err = b.api.PostMessageContext(ctx, channel, opts...)
		if err != nil {
			log.Printf("slack post message: %v", err)
			return
		}
		if b.outbound != nil {
			b.outbound.record(time.Now())
		}
		return
	}
	if reply == "" {
		reply = "…"
	}
	startRepair := time.Now()
	reply = b.repairOutboundReply(ctx, persona, userPayload, reply)
	log.Printf("repair_call: path=multiagent employee=%s ms=%d",
		strings.TrimSpace(b.cfg.EmployeeID), time.Since(startRepair).Milliseconds())
	reply = normalizeSlackReply(reply, b.cfg, b.botUserID)
	reply = enforceMultiagentMentionPolicy(reply, b.cfg, b.botUserID, handoff)
	reply = normalizeSlackReply(reply, b.cfg, b.botUserID)
	opts := []slack.MsgOption{slack.MsgOptionText(reply, false)}
	startPost := time.Now()
	_, _, err = b.api.PostMessageContext(ctx, channel, opts...)
	log.Printf("slack_post: path=multiagent employee=%s ms=%d err=%t",
		strings.TrimSpace(b.cfg.EmployeeID), time.Since(startPost).Milliseconds(), err != nil)
	if err != nil {
		log.Printf("slack post message: %v", err)
		return
	}
	if b.outbound != nil {
		b.outbound.record(time.Now())
	}
}
