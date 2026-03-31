package slackbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/llm"
	"github.com/bimross/employee-factory/internal/persona"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Appended after persona.md: Slack format + enforce voice/substance from whatever
// intelligence is in the loaded persona (any employee, any domain).
const slackReplySuffix = `

Slack reply rules (always follow)

Plain text only in the message body: this Slack app posts plain text, not mrkdwn. Do not use Markdown—no asterisk bold, no # headings, no backticks, no link syntax. Those show up as ugly raw punctuation. You may use numbered lines (1. 2. 3.) or bullet lines starting with - or • for structure.

Voice: Match the tone, diction, and reasoning style of the system persona above—this is who you are in Slack. Sound like that voice, not a generic assistant.

Substance: When the persona defines frameworks, facts, or priorities, treat that text as authoritative. Prefer those definitions and labels over broad defaults from general knowledge. If something is spelled out above, use it; do not substitute a parallel answer you “know from elsewhere.”

Length (hard): Aim for about 5–8 short lines for a typical answer—roughly one short Slack screen. Go longer only if the user explicitly asks for depth, a full script, or step-by-step detail. Otherwise: answer the question and stop.

No filler: Do not repeat the same idea in different words, do not add “In summary / Overall / It’s important to note,” and do not pad with generic industry boilerplate.`

// Bot runs Slack Socket Mode and responds using Cogito + persona.
type Bot struct {
	cfg     *config.Config
	api     *slack.Client
	sm      *socketmode.Client
	llm     *llm.EmployeeLLM
	persona *persona.Loader

	botUserID string
	mu        sync.Mutex
}

// New constructs a Socket Mode bot.
func New(cfg *config.Config, lm *llm.EmployeeLLM, p *persona.Loader) *Bot {
	api := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	return &Bot{
		cfg:     cfg,
		api:     api,
		sm:      socketmode.New(api),
		llm:     lm,
		persona: p,
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
	text := strings.TrimSpace(ev.Text)
	if text == "" && ev.Message != nil {
		text = strings.TrimSpace(ev.Message.Text)
	}
	if channel == "" || text == "" {
		return
	}

	// IMs: channel id often starts with "D"; App Home / some clients also set channel_type.
	isIM := strings.HasPrefix(channel, "D") || ev.ChannelType == "im" || ev.ChannelType == "mpim"
	if !isIM {
		mention := fmt.Sprintf("<@%s>", b.botUserID)
		if !strings.Contains(text, mention) {
			return
		}
		text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, mention, ""), "  ", " "))
	}

	b.postLLMReply(ctx, channel, text, ev.ThreadTimeStamp, ev.TimeStamp, isIM)
}

func (b *Bot) onAppMention(ctx context.Context, ev *slackevents.AppMentionEvent) {
	if ev == nil || ev.User == b.botUserID {
		return
	}
	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return
	}
	mention := fmt.Sprintf("<@%s>", b.botUserID)
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, mention, ""), "  ", " "))
	if text == "" {
		return
	}
	channel := strings.TrimSpace(ev.Channel)
	if channel == "" {
		return
	}
	b.postLLMReply(ctx, channel, text, ev.ThreadTimeStamp, ev.TimeStamp, false)
}

func (b *Bot) postLLMReply(ctx context.Context, channel, userText, threadTS, parentMessageTS string, isIM bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	system := b.persona.String()
	if system == "" {
		system = "You are a helpful assistant."
	}
	system += slackReplySuffix

	reply, err := b.llm.Reply(ctx, system, userText)
	if err != nil {
		log.Printf("llm reply error: %v", err)
		opts := []slack.MsgOption{slack.MsgOptionText("Sorry, I hit an error generating a reply.", false)}
		if parentMessageTS != "" {
			opts = append(opts, slack.MsgOptionTS(parentMessageTS))
		}
		_, _, _ = b.api.PostMessageContext(ctx, channel, opts...)
		return
	}
	if reply == "" {
		reply = "…"
	}

	opts := []slack.MsgOption{slack.MsgOptionText(reply, false)}
	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	} else if !isIM && parentMessageTS != "" {
		opts = append(opts, slack.MsgOptionTS(parentMessageTS))
	}

	_, _, err = b.api.PostMessageContext(ctx, channel, opts...)
	if err != nil {
		log.Printf("slack post message: %v", err)
	}
}
