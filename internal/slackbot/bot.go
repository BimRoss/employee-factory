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
		b.sm.Ack(*evt.Request)

		switch eventsAPI.Type {
		case slackevents.CallbackEvent:
			switch ev := eventsAPI.InnerEvent.Data.(type) {
			case *slackevents.MessageEvent:
				b.onMessage(ctx, ev)
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

	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return
	}

	channel := ev.Channel
	if channel == "" {
		return
	}

	isIM := strings.HasPrefix(channel, "D")
	if !isIM {
		mention := fmt.Sprintf("<@%s>", b.botUserID)
		if !strings.Contains(text, mention) {
			return
		}
		text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, mention, ""), "  ", " "))
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	system := b.persona.String()
	if system == "" {
		system = "You are a helpful assistant."
	}

	reply, err := b.llm.Reply(ctx, system, text)
	if err != nil {
		log.Printf("llm reply error: %v", err)
		_, _, _ = b.api.PostMessageContext(ctx, channel, slack.MsgOptionText("Sorry, I hit an error generating a reply.", false), slack.MsgOptionTS(ev.TimeStamp))
		return
	}
	if reply == "" {
		reply = "…"
	}

	opts := []slack.MsgOption{slack.MsgOptionText(reply, false)}
	threadTS := ev.ThreadTimeStamp
	if threadTS != "" {
		opts = append(opts, slack.MsgOptionTS(threadTS))
	} else if !isIM {
		opts = append(opts, slack.MsgOptionTS(ev.TimeStamp))
	}

	_, _, err = b.api.PostMessageContext(ctx, channel, opts...)
	if err != nil {
		log.Printf("slack post message: %v", err)
	}
}
