package slackbot

import (
	"context"
	"fmt"
	"log"
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

// multiagentSquadPasses is how many full ordered passes the squad runs per trigger (one pass =
// each participant posts once in MULTIAGENT_ORDER). A second lap used to run for @everyone and
// produced repetitive “accumulated plan” replies; one pass keeps the turn sharp. Further back-and-
// forth is human-driven (@mention bots again) or future squad-to-squad handling—not fixed rounds.
const multiagentSquadPasses = 1

// broadcastMultiagentTrigger is true for Slack’s channel-wide tokens. Used when no bot is
// @mentioned — each squad bot starts runMultiagentSession; each posts only its own slots.
func broadcastMultiagentTrigger(rawText string) bool {
	lower := strings.ToLower(rawText)
	return strings.Contains(lower, "<!everyone") || strings.Contains(lower, "<!channel")
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

func formatPriorSquadTurns(slots []string, slotIndex int, squadMsgs []slack.Message, idToKey map[string]string, maxRunes int) string {
	if slotIndex <= 0 || len(squadMsgs) == 0 {
		return ""
	}
	n := slotIndex
	if n > len(squadMsgs) {
		n = len(squadMsgs)
	}
	var lines []string
	for i := 0; i < n; i++ {
		key := idToKey[squadMsgs[i].User]
		if key == "" {
			key = squadMsgs[i].User
		}
		text := strings.TrimSpace(squadMsgs[i].Text)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", key, text))
	}
	if len(lines) == 0 {
		return ""
	}
	out := "Earlier responses in this multi-agent turn (in order):\n" + strings.Join(lines, "\n")
	r := []rune(out)
	if len(r) > maxRunes {
		out = "…[truncated]\n" + string(r[len(r)-maxRunes:])
	}
	return out
}

// runMultiagentSession coordinates sequential replies on the channel timeline (no thread_ts).
// messageTS is the triggering message timestamp; squad coordination uses messages posted after it.
// participants is the ordered squad subset (explicit @mentions) or full MULTIAGENT_ORDER (broadcast).
func (b *Bot) runMultiagentSession(ctx context.Context, channel, rawText string, messageTS string, participants []string) {
	if !b.cfg.MultiagentConfigured() {
		return
	}
	if len(participants) < 2 {
		return
	}
	rounds := multiagentSquadPasses
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

	for k, uid := range slots {
		if uid != b.botUserID {
			continue
		}
		waitCtx, cancel := context.WithTimeout(ctx, deadline)
		msgs, err := b.waitUntilSlot(waitCtx, channel, anchorTS, slots, k, poll)
		cancel()
		if err != nil {
			log.Printf("multiagent: slot %d wait failed (employee=%s): %v", k, b.cfg.EmployeeID, err)
			return
		}

		prior := formatPriorSquadTurns(slots, k, msgs, idToKey, b.cfg.LLMThreadMaxRunes)
		userPayload := userQuestion
		if prior != "" {
			userPayload = prior + "\n\n" + userQuestion
		}
		if b.useAlexHints() && b.cfg.LLMAlexHints {
			userPayload = router.WrapAlexUserMessage(userPayload)
		}

		log.Printf("multiagent: generating employee=%s slot=%d user_payload_runes=%d (includes prior squad context when slot>0)",
			b.cfg.EmployeeID, k, utf8.RuneCountInString(userPayload))
		b.postMultiagentReply(ctx, channel, userPayload)
	}
}

func (b *Bot) waitUntilSlot(ctx context.Context, channelID, parentTS string, slots []string, slotIndex int, poll time.Duration) ([]slack.Message, error) {
	k := slotIndex
	start := time.Now()
	attempts := 0
	for {
		attempts++
		msgs, err := b.squadMessagesInChannelAfter(ctx, channelID, parentTS)
		if err != nil {
			return nil, err
		}
		// Slot k is this bot's turn after exactly k prior squad messages in order (0-indexed).
		// We poll conversations.history until that prefix appears—so the previous bot has
		// finished PostMessage and Slack returns the full message before we call the LLM.
		if len(msgs) == k && prefixMatchesSquadSlots(msgs, slots, k) {
			log.Printf("multiagent: slot ready employee=%s slot=%d wait=%v polls=%d prior_squad_msgs=%d",
				b.cfg.EmployeeID, k, time.Since(start), attempts, len(msgs))
			return msgs, nil
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

func (b *Bot) postMultiagentReply(ctx context.Context, channel, userPayload string) {
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

	reply, err := b.llm.Reply(ctx, persona, slackReplySuffix, userPayload)
	if err != nil {
		log.Printf("llm reply error: %v", err)
		opts := []slack.MsgOption{slack.MsgOptionText("Sorry, I hit an error generating a reply.", false)}
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

	reply = formatOutgoingSlackMessage(reply, b.cfg)
	opts := []slack.MsgOption{slack.MsgOptionText(reply, false)}
	_, _, err = b.api.PostMessageContext(ctx, channel, opts...)
	if err != nil {
		log.Printf("slack post message: %v", err)
		return
	}
	if b.outbound != nil {
		b.outbound.record(time.Now())
	}
}
