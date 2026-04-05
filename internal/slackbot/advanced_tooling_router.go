package slackbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

type advancedTaskType string

const (
	advancedTaskRossOps     advancedTaskType = "ross_ops"
	advancedTaskJoanneEmail advancedTaskType = "joanne_send_email"
	advancedTaskJoanneDocs  advancedTaskType = "joanne_create_doc"
)

type advancedThreadMode string

const (
	advancedThreadModeOff     advancedThreadMode = "off"
	advancedThreadModeLogOnly advancedThreadMode = "log_only"
	advancedThreadModeEnforce advancedThreadMode = "enforce"
)

type advancedTaskSession struct {
	Channel       string
	RequestUserID string
	Task          advancedTaskType
	ThreadTS      string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ExpiresAt     time.Time
}

type advancedTaskDecision struct {
	AllowExecution bool
	ConsumeEvent   bool
	ExecutionTS    string
}

func (b *Bot) decideAdvancedTaskRouting(ctx context.Context, channel, requestUserID, messageTS, threadTS string, task advancedTaskType) advancedTaskDecision {
	if b == nil || b.cfg == nil {
		return advancedTaskDecision{AllowExecution: true, ExecutionTS: strings.TrimSpace(threadTS)}
	}
	mode := normalizeAdvancedThreadMode(b.cfg.AdvancedToolingThreadEnforcement)
	if mode == advancedThreadModeOff {
		return advancedTaskDecision{AllowExecution: true, ExecutionTS: strings.TrimSpace(threadTS)}
	}

	channel = strings.TrimSpace(channel)
	requestUserID = strings.TrimSpace(requestUserID)
	messageTS = strings.TrimSpace(messageTS)
	threadTS = strings.TrimSpace(threadTS)

	now := time.Now().UTC()
	active, ok := b.getAdvancedTaskSession(channel, requestUserID, task, now)
	activeTS := ""
	if ok {
		activeTS = strings.TrimSpace(active.ThreadTS)
	}

	if threadTS == "" {
		// Start a dedicated control thread off the top-level kickoff message.
		if activeTS == "" && b.cfg.AdvancedToolingSeedThreadOnTopLevel {
			seedTS := messageTS
			if seedTS == "" {
				seedTS = fmt.Sprintf("%d", now.Unix())
			}
			b.setAdvancedTaskSession(advancedTaskSession{
				Channel:       channel,
				RequestUserID: requestUserID,
				Task:          task,
				ThreadTS:      seedTS,
				CreatedAt:     now,
				UpdatedAt:     now,
				ExpiresAt:     now.Add(time.Duration(b.cfg.AdvancedToolingThreadTaskTTLSec) * time.Second),
			})
			b.postAdvancedTaskKickoff(ctx, channel, seedTS, task)
			b.logAdvancedTaskDecision(task, mode, false, true, "seed_thread", channel, requestUserID, messageTS, threadTS, seedTS)
			return advancedTaskDecision{ConsumeEvent: true}
		}
		if activeTS != "" {
			b.logAdvancedTaskDecision(task, mode, false, true, "off_thread_existing_session", channel, requestUserID, messageTS, threadTS, activeTS)
			if mode == advancedThreadModeEnforce {
				b.postAdvancedTaskRedirect(ctx, channel, activeTS, task)
				return advancedTaskDecision{ConsumeEvent: true}
			}
			return advancedTaskDecision{AllowExecution: true}
		}
		b.logAdvancedTaskDecision(task, mode, false, false, "top_level_no_session", channel, requestUserID, messageTS, threadTS, "")
		return advancedTaskDecision{AllowExecution: true}
	}

	if activeTS != "" && activeTS != threadTS {
		b.logAdvancedTaskDecision(task, mode, false, true, "thread_mismatch", channel, requestUserID, messageTS, threadTS, activeTS)
		if mode == advancedThreadModeEnforce {
			b.postAdvancedTaskRedirect(ctx, channel, activeTS, task)
			return advancedTaskDecision{ConsumeEvent: true}
		}
		return advancedTaskDecision{AllowExecution: true, ExecutionTS: threadTS}
	}

	if activeTS == "" {
		b.setAdvancedTaskSession(advancedTaskSession{
			Channel:       channel,
			RequestUserID: requestUserID,
			Task:          task,
			ThreadTS:      threadTS,
			CreatedAt:     now,
			UpdatedAt:     now,
			ExpiresAt:     now.Add(time.Duration(b.cfg.AdvancedToolingThreadTaskTTLSec) * time.Second),
		})
	} else {
		b.touchAdvancedTaskSession(channel, requestUserID, task, now)
	}
	b.logAdvancedTaskDecision(task, mode, true, false, "thread_execution", channel, requestUserID, messageTS, threadTS, threadTS)
	return advancedTaskDecision{AllowExecution: true, ExecutionTS: threadTS}
}

func normalizeAdvancedThreadMode(raw string) advancedThreadMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(advancedThreadModeEnforce):
		return advancedThreadModeEnforce
	case string(advancedThreadModeLogOnly):
		return advancedThreadModeLogOnly
	default:
		return advancedThreadModeOff
	}
}

func (b *Bot) getAdvancedTaskSession(channel, requestUserID string, task advancedTaskType, now time.Time) (advancedTaskSession, bool) {
	if b == nil {
		return advancedTaskSession{}, false
	}
	key := advancedTaskSessionKey(channel, requestUserID, task)
	b.advancedTaskMu.Lock()
	defer b.advancedTaskMu.Unlock()
	if b.advancedTaskByKey == nil {
		b.advancedTaskByKey = map[string]advancedTaskSession{}
	}
	session, ok := b.advancedTaskByKey[key]
	if !ok {
		return advancedTaskSession{}, false
	}
	if !session.ExpiresAt.IsZero() && session.ExpiresAt.Before(now) {
		delete(b.advancedTaskByKey, key)
		return advancedTaskSession{}, false
	}
	return session, true
}

func (b *Bot) setAdvancedTaskSession(session advancedTaskSession) {
	if b == nil {
		return
	}
	key := advancedTaskSessionKey(session.Channel, session.RequestUserID, session.Task)
	b.advancedTaskMu.Lock()
	defer b.advancedTaskMu.Unlock()
	if b.advancedTaskByKey == nil {
		b.advancedTaskByKey = map[string]advancedTaskSession{}
	}
	b.advancedTaskByKey[key] = session
}

func (b *Bot) touchAdvancedTaskSession(channel, requestUserID string, task advancedTaskType, now time.Time) {
	if b == nil {
		return
	}
	key := advancedTaskSessionKey(channel, requestUserID, task)
	b.advancedTaskMu.Lock()
	defer b.advancedTaskMu.Unlock()
	session, ok := b.advancedTaskByKey[key]
	if !ok {
		return
	}
	session.UpdatedAt = now
	if b.cfg != nil && b.cfg.AdvancedToolingThreadTaskTTLSec > 0 {
		session.ExpiresAt = now.Add(time.Duration(b.cfg.AdvancedToolingThreadTaskTTLSec) * time.Second)
	}
	b.advancedTaskByKey[key] = session
}

func advancedTaskSessionKey(channel, requestUserID string, task advancedTaskType) string {
	return strings.TrimSpace(channel) + "|" + strings.TrimSpace(requestUserID) + "|" + string(task)
}

func (b *Bot) postAdvancedTaskKickoff(parent context.Context, channel, threadTS string, task advancedTaskType) {
	if b == nil || b.api == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	text := "I started a task thread for this request. Reply here with details and confirmations so this task stays cleanly scoped."
	switch task {
	case advancedTaskJoanneEmail:
		text = "I started an email-task thread. Reply here with recipient/body details and `confirm send` when ready."
	case advancedTaskJoanneDocs:
		text = "I started a doc-task thread. Reply here with `title:` and `instruction:`/`body:` so I can generate it."
	case advancedTaskRossOps:
		text = "I started an ops-task thread. Reply here with namespace/target details and I will fetch exactly what you need."
	}
	if err := b.postSlackResponse(ctx, channel, slackResponse{
		Text:     text,
		ThreadTS: strings.TrimSpace(threadTS),
	}); err != nil {
		log.Printf("advanced_task: kickoff_post_failed task=%s channel=%s thread_ts=%s err=%v", task, strings.TrimSpace(channel), strings.TrimSpace(threadTS), err)
	}
}

func (b *Bot) postAdvancedTaskRedirect(parent context.Context, channel, threadTS string, task advancedTaskType) {
	if b == nil || b.api == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	text := "Please continue this advanced task in its existing thread so context stays clean."
	switch task {
	case advancedTaskJoanneEmail:
		text = "Please continue this email task in its existing thread so details and confirmation stay in one place."
	case advancedTaskJoanneDocs:
		text = "Please continue this doc task in its existing thread so drafting context stays in one place."
	case advancedTaskRossOps:
		text = "Please continue this ops task in its existing thread so request context stays clean."
	}
	if strings.TrimSpace(threadTS) != "" {
		text = text + " (thread ts: `" + strings.TrimSpace(threadTS) + "`)"
	}
	if err := b.postSlackResponse(ctx, channel, slackResponse{Text: text}); err != nil {
		log.Printf("advanced_task: redirect_post_failed task=%s channel=%s thread_ts=%s err=%v", task, strings.TrimSpace(channel), strings.TrimSpace(threadTS), err)
	}
}

func (b *Bot) logAdvancedTaskDecision(task advancedTaskType, mode advancedThreadMode, execute, consumed bool, reason, channel, requestUserID, messageTS, incomingThreadTS, activeThreadTS string) {
	log.Printf(
		"advanced_task_router: task=%s mode=%s execute=%t consumed=%t reason=%s channel=%s requester=%s message_ts=%s incoming_thread_ts=%s active_thread_ts=%s",
		task,
		mode,
		execute,
		consumed,
		strings.TrimSpace(reason),
		strings.TrimSpace(channel),
		strings.TrimSpace(requestUserID),
		strings.TrimSpace(messageTS),
		strings.TrimSpace(incomingThreadTS),
		strings.TrimSpace(activeThreadTS),
	)
}
