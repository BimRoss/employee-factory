package slackbot

import (
	"context"
	"testing"
	"time"

	"github.com/bimross/employee-factory/internal/config"
)

func TestDecideAdvancedTaskRouting_EnforceSeedsThread(t *testing.T) {
	b := &Bot{
		cfg: &config.Config{
			AdvancedToolingThreadEnforcement:    "enforce",
			AdvancedToolingSeedThreadOnTopLevel: true,
			AdvancedToolingThreadTaskTTLSec:     1200,
		},
		advancedTaskByKey: map[string]advancedTaskSession{},
	}
	decision := b.decideAdvancedTaskRouting(context.Background(), "C1", "U1", "1770000000.000100", "", advancedTaskJoanneEmail)
	if !decision.ConsumeEvent {
		t.Fatalf("expected top-level kickoff to be consumed")
	}
	if decision.AllowExecution {
		t.Fatalf("expected top-level kickoff to defer execution")
	}
	session, ok := b.getAdvancedTaskSession("C1", "U1", advancedTaskJoanneEmail, time.Now().UTC())
	if !ok {
		t.Fatalf("expected session to be created")
	}
	if session.ThreadTS != "1770000000.000100" {
		t.Fatalf("expected seeded thread ts to match message ts, got %q", session.ThreadTS)
	}
}

func TestDecideAdvancedTaskRouting_EnforceRejectsOffThreadFollowup(t *testing.T) {
	now := time.Now().UTC()
	b := &Bot{
		cfg: &config.Config{
			AdvancedToolingThreadEnforcement: "enforce",
			AdvancedToolingThreadTaskTTLSec:  1200,
		},
		advancedTaskByKey: map[string]advancedTaskSession{
			advancedTaskSessionKey("C1", "U1", advancedTaskRossOps): {
				Channel:       "C1",
				RequestUserID: "U1",
				Task:          advancedTaskRossOps,
				ThreadTS:      "1760000000.000001",
				CreatedAt:     now.Add(-1 * time.Minute),
				UpdatedAt:     now.Add(-1 * time.Minute),
				ExpiresAt:     now.Add(20 * time.Minute),
			},
		},
	}
	decision := b.decideAdvancedTaskRouting(context.Background(), "C1", "U1", "1770000000.000200", "", advancedTaskRossOps)
	if !decision.ConsumeEvent {
		t.Fatalf("expected off-thread follow-up to be consumed")
	}
	if decision.AllowExecution {
		t.Fatalf("expected off-thread follow-up execution to be blocked")
	}
}

func TestDecideAdvancedTaskRouting_InThreadExecutes(t *testing.T) {
	b := &Bot{
		cfg: &config.Config{
			AdvancedToolingThreadEnforcement: "enforce",
			AdvancedToolingThreadTaskTTLSec:  1200,
		},
		advancedTaskByKey: map[string]advancedTaskSession{},
	}
	decision := b.decideAdvancedTaskRouting(context.Background(), "C1", "U1", "1770000000.000300", "1770000000.000123", advancedTaskJoanneDocs)
	if decision.ConsumeEvent {
		t.Fatalf("did not expect in-thread execution to be consumed")
	}
	if !decision.AllowExecution {
		t.Fatalf("expected in-thread execution")
	}
	if decision.ExecutionTS != "1770000000.000123" {
		t.Fatalf("execution ts mismatch: %q", decision.ExecutionTS)
	}
}

func TestDecideAdvancedTaskRouting_LogOnlyKeepsExecution(t *testing.T) {
	now := time.Now().UTC()
	b := &Bot{
		cfg: &config.Config{
			AdvancedToolingThreadEnforcement: "log_only",
			AdvancedToolingThreadTaskTTLSec:  1200,
		},
		advancedTaskByKey: map[string]advancedTaskSession{
			advancedTaskSessionKey("C1", "U1", advancedTaskJoanneEmail): {
				Channel:       "C1",
				RequestUserID: "U1",
				Task:          advancedTaskJoanneEmail,
				ThreadTS:      "1760000000.000001",
				CreatedAt:     now.Add(-1 * time.Minute),
				UpdatedAt:     now.Add(-1 * time.Minute),
				ExpiresAt:     now.Add(20 * time.Minute),
			},
		},
	}
	decision := b.decideAdvancedTaskRouting(context.Background(), "C1", "U1", "1770000000.000400", "", advancedTaskJoanneEmail)
	if decision.ConsumeEvent {
		t.Fatalf("log_only should not consume off-thread follow-up")
	}
	if !decision.AllowExecution {
		t.Fatalf("log_only should preserve execution")
	}
}
