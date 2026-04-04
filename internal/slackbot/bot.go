package slackbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/bimross/employee-factory/internal/gmailsender"
	"github.com/bimross/employee-factory/internal/lessons"
	"github.com/bimross/employee-factory/internal/llm"
	"github.com/bimross/employee-factory/internal/opsproxy"
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

Hostility protocol: If the user is verbally aggressive toward you, do not use appeasing filler (for example, "okay, I can help with that"). Use one brief firm pushback line with light emotional mirroring, then immediately redirect to a concrete next action.

Rethink protocol: If the prompt includes "Rethink trigger", your first line must explicitly acknowledge reassessment and reflect the user's correction. Your second line must provide an updated position/confidence and one concrete next action. Do not repeat your prior thesis unchanged.

Self-reference: In plain text, refer to yourself as "me" (or "I"), never by your own name (Ross/Tim/Alex/Garth).

Company name: BimRoss (capital B, capital R). Never write BenRoss, Ben Ross, BIMRAS, or Bimross.

Substance: When the persona defines frameworks, facts, or priorities, treat that text as authoritative—but do not dump every framework as a sectioned essay. Apply judgment: one sharp take beats a catalog.

Succinctness and tokens: Every word costs latency and money. Default: one to two short lines total. Lead with the answer. If the question is prioritization (“what next,” “what should we work on,” “best move”), give one concrete pick on the first line and one short support line at most. Do not produce themed sections, pillar lists, or long “1–N” breakdowns unless the user explicitly asks for that format. Expand only when they ask for depth, steps, or a deliberate list.

Channel: You are in a shared channel—make the reply scannable in seconds.

@mentions and mini-coordination: You may @ross @tim @alex @garth (lowercase is fine) when handing off a next step, narrowing scope, building on or challenging a specific point, or making responsibility explicit—so the channel sees real coordination. Never @mention yourself (if you are Tim, do not write @tim). Avoid empty “+1” or “X nailed it” with no new substance; if you @ someone, add a concrete addition or question. Pick the mention by unresolved lane; do not default to @ross unless implementation/code/infra execution is the blocker. One or two mentions per reply is usually enough.

Multi-agent turns: If another bot already answered above you, do not copy their line. Add a distinct angle—risk, tradeoff, metric, or the next step they skipped—or ask them one sharp clarifying question with an @mention if needed.

No filler: Do not repeat the same idea in different words or pad with “In summary / Overall.” Finish sentences; if tight on space, cut scope, not grammar.

Opener variation: Avoid reusing the same discourse opener/catchphrase in consecutive replies in the same channel or thread.
Opener hygiene: In normal replies, do not start with sterile acknowledgment stubs like "Acknowledging.", "Noted.", or "Understood." as a standalone first line. Open with the concrete move, decision, or answer.`

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
	routeMu   sync.Mutex
	// activeBroadcastByChannel tracks in-flight broadcast sessions per channel.
	activeBroadcastByChannel map[string]int

	// threadOwner persists human-root thread owners when Redis is configured (optional cache).
	threadOwner threadstore.OwnerStore
	// generalAutoReplyLock coordinates cross-pod single-reaction behavior for plain #general messages.
	generalAutoReplyLock *generalAutoReplyLocker
	gmailSender          *gmailsender.Sender
	opsProxyClient       *opsproxy.Client
	runtimeLessons       *lessons.Manager
}

// New constructs a Socket Mode bot. owner may be nil (human-root owner is inferred from thread history).
func New(cfg *config.Config, lm *llm.EmployeeLLM, p *persona.Loader, owner threadstore.OwnerStore) *Bot {
	api := slack.New(cfg.SlackBotToken, slack.OptionAppLevelToken(cfg.SlackAppToken))
	window := time.Duration(cfg.SlackOutboundWindowSec) * time.Second
	var sender *gmailsender.Sender
	var opsClient *opsproxy.Client
	var runtimeLessonsStore lessons.Store = lessons.NoopStore{}
	if strings.EqualFold(strings.TrimSpace(cfg.EmployeeID), "joanne") && cfg.JoanneEmailEnabled {
		s, err := gmailsender.New(cfg)
		if err != nil {
			log.Printf("gmail sender init: %v", err)
		} else {
			sender = s
		}
	}
	if strings.EqualFold(strings.TrimSpace(cfg.EmployeeID), "ross") && cfg.RossOpsEnabled {
		client, err := opsproxy.NewClient(cfg.RossOpsProxyURL, cfg.RossOpsProxyToken, 12*time.Second)
		if err != nil {
			log.Printf("ross ops proxy client init: %v", err)
		} else {
			opsClient = client
		}
	}
	if strings.TrimSpace(cfg.RedisURL) != "" {
		store, err := lessons.NewRedisStore(strings.TrimSpace(cfg.RedisURL))
		if err != nil {
			log.Printf("runtime lessons redis init: %v", err)
		} else {
			runtimeLessonsStore = store
		}
	}
	return &Bot{
		cfg:                      cfg,
		api:                      api,
		sm:                       socketmode.New(api),
		llm:                      lm,
		persona:                  p,
		outbound:                 newOutboundGate(window, cfg.SlackOutboundMaxPerWindow),
		threadOwner:              owner,
		generalAutoReplyLock:     newGeneralAutoReplyLocker(strings.TrimSpace(cfg.RedisURL)),
		activeBroadcastByChannel: map[string]int{},
		gmailSender:              sender,
		opsProxyClient:           opsClient,
		runtimeLessons: lessons.New(lessons.Config{
			Enabled:        cfg.LessonsEnabled,
			LogOnly:        cfg.LessonsLogOnly,
			AutoApply:      cfg.LessonsAutoApply,
			MinConfidence:  cfg.LessonsMinConfidence,
			MaxActive:      cfg.LessonsMaxActive,
			MaxEvents:      cfg.LessonsMaxEvents,
			TTL:            time.Duration(cfg.LessonsTTLSeconds) * time.Second,
			MaxPromptRunes: cfg.LessonsMaxPromptRunes,
		}, runtimeLessonsStore),
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
	case socketmode.EventTypeConnected:
		log.Printf("slack socketmode connected")
	case socketmode.EventTypeDisconnect:
		log.Printf("slack socketmode disconnect: %v", evt.Data)
	case socketmode.EventTypeConnectionError:
		log.Printf("slack socketmode connection error: %v", evt.Data)
	case socketmode.EventTypeInvalidAuth:
		log.Printf("slack socketmode invalid auth: %v", evt.Data)
	case socketmode.EventTypeIncomingError:
		log.Printf("slack socketmode incoming error: %v", evt.Data)
	case socketmode.EventTypeErrorWriteFailed:
		log.Printf("slack socketmode write error: %v", evt.Data)
	case socketmode.EventTypeErrorBadMessage:
		log.Printf("slack socketmode bad message: %v", evt.Data)
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
	if !b.cfg.ChannelAllowed(channel) {
		log.Printf("slack_route: path=skip_channel_not_allowed employee=%s channel=%s", strings.TrimSpace(b.cfg.EmployeeID), channel)
		return
	}

	// BimRoss policy: one open channel (#chat-style), no DMs—ignore IMs.
	if strings.HasPrefix(channel, "D") || ev.ChannelType == "im" || ev.ChannelType == "mpim" {
		return
	}
	if b.applyAvailabilityRouter(ctx, availabilityRouteEvent{
		Path:      "message",
		Channel:   channel,
		MessageTS: ev.TimeStamp,
		ThreadTS:  ev.ThreadTimeStamp,
		RawText:   rawText,
		Phase:     routerPhaseIngress,
	}) {
		return
	}
	if b.tryHandleRossOps(ctx, channel, rawText, ev.User, ev.TimeStamp, "") {
		return
	}
	if b.tryHandleJoanneSendEmail(ctx, channel, rawText, ev.User, ev.TimeStamp, "") {
		return
	}
	if ts := strings.TrimSpace(ev.ThreadTimeStamp); ts != "" {
		if b.cfg.ThreadsEnabled() {
			log.Printf("slack_route: path=thread employee=%s channel=%s thread_ts=%s", strings.TrimSpace(b.cfg.EmployeeID), channel, ts)
			b.handleThreadMessage(ctx, channel, ev.User, rawText, ev.TimeStamp, ts)
		}
		return
	}

	// Another squad bot @mentioned this bot—organic follow-up (not a second multiagent lap).
	if ev.BotID != "" {
		if b.trySquadBotMentionTrigger(ctx, channel, rawText, ev) {
			log.Printf("slack_route: path=squad_bot_followup employee=%s channel=%s anchor=%s", strings.TrimSpace(b.cfg.EmployeeID), channel, strings.TrimSpace(ev.TimeStamp))
			return
		}
		return
	}

	route := decideChannelRoute(rawText, b.cfg)
	if route == routeBroadcastPresenceAck {
		if b.dispatchBroadcastPresenceAck(ctx, channel, rawText, ev.TimeStamp) {
			log.Printf("slack_route: path=broadcast_presence_ack employee=%s channel=%s anchor=%s", strings.TrimSpace(b.cfg.EmployeeID), channel, strings.TrimSpace(ev.TimeStamp))
		} else {
			log.Printf("slack_route: path=broadcast_presence_ack_skipped employee=%s channel=%s anchor=%s", strings.TrimSpace(b.cfg.EmployeeID), channel, strings.TrimSpace(ev.TimeStamp))
		}
		return
	}
	if route == routeBroadcastNovelty {
		if b.dispatchBroadcastMultiagent(ctx, channel, rawText, ev.TimeStamp) {
			log.Printf("slack_route: path=broadcast employee=%s channel=%s anchor=%s", strings.TrimSpace(b.cfg.EmployeeID), channel, strings.TrimSpace(ev.TimeStamp))
		} else {
			log.Printf("slack_route: path=broadcast_skipped employee=%s channel=%s anchor=%s", strings.TrimSpace(b.cfg.EmployeeID), channel, strings.TrimSpace(ev.TimeStamp))
		}
		return
	}

	mention := fmt.Sprintf("<@%s>", b.botUserID)
	if strings.Contains(rawText, mention) {
		if b.dispatchMultiagentChannel(ctx, channel, rawText, ev.TimeStamp) {
			log.Printf("slack_route: path=multiagent_mentions employee=%s channel=%s anchor=%s", strings.TrimSpace(b.cfg.EmployeeID), channel, strings.TrimSpace(ev.TimeStamp))
			return
		}
		text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rawText, mention, ""), "  ", " "))
		if text == "" {
			return
		}
		log.Printf("slack_route: path=single_mention employee=%s channel=%s anchor=%s", strings.TrimSpace(b.cfg.EmployeeID), channel, strings.TrimSpace(ev.TimeStamp))
		b.postLLMReply(ctx, channel, text, ev.TimeStamp, ev.User)
		return
	}
	if b.dispatchGeneralAutoReaction(ctx, channel, rawText, ev) {
		log.Printf("slack_route: path=general_auto_reaction employee=%s channel=%s anchor=%s", strings.TrimSpace(b.cfg.EmployeeID), channel, strings.TrimSpace(ev.TimeStamp))
		return
	}
}

func shouldRouteAsBroadcast(rawText string, cfg *config.Config) bool {
	return cfg != nil && cfg.MultiagentConfigured() && broadcastMultiagentTrigger(rawText)
}

type channelRouteKind string

const (
	routeNormal               channelRouteKind = "normal"
	routeBroadcastPresenceAck channelRouteKind = "broadcast_presence_ack"
	routeBroadcastNovelty     channelRouteKind = "broadcast_novelty"
)

func decideChannelRoute(rawText string, cfg *config.Config) channelRouteKind {
	if !shouldRouteAsBroadcast(rawText, cfg) {
		return routeNormal
	}
	presence := router.ClassifyPresenceCheck(rawText)
	if presence.IsPresenceCheck {
		return routeBroadcastPresenceAck
	}
	return routeBroadcastNovelty
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
	if !b.cfg.ChannelAllowed(channel) {
		log.Printf("slack_route: path=skip_channel_not_allowed employee=%s channel=%s", strings.TrimSpace(b.cfg.EmployeeID), channel)
		return
	}
	if strings.HasPrefix(channel, "D") {
		return
	}
	if b.applyAvailabilityRouter(ctx, availabilityRouteEvent{
		Path:      "app_mention",
		Channel:   channel,
		MessageTS: ev.TimeStamp,
		ThreadTS:  ev.ThreadTimeStamp,
		RawText:   rawText,
		Phase:     routerPhaseIngress,
	}) {
		return
	}
	if b.tryHandleRossOps(ctx, channel, rawText, ev.User, ev.TimeStamp, strings.TrimSpace(ev.ThreadTimeStamp)) {
		return
	}
	if b.tryHandleJoanneSendEmail(ctx, channel, rawText, ev.User, ev.TimeStamp, strings.TrimSpace(ev.ThreadTimeStamp)) {
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
	b.postLLMReply(ctx, channel, text, ev.TimeStamp, ev.User)
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
	if b.isBroadcastActive(channel) {
		log.Printf("multiagent: skip bot-mention followup reason=broadcast_active employee=%s channel=%s anchor=%s",
			strings.TrimSpace(b.cfg.EmployeeID), strings.TrimSpace(channel), strings.TrimSpace(ev.TimeStamp))
		return true
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
	b.postLLMReply(ctx, channel, payload, ev.TimeStamp, ev.User)
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
	basePool := orderedBroadcastPool(b.cfg)
	pool := resolveBroadcastCandidatePool(rawText, b.cfg)
	if removed := removedPoolKeys(basePool, pool); len(removed) > 0 {
		log.Printf("multiagent: broadcast pool filtered employee=%s anchor=%s removed=%s trigger=%q",
			strings.TrimSpace(b.cfg.EmployeeID),
			strings.TrimSpace(messageTS),
			strings.Join(removed, ","),
			strings.TrimSpace(rawText),
		)
	}
	participants := shuffleBroadcastParticipants(messageTS, pool, b.cfg.MultiagentShuffleSecret)
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
		pool,
		b.cfg.MultiagentShuffleSecret,
		b.cfg.MultiagentBroadcastBranchingProbability,
	) {
		effectiveHandoff = b.cfg.MultiagentBroadcastBranchingHandoffProbability
	}
	go func() {
		b.beginBroadcast(channel)
		defer b.endBroadcast(channel)
		b.runMultiagentSession(ctx, channel, rawText, messageTS, participants, rounds, effectiveHandoff)
	}()
	return true
}

// dispatchBroadcastPresenceAck handles @everyone presence checks with a deterministic
// one-line acknowledgment from each participant.
func (b *Bot) dispatchBroadcastPresenceAck(ctx context.Context, channel, rawText string, messageTS string) bool {
	if !b.cfg.MultiagentConfigured() {
		return false
	}
	if !broadcastMultiagentTrigger(rawText) {
		return false
	}
	basePool := orderedBroadcastPool(b.cfg)
	pool := resolveBroadcastCandidatePool(rawText, b.cfg)
	if len(pool) == 0 {
		pool = basePool
	}
	participants := shuffleBroadcastParticipants(messageTS, pool, b.cfg.MultiagentShuffleSecret)
	if len(participants) < 1 {
		return false
	}
	go func() {
		b.beginBroadcast(channel)
		defer b.endBroadcast(channel)
		b.runBroadcastPresenceAckSession(ctx, channel, messageTS, participants)
	}()
	return true
}

// dispatchGeneralAutoReaction handles plain #general messages (no explicit bot mentions)
// by deterministically selecting one squad participant and adding a single thumbs-up reaction.
func (b *Bot) dispatchGeneralAutoReaction(ctx context.Context, channel, rawText string, ev *slackevents.MessageEvent) bool {
	if ev == nil || b.cfg == nil {
		emp := ""
		if b != nil && b.cfg != nil {
			emp = strings.TrimSpace(b.cfg.EmployeeID)
		}
		log.Printf("general_auto_reaction: skip reason=missing_event_or_config employee=%s", emp)
		return false
	}
	mentions := mentionedSquadKeys(rawText, b.cfg)
	if len(mentions) > 0 {
		log.Printf("general_auto_reaction: skip reason=explicit_squad_mention employee=%s mentions=%s", strings.TrimSpace(b.cfg.EmployeeID), strings.Join(mentions, ","))
		return false
	}
	if !generalAutoReactionEligible(b.cfg, channel, ev.User) {
		log.Printf("general_auto_reaction: skip reason=ineligible employee=%s channel=%s user=%s", strings.TrimSpace(b.cfg.EmployeeID), strings.TrimSpace(channel), strings.TrimSpace(ev.User))
		return false
	}
	if skip, reason := shouldSkipGeneralAutoReply(rawText); skip {
		log.Printf("general_auto_reaction: skip reason=%s employee=%s channel=%s anchor=%s", reason, strings.TrimSpace(b.cfg.EmployeeID), strings.TrimSpace(channel), strings.TrimSpace(ev.TimeStamp))
		return false
	}
	anchorTS := strings.TrimSpace(ev.TimeStamp)
	if anchorTS == "" {
		log.Printf("general_auto_reaction: skip reason=empty_anchor employee=%s", strings.TrimSpace(b.cfg.EmployeeID))
		return false
	}
	winner := selectSingleGeneralParticipant(anchorTS, b.cfg.MultiagentOrder, b.cfg.MultiagentShuffleSecret)
	if winner == "" {
		log.Printf("general_auto_reaction: skip reason=empty_winner employee=%s anchor=%s", strings.TrimSpace(b.cfg.EmployeeID), anchorTS)
		return false
	}
	selfKey := strings.ToLower(strings.TrimSpace(b.cfg.EmployeeID))
	log.Printf("general_auto_reaction: candidate employee=%s winner=%s anchor=%s", selfKey, winner, anchorTS)
	if selfKey == winner {
		claimant := "winner:" + selfKey
		status := b.tryGeneralAutoReplyClaim(ctx, channel, anchorTS, claimant)
		if !generalAutoReplyWinnerShouldPost(status) {
			log.Printf("general_auto_reaction: winner_claim_missed employee=%s anchor=%s claim_status=%s", selfKey, anchorTS, status)
			return false
		}
		if status == generalAutoReplyClaimBackendDown {
			log.Printf("general_auto_reaction: winner_fallback_without_claim employee=%s anchor=%s", selfKey, anchorTS)
		}
		if b.postGeneralAutoReactionWithResult(ctx, channel, anchorTS) {
			log.Printf("general_auto_reaction: reacted employee=%s path=winner anchor=%s claim_status=%s", selfKey, anchorTS, status)
			return true
		}
		log.Printf("general_auto_reaction: react_failed employee=%s path=winner anchor=%s claim_status=%s", selfKey, anchorTS, status)
		if status == generalAutoReplyClaimAcquired {
			b.releaseGeneralAutoReplyClaim(ctx, channel, anchorTS, claimant)
		}
		return false
	}
	if b.generalAutoReplyLock == nil {
		log.Printf("general_auto_reaction: skip reason=no_lock_for_failover employee=%s winner=%s anchor=%s", selfKey, winner, anchorTS)
		return false
	}
	delay := generalAutoReplyFailoverDelay(selfKey, b.cfg.MultiagentOrder)
	go b.tryGeneralAutoReplyFailover(ctx, channel, anchorTS, selfKey, winner, delay)
	log.Printf("general_auto_reaction: failover_scheduled employee=%s winner=%s delay=%s anchor=%s", selfKey, winner, delay, anchorTS)
	return false
}

func generalAutoReactionEligible(cfg *config.Config, channel, userID string) bool {
	if cfg == nil || !cfg.MultiagentConfigured() || !cfg.MultiagentGeneralAutoReactionEnabled {
		return false
	}
	generalChannel := strings.TrimSpace(cfg.SlackGeneralChannelID)
	allowedUser := strings.TrimSpace(cfg.ChatAllowedUserID)
	if generalChannel == "" || allowedUser == "" {
		return false
	}
	return strings.TrimSpace(channel) == generalChannel && strings.TrimSpace(userID) == allowedUser
}

func generalAutoReplyNoSquadMentions(rawText string, cfg *config.Config) bool {
	return len(mentionedSquadKeys(rawText, cfg)) == 0
}

func generalAutoReplyFailoverDelay(selfKey string, order []string) time.Duration {
	base := 4 * time.Second
	for i, key := range order {
		if strings.EqualFold(strings.TrimSpace(key), strings.TrimSpace(selfKey)) {
			return base + time.Duration(i+1)*time.Second
		}
	}
	return base + 5*time.Second
}

func (b *Bot) tryGeneralAutoReplyFailover(ctx context.Context, channel, anchorTS, selfKey, winner string, delay time.Duration) {
	if delay > 0 {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
	claimant := "failover:" + selfKey
	status := b.tryGeneralAutoReplyClaim(ctx, channel, anchorTS, claimant)
	if !generalAutoReplyFailoverShouldPost(status) {
		log.Printf("general_auto_reaction: failover_not_selected employee=%s winner=%s anchor=%s claim_status=%s", selfKey, winner, anchorTS, status)
		return
	}
	if b.postGeneralAutoReactionWithResult(ctx, channel, anchorTS) {
		log.Printf("general_auto_reaction: reacted employee=%s path=failover winner=%s anchor=%s claim_status=%s", selfKey, winner, anchorTS, status)
		return
	}
	log.Printf("general_auto_reaction: react_failed employee=%s path=failover winner=%s anchor=%s claim_status=%s", selfKey, winner, anchorTS, status)
	b.releaseGeneralAutoReplyClaim(ctx, channel, anchorTS, claimant)
}

func (b *Bot) tryGeneralAutoReplyClaim(ctx context.Context, channel, anchorTS, claimant string) generalAutoReplyClaimStatus {
	if b.generalAutoReplyLock == nil {
		if strings.HasPrefix(claimant, "winner:") {
			return generalAutoReplyClaimBackendDown
		}
		return generalAutoReplyClaimAlreadyClaimed
	}
	status, err := b.generalAutoReplyLock.TryClaim(ctx, channel, anchorTS, claimant, 90*time.Second)
	if err != nil {
		log.Printf("general_auto_reaction: claim_error claimant=%s channel=%s anchor=%s err=%v", claimant, channel, anchorTS, err)
	}
	return status
}

func generalAutoReplyWinnerShouldPost(status generalAutoReplyClaimStatus) bool {
	return status == generalAutoReplyClaimAcquired || status == generalAutoReplyClaimBackendDown
}

func generalAutoReplyFailoverShouldPost(status generalAutoReplyClaimStatus) bool {
	return status == generalAutoReplyClaimAcquired
}

func (b *Bot) releaseGeneralAutoReplyClaim(ctx context.Context, channel, anchorTS, claimant string) {
	if b.generalAutoReplyLock == nil {
		return
	}
	if err := b.generalAutoReplyLock.ReleaseIfOwned(ctx, channel, anchorTS, claimant); err != nil {
		log.Printf("general_auto_reaction: release_error claimant=%s channel=%s anchor=%s err=%v", claimant, channel, anchorTS, err)
	}
}

func (b *Bot) postGeneralAutoReactionWithResult(ctx context.Context, channel, messageTS string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.outbound != nil && !b.outbound.allow(now) {
		emp := strings.TrimSpace(b.cfg.EmployeeID)
		if emp == "" {
			emp = "default"
		}
		log.Printf("slack outbound rate limit: skipping general auto reaction (employee=%s channel=%s)", emp, channel)
		return false
	}

	ref := slack.ItemRef{
		Channel:   strings.TrimSpace(channel),
		Timestamp: strings.TrimSpace(messageTS),
	}
	if err := b.api.AddReactionContext(ctx, "+1", ref); err != nil {
		log.Printf("slack add reaction: %v", err)
		return false
	}
	if b.outbound != nil {
		b.outbound.record(time.Now())
	}
	return true
}

func (b *Bot) postLLMReply(ctx context.Context, channel, userText, messageTS, sourceUserID string) {
	_ = b.postLLMReplyWithResult(ctx, channel, userText, messageTS, sourceUserID)
}

func (b *Bot) postLLMReplyWithResult(ctx context.Context, channel, userText, messageTS, sourceUserID string) bool {
	if b.applyAvailabilityRouter(ctx, availabilityRouteEvent{
		Path:      "post_llm_channel",
		Channel:   channel,
		MessageTS: messageTS,
		RawText:   userText,
		Phase:     routerPhasePreLLM,
	}) {
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.outbound != nil && !b.outbound.allow(now) {
		emp := strings.TrimSpace(b.cfg.EmployeeID)
		if emp == "" {
			emp = "default"
		}
		log.Printf("slack outbound rate limit: skipping reply (employee=%s channel=%s)", emp, channel)
		return false
	}

	persona := b.persona.String()
	if persona == "" {
		persona = "You are a helpful assistant."
	}

	userPayload := strings.TrimSpace(userText)
	if b.useAlexHints() && b.cfg.LLMAlexHints {
		userPayload = router.WrapAlexUserMessage(userPayload)
	}
	if tc := b.channelHistoryContextBlock(ctx, channel, messageTS, sourceUserID); tc != "" {
		userPayload = tc + "\n\n" + userPayload
	}
	userPayload = b.prependRuntimeLessons(ctx, b.cfg.EmployeeID, userPayload)
	priorSelf := b.latestPriorEmployeeMessageInChannel(ctx, channel, messageTS)
	userPayload = prependRethinkCue(userPayload, userText, priorSelf)
	userPayload = prependHostilityCue(userPayload, userText)
	log.Printf("context_build: path=channel employee=%s channel=%s payload_runes=%d",
		strings.TrimSpace(b.cfg.EmployeeID), strings.TrimSpace(channel), utf8.RuneCountInString(userPayload))

	llmCtx, cancelLLM := b.withLLMTimeout(ctx)
	startLLM := time.Now()
	reply, err := b.llm.Reply(llmCtx, persona, slackReplySuffix, userPayload)
	cancelLLM()
	log.Printf("llm_call: path=channel employee=%s ms=%d err=%t",
		strings.TrimSpace(b.cfg.EmployeeID), time.Since(startLLM).Milliseconds(), err != nil)
	if err != nil {
		log.Printf("llm reply error (employee=%s): %v", strings.TrimSpace(b.cfg.EmployeeID), err)
		opts := []slack.MsgOption{slack.MsgOptionText(llmErrorUserMessage(err), false)}
		_, _, err = b.api.PostMessageContext(ctx, channel, opts...)
		if err != nil {
			log.Printf("slack post message: %v", err)
			return false
		}
		if b.outbound != nil {
			b.outbound.record(time.Now())
		}
		return true
	}
	if reply == "" {
		reply = "…"
	}
	startRepair := time.Now()
	reply = b.repairOutboundReply(ctx, persona, userPayload, reply)
	log.Printf("repair_call: path=channel employee=%s ms=%d",
		strings.TrimSpace(b.cfg.EmployeeID), time.Since(startRepair).Milliseconds())
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
	opts := []slack.MsgOption{slack.MsgOptionText(reply, false)}
	startPost := time.Now()
	_, _, err = b.api.PostMessageContext(ctx, channel, opts...)
	log.Printf("slack_post: path=channel employee=%s ms=%d err=%t",
		strings.TrimSpace(b.cfg.EmployeeID), time.Since(startPost).Milliseconds(), err != nil)
	if err != nil {
		log.Printf("slack post message: %v", err)
		return false
	}
	if b.outbound != nil {
		b.outbound.record(time.Now())
	}
	b.recordRuntimeLesson(ctx, "post_llm_channel", channel, "", messageTS, userText, reply)
	return true
}

func (b *Bot) useAlexHints() bool {
	id := strings.ToLower(strings.TrimSpace(b.cfg.EmployeeID))
	return id == "" || id == "alex"
}

func (b *Bot) withLLMTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if b == nil || b.cfg == nil || b.cfg.LLMReplyTimeoutSec <= 0 {
		return context.WithTimeout(ctx, 35*time.Second)
	}
	return context.WithTimeout(ctx, time.Duration(b.cfg.LLMReplyTimeoutSec)*time.Second)
}

func (b *Bot) latestPriorEmployeeMessageInChannel(ctx context.Context, channelID, currentMsgTS string) string {
	if strings.TrimSpace(channelID) == "" || strings.TrimSpace(currentMsgTS) == "" {
		return ""
	}
	limit := b.cfg.LLMThreadMaxMessages
	if limit < 10 {
		limit = 10
	}
	params := &slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    currentMsgTS,
		Inclusive: false,
		Limit:     limit,
	}
	resp, err := b.api.GetConversationHistoryContext(ctx, params)
	if err != nil {
		return ""
	}
	for _, m := range resp.Messages {
		if strings.TrimSpace(m.Text) == "" {
			continue
		}
		if m.User == b.botUserID {
			return strings.TrimSpace(m.Text)
		}
		if k, ok := squadKeyForSlackUser(b.cfg, m.User); ok && strings.EqualFold(k, b.cfg.EmployeeID) {
			return strings.TrimSpace(m.Text)
		}
	}
	return ""
}

func (b *Bot) beginBroadcast(channel string) {
	b.routeMu.Lock()
	defer b.routeMu.Unlock()
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return
	}
	b.activeBroadcastByChannel[channel]++
}

func (b *Bot) endBroadcast(channel string) {
	b.routeMu.Lock()
	defer b.routeMu.Unlock()
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return
	}
	n := b.activeBroadcastByChannel[channel]
	if n <= 1 {
		delete(b.activeBroadcastByChannel, channel)
		return
	}
	b.activeBroadcastByChannel[channel] = n - 1
}

func (b *Bot) isBroadcastActive(channel string) bool {
	b.routeMu.Lock()
	defer b.routeMu.Unlock()
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return false
	}
	return b.activeBroadcastByChannel[channel] > 0
}

// channelHistoryContextBlock loads recent messages on the channel timeline before the current
// message (conversations.history). No threads/DMs—one open channel, linear context.
func (b *Bot) channelHistoryContextBlock(ctx context.Context, channelID, currentMsgTS, triggerUserID string) string {
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
	msgs = clipMessagesToGrantBoundary(msgs, b.cfg.ChatAllowedUserID, shouldEnforceGrantBoundary(triggerUserID, b.cfg.ChatAllowedUserID))
	if len(msgs) == 0 {
		return ""
	}
	type historyLine struct {
		role string
		text string
	}
	var entries []historyLine
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
		isBotLike := m.BotID != "" || m.User == b.botUserID
		if sk, ok := squadKeyForSlackUser(b.cfg, m.User); ok {
			role = sk
			isBotLike = true
		} else if isBotLike {
			role = "assistant"
		}
		if isBotLike && isOperationalModelErrorLine(text) {
			continue
		}
		entries = append(entries, historyLine{role: role, text: text})
	}
	if len(entries) == 0 {
		return ""
	}
	lines := make([]string, 0, len(entries))
	for i, e := range entries {
		indexFromLatest := len(entries) - 1 - i
		lines = append(lines, formatWeightedContext(e.role, e.text, indexFromLatest, b.cfg.LLMContextWeightDecay, b.cfg.LLMContextWeightWindow))
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
			if isOperationalModelErrorLine(t) {
				continue
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

// isOperationalModelErrorLine detects short operator-facing fallback strings so they
// are not recycled into future prompts via channel/thread history context.
func isOperationalModelErrorLine(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return false
	}
	if strings.Contains(t, "model issue; ross needs to check logs") {
		return true
	}
	if strings.Contains(t, "ross needs to check logs") && strings.Contains(t, "model") {
		return true
	}
	if strings.HasPrefix(t, "the model provider returned http ") {
		return true
	}
	if strings.HasPrefix(t, "the model rejected this request format (400)") {
		return true
	}
	if strings.HasPrefix(t, "i hit a model error") ||
		strings.HasPrefix(t, "i hit a model timeout") ||
		strings.HasPrefix(t, "the model provider is temporarily unavailable") ||
		strings.HasPrefix(t, "the model provider is temporarily overloaded") ||
		strings.Contains(t, "try me again") ||
		strings.Contains(t, "send that one more time") ||
		strings.Contains(t, "give me one more ping") {
		return true
	}
	return false
}
