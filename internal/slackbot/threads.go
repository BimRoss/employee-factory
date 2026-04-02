package slackbot

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/router"
	"github.com/bimross/employee-factory/internal/threadstore"
	"github.com/slack-go/slack"
)

// threadRoutingShouldReply implements v1: if the CEO @mentions squad members, only those respond;
// otherwise only the thread owner responds.
func threadRoutingShouldReply(empKey string, ownerKey string, mentionedKeys []string) bool {
	if len(mentionedKeys) > 0 {
		for _, k := range mentionedKeys {
			if k == empKey {
				return true
			}
		}
		return false
	}
	return strings.TrimSpace(empKey) == strings.TrimSpace(ownerKey) && ownerKey != ""
}

func squadKeyForSlackUser(cfg *config.Config, userID string) (key string, ok bool) {
	if cfg == nil || len(cfg.MultiagentBotUserIDs) == 0 {
		return "", false
	}
	for k, uid := range cfg.MultiagentBotUserIDs {
		if uid == userID {
			return k, true
		}
	}
	return "", false
}

func sortMessagesOldestFirst(msgs []slack.Message) {
	sort.Slice(msgs, func(i, j int) bool {
		return parseSlackTSToFloat(msgs[i].Timestamp) < parseSlackTSToFloat(msgs[j].Timestamp)
	})
}

// fetchThreadMessages loads all messages in a thread (paginated).
func (b *Bot) fetchThreadMessages(ctx context.Context, channelID, threadTS string) ([]slack.Message, error) {
	var all []slack.Message
	cursor := ""
	for i := 0; i < 20; i++ {
		params := &slack.GetConversationRepliesParameters{
			ChannelID: channelID,
			Timestamp: threadTS,
			Cursor:    cursor,
			Limit:     200,
		}
		msgs, hasMore, next, err := b.api.GetConversationRepliesContext(ctx, params)
		if err != nil {
			return nil, err
		}
		all = append(all, msgs...)
		if !hasMore || next == "" {
			break
		}
		cursor = next
	}
	sortMessagesOldestFirst(all)
	return all, nil
}

func findThreadRoot(msgs []slack.Message, threadTS string) *slack.Message {
	for i := range msgs {
		if msgs[i].Timestamp == threadTS {
			return &msgs[i]
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return &msgs[0]
}

// isFirstMessageFromUser returns true if msgTS is the first message in msgs (oldest-first) from userID (excluding root index 0 if root matches threadTS).
func isFirstReplyFromUser(msgs []slack.Message, threadTS, userID, msgTS string) bool {
	for _, m := range msgs {
		if m.Timestamp == threadTS {
			continue
		}
		if m.User != userID {
			continue
		}
		return m.Timestamp == msgTS
	}
	return false
}

func threadTranscriptBefore(cfg *config.Config, botUserID string, msgs []slack.Message, threadTS, currentMsgTS string, maxRunes int) string {
	type threadLine struct {
		role string
		text string
	}
	var entries []threadLine
	for _, m := range msgs {
		if m.Timestamp == currentMsgTS {
			continue
		}
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		if m.SubType == slack.MsgSubTypeMessageChanged || m.SubType == slack.MsgSubTypeMessageDeleted {
			continue
		}
		role := "user"
		if m.BotID != "" || m.User == botUserID {
			role = "assistant"
		} else if sk, ok := squadKeyForSlackUser(cfg, m.User); ok {
			role = sk
		}
		entries = append(entries, threadLine{role: role, text: text})
	}
	if len(entries) == 0 {
		return ""
	}
	decay := 0.5
	window := 3
	if cfg != nil {
		decay = cfg.LLMContextWeightDecay
		window = cfg.LLMContextWeightWindow
	}
	lines := make([]string, 0, len(entries))
	for i, e := range entries {
		indexFromLatest := len(entries) - 1 - i
		lines = append(lines, formatWeightedContext(e.role, e.text, indexFromLatest, decay, window))
	}
	out := "Thread so far (oldest first):\n" + strings.Join(lines, "\n")
	r := []rune(out)
	if len(r) > maxRunes {
		out = "…[truncated; oldest lines dropped]\n" + string(r[len(r)-maxRunes:])
	}
	return out
}

// resolveThreadOwner returns owner employee key for the thread (bot-root or human-root).
func (b *Bot) resolveThreadOwner(ctx context.Context, channelID, threadTS string, msgs []slack.Message, root *slack.Message, currentText, currentMsgTS string) (ownerKey string, err error) {
	cfg := b.cfg
	if root == nil {
		return "", fmt.Errorf("no thread root")
	}
	if sk, ok := squadKeyForSlackUser(cfg, root.User); ok {
		return sk, nil
	}

	st := b.threadOwner
	if st == nil {
		st = threadstore.Noop{}
	}
	if stored, ok, err := st.Get(ctx, channelID, threadTS); err != nil {
		return "", err
	} else if ok && strings.TrimSpace(stored) != "" {
		return strings.TrimSpace(stored), nil
	}

	uid := cfg.ChatAllowedUserID
	var firstCEO *slack.Message
	for _, m := range msgs {
		if m.Timestamp == threadTS {
			continue
		}
		if m.User != uid {
			continue
		}
		cp := m
		firstCEO = &cp
		break
	}
	if firstCEO != nil {
		men := mentionedSquadKeys(firstCEO.Text, cfg)
		if len(men) == 1 {
			k := men[0]
			ttl := time.Duration(cfg.ThreadOwnerTTLSec) * time.Second
			if ttl <= 0 {
				ttl = 30 * 24 * time.Hour
			}
			_ = st.Set(ctx, channelID, threadTS, k, ttl)
			return k, nil
		}
		return "", fmt.Errorf("human-root thread: first reply must @mention exactly one squad agent")
	}

	// No CEO message in thread yet — bootstrap with this message
	if !isFirstReplyFromUser(msgs, threadTS, uid, currentMsgTS) {
		return "", fmt.Errorf("human-root thread: establish owner by @mentioning exactly one squad agent in the first thread reply")
	}
	mentioned := mentionedSquadKeys(currentText, cfg)
	if len(mentioned) != 1 {
		return "", fmt.Errorf("human-root thread: first reply must @mention exactly one squad agent (got %d)", len(mentioned))
	}
	if _, ok := cfg.MultiagentBotUserIDs[mentioned[0]]; !ok {
		return "", fmt.Errorf("invalid mentioned key")
	}
	ttl := time.Duration(cfg.ThreadOwnerTTLSec) * time.Second
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	if err := st.Set(ctx, channelID, threadTS, mentioned[0], ttl); err != nil {
		return "", fmt.Errorf("redis set owner: %w", err)
	}
	return mentioned[0], nil
}

// handleThreadMessage processes CEO messages inside a thread. Returns true if the event was consumed.
func (b *Bot) handleThreadMessage(ctx context.Context, channel, userID, rawText, messageTS, threadTS string) bool {
	cfg := b.cfg
	if !cfg.ThreadsEnabled() || channel != cfg.SlackChatChannelID {
		return true
	}
	if userID != cfg.ChatAllowedUserID {
		return true
	}
	if !cfg.MultiagentConfigured() {
		log.Printf("threads: multiagent not configured, skipping")
		return true
	}

	msgs, err := b.fetchThreadMessages(ctx, channel, threadTS)
	if err != nil {
		log.Printf("threads: fetch replies: %v", err)
		return true
	}
	root := findThreadRoot(msgs, threadTS)
	ownerKey, err := b.resolveThreadOwner(ctx, channel, threadTS, msgs, root, rawText, messageTS)
	if err != nil {
		log.Printf("threads: owner: %v", err)
		return true
	}

	emp := strings.ToLower(strings.TrimSpace(cfg.EmployeeID))
	mentioned := mentionedSquadKeys(rawText, cfg)
	if !threadRoutingShouldReply(emp, ownerKey, mentioned) {
		return true
	}

	squadIDs := squadUserIDSet(cfg)
	userText := strings.TrimSpace(stripSquadUserMentions(rawText, squadIDs))
	if userText == "" {
		userText = "(no text besides squad mentions)"
	}
	if b.useAlexHints() && cfg.LLMAlexHints {
		userText = router.WrapAlexUserMessage(userText)
	}
	tc := threadTranscriptBefore(cfg, b.botUserID, msgs, threadTS, messageTS, cfg.LLMThreadMaxRunes)
	if tc != "" {
		userText = tc + "\n\n" + userText
	}

	b.postLLMReplyInThread(ctx, channel, userText, messageTS, threadTS)
	return true
}

func (b *Bot) postLLMReplyInThread(ctx context.Context, channel, userText, messageTS, threadTS string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.outbound != nil && !b.outbound.allow(now) {
		log.Printf("slack outbound rate limit: skipping thread reply (employee=%s channel=%s)", b.cfg.EmployeeID, channel)
		return
	}

	persona := b.persona.String()
	if persona == "" {
		persona = "You are a helpful assistant."
	}

	reply, err := b.llm.Reply(ctx, persona, slackReplySuffix, userText)
	if err != nil {
		log.Printf("llm reply error: %v", err)
		opts := []slack.MsgOption{
			slack.MsgOptionText(llmErrorUserMessage(err), false),
			slack.MsgOptionTS(threadTS),
		}
		if _, _, err := b.api.PostMessageContext(ctx, channel, opts...); err != nil {
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
	reply = b.repairOutboundReply(ctx, persona, userText, reply)
	reply = normalizeSlackReply(reply, b.cfg, b.botUserID)
	if b.cfg.MultiagentConfigured() {
		handoff, _ := shouldHandoff(
			b.cfg.MultiagentHandoffProbability,
			b.cfg.MultiagentHandoffMinProbability,
			b.cfg.MultiagentHandoffMaxProbability,
		)
		reply = enforceMultiagentMentionPolicy(reply, b.cfg, b.botUserID, handoff)
		reply = normalizeSlackReply(reply, b.cfg, b.botUserID)
	}
	opts := []slack.MsgOption{
		slack.MsgOptionText(reply, false),
		slack.MsgOptionTS(threadTS),
	}
	if _, _, err := b.api.PostMessageContext(ctx, channel, opts...); err != nil {
		log.Printf("slack post message: %v", err)
		return
	}
	if b.outbound != nil {
		b.outbound.record(time.Now())
	}
}
