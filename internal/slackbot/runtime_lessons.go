package slackbot

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/bimross/employee-factory/internal/lessons"
)

func (b *Bot) prependRuntimeLessons(ctx context.Context, employee, payload string) string {
	if b == nil || b.runtimeLessons == nil {
		return payload
	}
	prefix, count, err := b.runtimeLessons.PromptPrefix(ctx, employee)
	if err != nil {
		log.Printf("runtime_lessons: inject_error employee=%s err=%v", strings.TrimSpace(employee), err)
		return payload
	}
	if strings.TrimSpace(prefix) == "" {
		return payload
	}
	log.Printf("runtime_lessons: injected employee=%s count=%d", strings.TrimSpace(employee), count)
	return prefix + "\n\n" + payload
}

func (b *Bot) recordRuntimeLesson(ctx context.Context, path, channel, threadTS, messageTS, sourceText, finalReply string) {
	if b == nil || b.runtimeLessons == nil || b.cfg == nil {
		return
	}
	employee := strings.TrimSpace(strings.ToLower(b.cfg.EmployeeID))
	if employee == "" {
		employee = "default"
	}
	decision, err := b.runtimeLessons.Capture(ctx, lessons.Event{
		Employee:       employee,
		Path:           strings.TrimSpace(path),
		Channel:        strings.TrimSpace(channel),
		ThreadTS:       strings.TrimSpace(threadTS),
		MessageTS:      strings.TrimSpace(messageTS),
		SourceUserText: strings.TrimSpace(sourceText),
		FinalReply:     strings.TrimSpace(finalReply),
		Timestamp:      time.Now().UTC(),
	})
	if err != nil {
		log.Printf("runtime_lessons: capture_error employee=%s path=%s err=%v", employee, path, err)
		return
	}
	if decision.Applied {
		log.Printf("runtime_lessons: promoted employee=%s anchor=%s confidence=%.2f path=%s", employee, decision.AnchorKey, decision.Confidence, path)
		return
	}
	if decision.SkipReason != "" {
		log.Printf("runtime_lessons: skipped employee=%s path=%s reason=%s confidence=%.2f", employee, path, decision.SkipReason, decision.Confidence)
	}
}
