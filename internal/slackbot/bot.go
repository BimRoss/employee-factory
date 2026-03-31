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
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Appended after persona.md: Slack format + enforce voice/substance from whatever
// intelligence is in the loaded persona (any employee, any domain).
const slackReplySuffix = `

Slack reply rules (always follow)

Plain text only in the message body: this Slack app posts plain text, not mrkdwn. Do not use Markdown—no asterisk bold, no # headings, no backticks, no link syntax. Those show up as ugly raw punctuation. You may use numbered lines (1. 2. 3.) or bullet lines starting with - or • for structure.

Voice: Match the tone, diction, and reasoning style of the system persona above—this is who you are in Slack. Not a generic assistant.

Company name: **BimRoss** (capital B, capital R). Never write BenRoss, Ben Ross, BIMRAS, or Bimross.

Substance: When the persona defines frameworks, facts, or priorities, treat that text as authoritative. Prefer those definitions and labels over broad defaults from general knowledge.

Succinctness and tokens: Every word costs latency and money. Be **dense and complete**, not padded. Answer the **direct question first**, then add only what helps the thread or channel. Aim for **about 4–7 short lines** for a normal reply—roughly one Slack screen on mobile. Expand only when the user clearly asks for depth, a script, or step-by-step detail. **Do not** trail off mid-thought: finish sentences; if you are tight on space, shorten scope, not grammar.

Channel: You are in a **shared channel**—make the reply useful to others skimming the thread, not only a private lecture.

No filler: Do not repeat the same idea in different words, do not add “In summary / Overall / It’s important to note,” and do not pad with generic industry boilerplate.`

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
}

// New constructs a Socket Mode bot.
func New(cfg *config.Config, lm *llm.EmployeeLLM, p *persona.Loader) *Bot {
	api := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	window := time.Duration(cfg.SlackOutboundWindowSec) * time.Second
	return &Bot{
		cfg:      cfg,
		api:      api,
		sm:       socketmode.New(api),
		llm:      lm,
		persona:  p,
		outbound: newOutboundGate(window, cfg.SlackOutboundMaxPerWindow),
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
	if ev.BotID != "" || ev.SubType == "message_changed" || ev.SubType == "message_deleted" {
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

	// IMs: channel id often starts with "D"; App Home / some clients also set channel_type.
	isIM := strings.HasPrefix(channel, "D") || ev.ChannelType == "im" || ev.ChannelType == "mpim"
	if !isIM {
		mention := fmt.Sprintf("<@%s>", b.botUserID)
		if !strings.Contains(rawText, mention) {
			return
		}
		if b.dispatchMultiagentChannel(ctx, channel, rawText, ev.ThreadTimeStamp, ev.TimeStamp) {
			return
		}
		text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rawText, mention, ""), "  ", " "))
		if text == "" {
			return
		}
		b.postLLMReply(ctx, channel, text, ev.ThreadTimeStamp, ev.TimeStamp, isIM)
		return
	}

	b.postLLMReply(ctx, channel, rawText, ev.ThreadTimeStamp, ev.TimeStamp, isIM)
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
	if b.dispatchMultiagentChannel(ctx, channel, rawText, ev.ThreadTimeStamp, ev.TimeStamp) {
		return
	}
	mention := fmt.Sprintf("<@%s>", b.botUserID)
	text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rawText, mention, ""), "  ", " "))
	if text == "" {
		return
	}
	b.postLLMReply(ctx, channel, text, ev.ThreadTimeStamp, ev.TimeStamp, false)
}

// dispatchMultiagentChannel starts a sequential multi-bot session when squad env is configured
// and two or more squad bots are mentioned. Single-bot behavior stays in the caller.
func (b *Bot) dispatchMultiagentChannel(ctx context.Context, channel, rawText string, threadTS, messageTS string) bool {
	if !b.cfg.MultiagentConfigured() {
		return false
	}
	if len(mentionedSquadKeys(rawText, b.cfg)) < 2 {
		return false
	}
	go b.runMultiagentSession(ctx, channel, rawText, threadTS, messageTS, false)
	return true
}

func (b *Bot) postLLMReply(ctx context.Context, channel, userText, threadTS, messageTS string, isIM bool) {
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
	if tc := b.slackContextBlock(ctx, channel, threadTS, messageTS, isIM); tc != "" {
		userPayload = tc + "\n\n" + userPayload
	}

	reply, err := b.llm.Reply(ctx, persona, slackReplySuffix, userPayload)
	if err != nil {
		log.Printf("llm reply error: %v", err)
		opts := []slack.MsgOption{slack.MsgOptionText("Sorry, I hit an error generating a reply.", false)}
		if ts := threadReplyTS(threadTS); ts != "" {
			opts = append(opts, slack.MsgOptionTS(ts))
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

	opts := []slack.MsgOption{slack.MsgOptionText(reply, false)}
	if ts := threadReplyTS(threadTS); ts != "" {
		opts = append(opts, slack.MsgOptionTS(ts))
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

// threadReplyTS chooses Slack's thread_ts for PostMessage. We only thread when the user
// is already in a thread (threadTS set). For top-level channel @mentions we omit thread_ts
// so the bot replies on the main channel timeline—matching “chat with Alex in #sales”
// without collapsing everything into a thread under the first ping.
func threadReplyTS(threadTS string) string {
	return threadTS
}

func (b *Bot) useAlexHints() bool {
	id := strings.ToLower(strings.TrimSpace(b.cfg.EmployeeID))
	return id == "" || id == "alex"
}

// slackContextBlock adds prior turns: thread replies when thread_ts is set, otherwise
// recent DM/MPIM history (Slack does not set thread_ts on linear IM chat, so threads alone
// miss most 1:1 context).
func (b *Bot) slackContextBlock(ctx context.Context, channelID, threadTS, currentMsgTS string, isIM bool) string {
	if tc := b.threadContextBlock(ctx, channelID, threadTS, currentMsgTS); tc != "" {
		return tc
	}
	if isIM && strings.TrimSpace(currentMsgTS) != "" {
		if im := b.imHistoryContextBlock(ctx, channelID, currentMsgTS); im != "" {
			return im
		}
	}
	return ""
}

// threadContextBlock fetches prior messages in a Slack thread (no extra LLM calls).
func (b *Bot) threadContextBlock(ctx context.Context, channelID, threadTS, currentMsgTS string) string {
	if threadTS == "" {
		return ""
	}
	params := &slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTS,
		Limit:     b.cfg.LLMThreadMaxMessages,
	}
	msgs, _, _, err := b.api.GetConversationRepliesContext(ctx, params)
	if err != nil {
		log.Printf("thread fetch: %v", err)
		return ""
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
		role := "user"
		if m.BotID != "" || m.User == b.botUserID {
			role = "assistant"
		}
		lines = append(lines, "["+role+"] "+text)
	}
	if len(lines) == 0 {
		return ""
	}
	out := "Earlier in this Slack thread (oldest first):\n" + strings.Join(lines, "\n")
	r := []rune(out)
	if len(r) > b.cfg.LLMThreadMaxRunes {
		out = "…[thread truncated; oldest lines dropped]\n" + string(r[len(r)-b.cfg.LLMThreadMaxRunes:])
	}
	return out
}

// imHistoryContextBlock loads messages in this DM/MPIM before the current message via
// conversations.history (requires im:history / mpim:history as applicable).
func (b *Bot) imHistoryContextBlock(ctx context.Context, channelID, currentMsgTS string) string {
	params := &slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    currentMsgTS,
		Inclusive: false,
		Limit:     b.cfg.LLMThreadMaxMessages,
	}
	resp, err := b.api.GetConversationHistoryContext(ctx, params)
	if err != nil {
		log.Printf("im history fetch: %v", err)
		return ""
	}
	if len(resp.Messages) == 0 {
		return ""
	}
	// API returns newest-first; present oldest-first to match threadContextBlock.
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
	out := "Earlier in this Slack conversation (oldest first):\n" + strings.Join(lines, "\n")
	r := []rune(out)
	if len(r) > b.cfg.LLMThreadMaxRunes {
		out = "…[conversation truncated; oldest lines dropped]\n" + string(r[len(r)-b.cfg.LLMThreadMaxRunes:])
	}
	return out
}
