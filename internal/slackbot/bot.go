package slackbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/llm"
	"github.com/bimross/employee-factory/internal/persona"
	"github.com/bimross/employee-factory/internal/router"
	"github.com/bimross/employee-factory/internal/threadstore"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Appended after persona.md: Slack format + enforce voice/substance from whatever
// intelligence is in the loaded persona (any employee, any domain).
const slackReplySuffix = `

Slack reply rules (always follow)

Formatting: Slack uses mrkdwn (not GitHub Markdown). For bold use *single* asterisk pairs only, like *this*—never double asterisks. The pipeline strips stray ** but you must not emit them. Inline code: one backtick. No # headings or [text](url); paste plain https URLs if needed.

Voice: Match the tone, diction, and reasoning style of the system persona above—this is who you are in Slack. Not a generic assistant.

Company name: BimRoss (capital B, capital R). Never write BenRoss, Ben Ross, BIMRAS, or Bimross.

Substance: When the persona defines frameworks, facts, or priorities, treat that text as authoritative—but do not dump every framework as a sectioned essay. Apply judgment: one sharp take beats a catalog.

Succinctness and tokens: Every word costs latency and money. Default: one to two short lines total. Lead with the answer. If the question is prioritization (“what next,” “what should we work on,” “best move”), give one concrete pick on the first line and one short support line at most. Do not produce themed sections, pillar lists, or long “1–N” breakdowns unless the user explicitly asks for that format. Expand only when they ask for depth, steps, or a deliberate list.

Channel: You are in a shared channel—make the reply scannable in seconds.

@mentions and mini-coordination: You may @ross @tim @alex @garth (lowercase is fine) when handing off a next step, narrowing scope, building on or challenging a specific point, or making responsibility explicit—so the channel sees real coordination. Never @mention yourself (if you are Tim, do not write @tim). Avoid empty “+1” or “X nailed it” with no new substance; if you @ someone, add a concrete addition or question. One or two mentions per reply is usually enough.

Multi-agent turns: If another bot already answered above you, do not copy their line. Add a distinct angle—risk, tradeoff, metric, or the next step they skipped—or ask them one sharp clarifying question with an @mention if needed.

No filler: Do not repeat the same idea in different words or pad with “In summary / Overall.” Finish sentences; if tight on space, cut scope, not grammar.`

// Bot runs Slack Socket Mode and responds using OpenAI-compatible chat + persona.
type Bot struct {
	cfg     *config.Config
	api     *slack.Client
	sm      *socketmode.Client
	llm     *llm.EmployeeLLM
	persona *persona.Loader

	botUserID string
	mu        sync.Mutex
	outbound  *outboundGate

	// threadOwner persists human-root thread owners when Redis is configured (optional cache).
	threadOwner threadstore.OwnerStore
}

// New constructs a Socket Mode bot. owner may be nil (human-root owner is inferred from thread history).
func New(cfg *config.Config, lm *llm.EmployeeLLM, p *persona.Loader, owner threadstore.OwnerStore) *Bot {
	api := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	window := time.Duration(cfg.SlackOutboundWindowSec) * time.Second
	return &Bot{
		cfg:         cfg,
		api:         api,
		sm:          socketmode.New(api),
		llm:         lm,
		persona:     p,
		outbound:    newOutboundGate(window, cfg.SlackOutboundMaxPerWindow),
		threadOwner: owner,
	}
}

// Run blocks until context is cancelled or the socket connection fails fatally.
func (b *Bot) Run(ctx context.Context) error {
	auth, err := b.api.AuthTest()
	if err != nil {
		return fmt.Errorf("slack auth.test: %w", err)
	}
	b.botUserID = auth.UserID
	log.Printf("slack connected as bot user_id=%s team=%s", b.botUserID, auth.Team)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-b.sm.Events:
				if !ok {
					return
				}
				b.handleEvent(ctx, evt)
			}
		}
	}()

	return b.sm.RunContext(ctx)
}

func (b *Bot) handleEvent(ctx context.Context, evt socketmode.Event) {
	switch evt.Type {
	case socketmode.EventTypeConnecting:
		log.Printf("slack socketmode connecting")
	case socketmode.EventTypeConnectionError:
		log.Printf("slack socketmode connection error: %v", evt.Data)
	case socketmode.EventTypeEventsAPI:
		eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
		if !ok {
			return
		}
		if evt.Request != nil {
			b.sm.Ack(*evt.Request)
		}

		switch eventsAPI.Type {
		case slackevents.CallbackEvent:
			switch ev := eventsAPI.InnerEvent.Data.(type) {
			case *slackevents.MessageEvent:
				b.onMessage(ctx, ev)
			case *slackevents.AppMentionEvent:
				b.onAppMention(ctx, ev)
			default:
				log.Printf("slack: unhandled Events API inner type %T", eventsAPI.InnerEvent.Data)
			}
		case slackevents.URLVerification:
			// Ack above satisfies Socket Mode; no extra work.
		default:
			// ignore
		}
	}
}

func (b *Bot) onMessage(ctx context.Context, ev *slackevents.MessageEvent) {
	if ev == nil {
		return
	}
	if ev.SubType == "message_changed" || ev.SubType == "message_deleted" {
		return
	}
	if ev.User == b.botUserID {
		return
	}

	channel := strings.TrimSpace(ev.Channel)
	if channel == "" && ev.Message != nil {
		channel = strings.TrimSpace(ev.Message.Channel)
	}
	rawText := strings.TrimSpace(ev.Text)
	if rawText == "" && ev.Message != nil {
		rawText = strings.TrimSpace(ev.Message.Text)
	}
	if channel == "" || rawText == "" {
		return
	}

	// BimRoss policy: one open channel (#chat-style), no DMs—ignore IMs.
	if strings.HasPrefix(channel, "D") || ev.ChannelType == "im" || ev.ChannelType == "mpim" {
		return
	}
	if ts := strings.TrimSpace(ev.ThreadTimeStamp); ts != "" {
		if b.cfg.ThreadsEnabled() {
			b.handleThreadMessage(ctx, channel, ev.User, rawText, ev.TimeStamp, ts)
		}
		return
	}

	// Another squad bot @mentioned this bot—organic follow-up (not a second multiagent lap).
	if ev.BotID != "" {
		if b.trySquadBotMentionTrigger(ctx, channel, rawText, ev) {
			return
		}
		return
	}

	mention := fmt.Sprintf("<@%s>", b.botUserID)
	if strings.Contains(rawText, mention) {
		if b.dispatchMultiagentChannel(ctx, channel, rawText, ev.TimeStamp) {
			return
		}
		text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rawText, mention, ""), "  ", " "))
		if text == "" {
			return
		}
		b.postLLMReply(ctx, channel, text, ev.TimeStamp)
		return
	}
	// message.channels: @everyone without bot @mentions; each bot runs the same session with a
	// SHA-256–seeded random turn order (see shuffleBroadcastParticipants) and MULTIAGENT_BROADCAST_ROUNDS passes.
	if b.dispatchBroadcastMultiagent(ctx, channel, rawText, ev.TimeStamp) {
		return
	}
}

func (b *Bot) onAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
	if ev == nil || ev.User == b.botUserID {
		return
	}
	rawText := strings.TrimSpace(ev.Text)
	if rawText == "" {
		return
	}
	channel := strings.TrimSpace(ev.Channel)
	if channel == "" {
		return
	}
	if strings.HasPrefix(channel, "D") {
		return
	}
	if ts := strings.TrimSpace(ev.ThreadTimeStamp); ts != "" {
		if b.cfg.ThreadsEnabled() {
			b.handleThreadMessage(ctx, channel, ev.User, rawText, ev.TimeStamp, ts)
		}
		return
	}
	if b.dispatchMultiagentChannel(ctx, channel, rawText, ev.TimeStamp) {
		return
	}
	mention := fmt.Sprintf("<@%s>", b.botUserID)
	text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rawText, mention, ""), "  ", " "))
	if text == "" {
		return
	}
	b.postLLMReply(ctx, channel, text, ev.TimeStamp)
}

// trySquadBotMentionTrigger handles messages posted by a squad bot that @mention this bot.
// Returns true if the event was handled (including skipped due to run cap).
func (b *Bot) trySquadBotMentionTrigger(ctx context.Context, channel, rawText string, ev *slackevents.MessageEvent) bool {
	if !b.cfg.MultiagentConfigured() {
		return false
	}
	squad := squadUserIDSet(b.cfg)
	if !squad[ev.User] {
		return false
	}
	mention := fmt.Sprintf("<@%s>", b.botUserID)
	if !strings.Contains(rawText, mention) {
		return false
	}

	n, err := b.squadRunCountThrough(ctx, channel, ev.TimeStamp)
	if err != nil {
		log.Printf("multiagent: squad run count: %v", err)
		return true
	}
	max := b.cfg.MultiagentSquadRunMax
	if max > 0 && n >= max {
		log.Printf("multiagent: squad run cap (%d) reached (n=%d), skipping bot-mention reply", max, n)
		return true
	}

	text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rawText, mention, ""), "  ", " "))
	if text == "" {
		text = "(no text besides mention)"
	}
	payload := "A squad bot addressed you in-channel:\n" + text
	b.postLLMReply(ctx, channel, payload, ev.TimeStamp)
	return true
}

// squadRunCountThrough counts squad-bot messages in the current run through the message at throughTS (inclusive).
func (b *Bot) squadRunCountThrough(ctx context.Context, channelID, throughTS string) (int, error) {
	squad := squadUserIDSet(b.cfg)
	limit := 100
	params := &slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    throughTS,
		Inclusive: true,
		Limit:     limit,
	}
	resp, err := b.api.GetConversationHistoryContext(ctx, params)
	if err != nil {
		return 0, err
	}
	msgs := append([]slack.Message(nil), resp.Messages...)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	idx := -1
	target := parseSlackTSToFloat(throughTS)
	for i := range msgs {
		if msgs[i].Timestamp == throughTS {
			idx = i
			break
		}
	}
	if idx == -1 {
		for i := range msgs {
			if parseSlackTSToFloat(msgs[i].Timestamp) == target {
				idx = i
				break
			}
		}
	}
	if idx == -1 {
		return 0, fmt.Errorf("through message not in history window")
	}
	return countSquadMessagesInRun(msgs, squad, idx), nil
}

// dispatchMultiagentChannel starts a sequential multi-bot session when squad env is configured
// and two or more squad bots are mentioned. Single-bot behavior stays in the caller.
func (b *Bot) dispatchMultiagentChannel(ctx context.Context, channel, rawText string, messageTS string) bool {
	if !b.cfg.MultiagentConfigured() {
		return false
	}
	participants := mentionedSquadKeys(rawText, b.cfg)
	if len(participants) < 2 {
		return false
	}
	go b.runMultiagentSession(ctx, channel, rawText, messageTS, participants, multiagentSquadPasses, b.cfg.MultiagentHandoffProbability)
	return true
}

// dispatchBroadcastMultiagent handles @everyone (Slack <!everyone>) when no bot
// is @mentioned. Each squad bot receives message.channels and runs the same session: each process only
// posts when the turn is that bot’s Slack user id (see runMultiagentSession)—so every squad bot must run the
// session, not just MULTIAGENT_ORDER[0].
func (b *Bot) dispatchBroadcastMultiagent(ctx context.Context, channel, rawText string, messageTS string) bool {
	if !b.cfg.MultiagentConfigured() {
		return false
	}
	if !broadcastMultiagentTrigger(rawText) {
		return false
	}
	participants := shuffleBroadcastParticipants(messageTS, b.cfg.MultiagentOrder, b.cfg.MultiagentShuffleSecret)
	if len(participants) < 2 {
		return false
	}
	rounds := b.cfg.MultiagentBroadcastRounds
	if rounds < 1 {
		rounds = 1
	}
	effectiveHandoff := b.cfg.MultiagentBroadcastHandoffProbability
	if b.cfg.MultiagentBroadcastBranchingEnabled && shouldUseBroadcastBranchMode(
		messageTS,
		b.cfg.MultiagentOrder,
		b.cfg.MultiagentShuffleSecret,
		b.cfg.MultiagentBroadcastBranchingProbability,
	) {
		effectiveHandoff = b.cfg.MultiagentBroadcastBranchingHandoffProbability
	}
	go b.runMultiagentSession(ctx, channel, rawText, messageTS, participants, rounds, effectiveHandoff)
	return true
}

func (b *Bot) postLLMReply(ctx context.Context, channel, userText, messageTS string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.outbound != nil && !b.outbound.allow(now) {
		emp := strings.TrimSpace(b.cfg.EmployeeID)
		if emp == "" {
			emp = "default"
		}
		log.Printf("slack outbound rate limit: skipping reply (employee=%s channel=%s)", emp, channel)
		return
	}

	persona := b.persona.String()
	if persona == "" {
		persona = "You are a helpful assistant."
	}

	userPayload := strings.TrimSpace(userText)
	if b.useAlexHints() && b.cfg.LLMAlexHints {
		userPayload = router.WrapAlexUserMessage(userPayload)
	}
	if tc := b.channelHistoryContextBlock(ctx, channel, messageTS); tc != "" {
		userPayload = tc + "\n\n" + userPayload
	}

	reply, err := b.llm.Reply(ctx, persona, slackReplySuffix, userPayload)
	if err != nil {
		log.Printf("llm reply error: %v", err)
		opts := []slack.MsgOption{slack.MsgOptionText("Quick take: resend that and I will answer directly in one clean pass.", false)}
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
	reply = b.repairOutboundReply(ctx, persona, userPayload, reply)
	reply = normalizeSlackReply(reply, b.cfg, b.botUserID)
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

func (b *Bot) useAlexHints() bool {
	id := strings.ToLower(strings.TrimSpace(b.cfg.EmployeeID))
	return id == "" || id == "alex"
}

// channelHistoryContextBlock loads recent messages on the channel timeline before the current
// message (conversations.history). No threads/DMs—one open channel, linear context.
func (b *Bot) channelHistoryContextBlock(ctx context.Context, channelID, currentMsgTS string) string {
	if strings.TrimSpace(currentMsgTS) == "" {
		return ""
	}
	params := &slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    currentMsgTS,
		Inclusive: false,
		Limit:     b.cfg.LLMThreadMaxMessages,
	}
	resp, err := b.api.GetConversationHistoryContext(ctx, params)
	if err != nil {
		log.Printf("channel history fetch: %v", err)
		return ""
	}
	if len(resp.Messages) == 0 {
		return ""
	}
	msgs := append([]slack.Message(nil), resp.Messages...)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	var lines []string
	for _, m := range msgs {
		if m.Timestamp == currentMsgTS {
			continue
		}
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		if m.SubType == "message_changed" || m.SubType == "message_deleted" {
			continue
		}
		role := "user"
		if m.BotID != "" || m.User == b.botUserID {
			role = "assistant"
		}
		lines = append(lines, "["+role+"] "+text)
	}
	if len(lines) == 0 {
		return ""
	}
	out := "Earlier in this channel (oldest first):\n" + strings.Join(lines, "\n")
	if b.cfg.LLMChannelIncludeThreads {
		if sn := b.channelThreadSnippetsForMessages(ctx, channelID, msgs); sn != "" {
			out = out + "\n\n" + sn
		}
	}
	r := []rune(out)
	if len(r) > b.cfg.LLMThreadMaxRunes {
		out = "…[truncated; oldest lines dropped]\n" + string(r[len(r)-b.cfg.LLMThreadMaxRunes:])
	}
	return out
}

// channelThreadSnippetsForMessages appends compact thread reply text for recent top-level messages with replies.
func (b *Bot) channelThreadSnippetsForMessages(ctx context.Context, channelID string, msgsOldestFirst []slack.Message) string {
	scan := b.cfg.LLMChannelThreadParentScan
	if scan < 1 {
		scan = 4
	}
	maxR := b.cfg.LLMChannelThreadRepliesMax
	if maxR < 1 {
		maxR = 15
	}
	var sections []string
	n := 0
	for i := len(msgsOldestFirst) - 1; i >= 0 && n < scan; i-- {
		m := msgsOldestFirst[i]
		if strings.TrimSpace(m.ThreadTimestamp) != "" {
			continue
		}
		if m.ReplyCount < 1 {
			continue
		}
		n++
		threadMsgs, err := b.fetchThreadMessages(ctx, channelID, m.Timestamp)
		if err != nil {
			log.Printf("channel thread snippet fetch: %v", err)
			continue
		}
		if len(threadMsgs) <= 1 {
			continue
		}
		var sub []string
		count := 0
		for _, tm := range threadMsgs {
			if tm.Timestamp == m.Timestamp {
				continue
			}
			t := strings.TrimSpace(tm.Text)
			if t == "" {
				continue
			}
			if tm.SubType == "message_changed" || tm.SubType == "message_deleted" {
				continue
			}
			role := "user"
			if tm.BotID != "" || tm.User == b.botUserID {
				role = "assistant"
			} else if sk, ok := squadKeyForSlackUser(b.cfg, tm.User); ok {
				role = sk
			}
			sub = append(sub, fmt.Sprintf("[%s] %s", role, t))
			count++
			if count >= maxR {
				break
			}
		}
		if len(sub) == 0 {
			continue
		}
		sections = append(sections, fmt.Sprintf("Thread under message ts=%s (%d replies): %s", m.Timestamp, m.ReplyCount, strings.Join(sub, " | ")))
	}
	if len(sections) == 0 {
		return ""
	}
	out := "Thread snippets (recent parents with replies):\n" + strings.Join(sections, "\n")
	r := []rune(out)
	const maxSnip = 6000
	if len(r) > maxSnip {
		out = "…[thread snippets truncated]\n" + string(r[len(r)-maxSnip:])
	}
	return out
}
