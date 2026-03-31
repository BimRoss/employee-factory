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

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/router"
	"github.com/slack-go/slack"
)

var (
	reSlackUserMention = regexp.MustCompile(`<@(U[A-Za-z0-9]+)>`)
	// Natural-language "everyone" as a whole word (RE2 has no \b; approximate).
	reEveryoneWord = regexp.MustCompile(`(?i)(^|[^a-zA-Z0-9_])everyone([^a-zA-Z0-9_]|$)`)
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

// everyoneTriggerTwoRounds is true when the message should run two full squad rounds.
func everyoneTriggerTwoRounds(rawText string) bool {
	lower := strings.ToLower(rawText)
	if strings.Contains(lower, "<!everyone>") || strings.Contains(lower, "<!channel>") {
		return true
	}
	return reEveryoneWord.MatchString(rawText)
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

// runMultiagentSession coordinates sequential in-thread replies. threadRootTS is the Slack thread parent ts.
func (b *Bot) runMultiagentSession(ctx context.Context, channel, rawText string, threadTS, messageTS string, isIM bool) {
	if isIM || !b.cfg.MultiagentConfigured() {
		return
	}
	participants := mentionedSquadKeys(rawText, b.cfg)
	if len(participants) < 2 {
		return
	}
	rounds := 1
	if everyoneTriggerTwoRounds(rawText) {
		rounds = 2
	}
	slots := buildSlots(participants, rounds, b.cfg.MultiagentBotUserIDs)
	if len(slots) == 0 {
		return
	}

	threadRoot := strings.TrimSpace(messageTS)
	if ts := strings.TrimSpace(threadTS); ts != "" {
		threadRoot = ts
	}

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
		msgs, err := b.waitUntilSlot(waitCtx, channel, threadRoot, slots, k, poll)
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

		b.postMultiagentReply(ctx, channel, userPayload, threadRoot)
	}
}

func (b *Bot) waitUntilSlot(ctx context.Context, channelID, threadRootTS string, slots []string, slotIndex int, poll time.Duration) ([]slack.Message, error) {
	k := slotIndex
	for {
		msgs, err := b.squadMessagesInThread(ctx, channelID, threadRootTS)
		if err != nil {
			return nil, err
		}
		if len(msgs) == k && prefixMatchesSquadSlots(msgs, slots, k) {
			return msgs, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for multi-agent slot %d: %w", k, ctx.Err())
		case <-time.After(poll):
		}
	}
}

func (b *Bot) squadMessagesInThread(ctx context.Context, channelID, threadRootTS string) ([]slack.Message, error) {
	squad := squadUserIDSet(b.cfg)
	params := &slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadRootTS,
		Limit:     b.cfg.LLMThreadMaxMessages,
	}
	msgs, _, _, err := b.api.GetConversationRepliesContext(ctx, params)
	if err != nil {
		return nil, err
	}
	var out []slack.Message
	for _, m := range msgs {
		if strings.TrimSpace(m.Timestamp) == strings.TrimSpace(threadRootTS) {
			continue
		}
		if squad[m.User] {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return parseSlackTSToFloat(out[i].Timestamp) < parseSlackTSToFloat(out[j].Timestamp)
	})
	return out, nil
}

func (b *Bot) postMultiagentReply(ctx context.Context, channel, userPayload, threadRootTS string) {
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
		opts := []slack.MsgOption{
			slack.MsgOptionText("Sorry, I hit an error generating a reply.", false),
			slack.MsgOptionTS(threadRootTS),
		}
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

	opts := []slack.MsgOption{
		slack.MsgOptionText(reply, false),
		slack.MsgOptionTS(threadRootTS),
	}
	_, _, err = b.api.PostMessageContext(ctx, channel, opts...)
	if err != nil {
		log.Printf("slack post message: %v", err)
		return
	}
	if b.outbound != nil {
		b.outbound.record(time.Now())
	}
}
