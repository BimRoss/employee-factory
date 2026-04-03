package slackbot

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/slack-go/slack"
)

func testCfgSquad() *config.Config {
	return &config.Config{
		MultiagentEnabled: true,
		MultiagentBotUserIDs: map[string]string{
			"ross": "UROSS001",
			"tim":  "UTIM002",
			"alex": "UALEX003",
		},
		MultiagentOrder: []string{"ross", "tim", "alex"},
	}
}

func TestParseMentionedUserIDs(t *testing.T) {
	text := "Hi <@UALEX003> and <@UTIM002> <@UROSS001> repeat <@UALEX003>"
	got := parseMentionedUserIDs(text)
	want := []string{"UALEX003", "UTIM002", "UROSS001"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestMentionedSquadKeys_order(t *testing.T) {
	cfg := testCfgSquad()
	raw := "<@UTIM002> <@UROSS001> what next?"
	got := mentionedSquadKeys(raw, cfg)
	want := []string{"ross", "tim"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestBuildSlots_rounds(t *testing.T) {
	cfg := testCfgSquad()
	participants := []string{"ross", "tim"}
	slots := buildSlots(participants, 2, cfg.MultiagentBotUserIDs)
	want := []string{"UROSS001", "UTIM002", "UROSS001", "UTIM002"}
	if len(slots) != len(want) {
		t.Fatalf("got %v want %v", slots, want)
	}
	for i := range want {
		if slots[i] != want[i] {
			t.Fatalf("idx %d: got %q want %q", i, slots[i], want[i])
		}
	}
}

func TestCountSquadMessagesInRun(t *testing.T) {
	squad := map[string]bool{"U1": true, "U2": true}
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "UH", Text: "hi"}},
		{Msg: slack.Msg{User: "U1", Text: "a"}},
		{Msg: slack.Msg{User: "U2", Text: "b"}},
		{Msg: slack.Msg{User: "U1", Text: "c"}},
	}
	if n := countSquadMessagesInRun(msgs, squad, 3); n != 3 {
		t.Fatalf("want 3 squad in run after human, got %d", n)
	}
	if n := countSquadMessagesInRun(msgs, squad, 2); n != 2 {
		t.Fatalf("want 2 through idx 2, got %d", n)
	}
	allSquad := []slack.Message{
		{Msg: slack.Msg{User: "U1"}},
		{Msg: slack.Msg{User: "U2"}},
	}
	if n := countSquadMessagesInRun(allSquad, squad, 1); n != 2 {
		t.Fatalf("no human in window: want 2, got %d", n)
	}
}

func TestShuffleBroadcastParticipants_deterministic(t *testing.T) {
	order := []string{"ross", "tim", "alex", "garth"}
	a := shuffleBroadcastParticipants("1743491234.567890", order, "")
	b := shuffleBroadcastParticipants("1743491234.567890", order, "")
	if len(a) != len(order) {
		t.Fatalf("len %d", len(a))
	}
	seen := map[string]bool{}
	for _, k := range a {
		seen[k] = true
	}
	for _, k := range order {
		if !seen[k] {
			t.Fatalf("missing key %q in %v", k, a)
		}
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("same anchor should match: %v vs %v", a, b)
		}
	}
}

func TestShuffleBroadcastParticipants_anchorChangesPermutation(t *testing.T) {
	order := []string{"ross", "tim", "alex", "garth"}
	a := shuffleBroadcastParticipants("1743491234.567890", order, "")
	b := shuffleBroadcastParticipants("1743491234.567891", order, "")
	if slices.Equal(a, b) {
		t.Fatalf("expected different permutations for different anchors, got %v", a)
	}
}

func TestShuffleBroadcastParticipants_secretChangesPermutation(t *testing.T) {
	order := []string{"ross", "tim", "alex", "garth"}
	a := shuffleBroadcastParticipants("1743491234.567890", order, "")
	b := shuffleBroadcastParticipants("1743491234.567890", order, "salt")
	if len(a) != len(b) {
		t.Fatal("length mismatch")
	}
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("expected secret to change permutation")
	}
}

func TestBroadcastMultiagentTrigger(t *testing.T) {
	if !broadcastMultiagentTrigger("<!everyone> hi") {
		t.Fatal("everyone token")
	}
	if !broadcastMultiagentTrigger("x <!channel|@channel> y") {
		t.Fatal("channel token should trigger broadcast")
	}
	if !broadcastMultiagentTrigger("x @channel y") {
		t.Fatal("plain @channel should trigger broadcast")
	}
	if !broadcastMultiagentTrigger("x @everyone y") {
		t.Fatal("plain @everyone should trigger broadcast")
	}
	if broadcastMultiagentTrigger("Hey everyone") {
		t.Fatal("plain everyone text does not start broadcast (only coordinator + Slack tokens)")
	}
}

func TestShouldUseBroadcastBranchMode_deterministic(t *testing.T) {
	order := []string{"ross", "tim", "alex", "garth"}
	a := shouldUseBroadcastBranchMode("1743491234.567890", order, "secret", 0.5)
	b := shouldUseBroadcastBranchMode("1743491234.567890", order, "secret", 0.5)
	if a != b {
		t.Fatalf("expected deterministic result for same anchor/secret/order: %v vs %v", a, b)
	}
}

func TestShouldUseBroadcastBranchMode_bounds(t *testing.T) {
	order := []string{"ross", "tim"}
	if shouldUseBroadcastBranchMode("1743491234.567890", order, "secret", 0.0) {
		t.Fatal("probability 0 should never branch")
	}
	if !shouldUseBroadcastBranchMode("1743491234.567890", order, "secret", 1.0) {
		t.Fatal("probability 1 should always branch")
	}
}

func TestShouldTriggerGeneralAutoReply_deterministic(t *testing.T) {
	order := []string{"ross", "tim", "alex", "garth"}
	a := shouldTriggerGeneralAutoReply("1743491234.567890", order, "secret", 0.4)
	b := shouldTriggerGeneralAutoReply("1743491234.567890", order, "secret", 0.4)
	if a != b {
		t.Fatalf("expected deterministic result for same anchor/secret/order: %v vs %v", a, b)
	}
}

func TestShouldTriggerGeneralAutoReply_bounds(t *testing.T) {
	order := []string{"ross", "tim"}
	if shouldTriggerGeneralAutoReply("1743491234.567890", order, "secret", 0.0) {
		t.Fatal("probability 0 should never trigger")
	}
	if !shouldTriggerGeneralAutoReply("1743491234.567890", order, "secret", 1.0) {
		t.Fatal("probability 1 should always trigger")
	}
}

func TestSelectSingleGeneralParticipant_deterministic(t *testing.T) {
	order := []string{"ross", "tim", "alex", "garth"}
	a := selectSingleGeneralParticipant("1743491234.567890", order, "secret")
	b := selectSingleGeneralParticipant("1743491234.567890", order, "secret")
	if a != b {
		t.Fatalf("same anchor should pick same winner: %q vs %q", a, b)
	}
	if a == "" {
		t.Fatal("expected non-empty winner")
	}
}

func TestSelectSingleGeneralParticipant_changesWithInputs(t *testing.T) {
	order := []string{"ross", "tim", "alex", "garth"}
	a := selectSingleGeneralParticipant("1743491234.567890", order, "")
	b := selectSingleGeneralParticipant("1743491234.567891", order, "")
	c := selectSingleGeneralParticipant("1743491234.567890", order, "salt")
	if a == b && a == c {
		t.Fatalf("expected anchor or secret change to alter winner at least once: a=%q b=%q c=%q", a, b, c)
	}
}

func TestMixedEveryoneAndSingleMention_parsing(t *testing.T) {
	cfg := testCfgSquad()
	raw := "<!everyone> quick take from <@UALEX003>"
	if !broadcastMultiagentTrigger(raw) {
		t.Fatal("expected broadcast trigger to remain true for <!everyone>")
	}
	got := mentionedSquadKeys(raw, cfg)
	if len(got) != 1 || got[0] != "alex" {
		t.Fatalf("expected single targeted mention preserved, got %v", got)
	}
	if !shouldRouteAsBroadcast(raw, cfg) {
		t.Fatal("expected routing precedence to prefer broadcast in mixed summon messages")
	}
}

func TestStripSquadUserMentions(t *testing.T) {
	cfg := testCfgSquad()
	squad := squadUserIDSet(cfg)
	text := "<@UROSS001> <@UTIM002> hello <!channel>"
	out := stripSquadUserMentions(text, squad)
	if out != "hello" {
		t.Fatalf("got %q want %q", out, "hello")
	}
}

func TestPrefixMatchesSquadSlots(t *testing.T) {
	slots := []string{"U1", "U2", "U3"}
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "U1"}},
		{Msg: slack.Msg{User: "U2"}},
	}
	if !prefixMatchesSquadSlots(msgs, slots, 2) {
		t.Fatal("expected match for k=2")
	}
	if prefixMatchesSquadSlots(msgs, slots, 3) {
		t.Fatal("should not match k=3")
	}
	if !prefixMatchesSquadSlots(nil, slots, 0) {
		t.Fatal("k=0 empty ok")
	}
}

func TestFormatPriorSquadTurns_weightedAndBounded(t *testing.T) {
	slots := []string{"U1", "U2", "U3"}
	idToKey := map[string]string{"U1": "ross", "U2": "tim", "U3": "alex"}
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "U1", Text: "first"}},
		{Msg: slack.Msg{User: "U2", Text: "second"}},
		{Msg: slack.Msg{User: "U3", Text: "third"}},
	}
	out := formatPriorSquadTurns(slots, 3, msgs, idToKey, 120, 0.5, 3)
	if !strings.Contains(out, "[w=1.00]") || !strings.Contains(out, "[w=0.50]") {
		t.Fatalf("expected weighted markers in prior context, got %q", out)
	}
	if len([]rune(out)) > 140 { // account for truncation prefix when capped.
		t.Fatalf("expected bounded prior context length, got %d runes", len([]rune(out)))
	}
}

func TestSampleHandoffProbability_withinBounds(t *testing.T) {
	const (
		minP = 0.25
		maxP = 0.75
	)
	for i := 0; i < 200; i++ {
		p := sampleHandoffProbability(0.5, minP, maxP)
		if p < minP || p > maxP {
			t.Fatalf("sample out of bounds: %.4f", p)
		}
	}
}

func TestSampleHandoffProbability_zeroBaseDisables(t *testing.T) {
	if p := sampleHandoffProbability(0, 0.25, 0.75); p != 0 {
		t.Fatalf("expected zero probability when base is zero, got %.4f", p)
	}
}

func TestRecencyWeight_defaultDecay(t *testing.T) {
	if got := recencyWeight(0, 0.5, 3); math.Abs(got-1.0) > 0.0001 {
		t.Fatalf("latest should weight 1.0, got %.4f", got)
	}
	if got := recencyWeight(1, 0.5, 3); math.Abs(got-0.5) > 0.0001 {
		t.Fatalf("second latest should weight 0.5, got %.4f", got)
	}
	if got := recencyWeight(2, 0.5, 3); math.Abs(got-0.25) > 0.0001 {
		t.Fatalf("third latest should weight 0.25, got %.4f", got)
	}
	if got := recencyWeight(9, 0.5, 3); math.Abs(got-0.25) > 0.0001 {
		t.Fatalf("weights should cap at window floor, got %.4f", got)
	}
}

func TestEvaluateMultiagentSlotState_exactReady(t *testing.T) {
	slots := []string{"U1", "U2", "U3"}
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "U1"}},
		{Msg: slack.Msg{User: "U2"}},
	}
	mode, reason, ok := evaluateMultiagentSlotState(2, msgs, slots, 1500*time.Millisecond, 10*time.Second, true)
	if !ok {
		t.Fatal("expected slot to be ready")
	}
	if mode != multiagentWaitModeExact {
		t.Fatalf("mode=%q want %q", mode, multiagentWaitModeExact)
	}
	if reason != "exact_prefix_ready" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestEvaluateMultiagentSlotState_softTimeoutDegrades(t *testing.T) {
	slots := []string{"U1", "U2", "U3", "U4"}
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "U1"}},
	}
	mode, reason, ok := evaluateMultiagentSlotState(2, msgs, slots, 12*time.Second, 10*time.Second, true)
	if !ok {
		t.Fatal("expected degraded start after soft timeout")
	}
	if mode != multiagentWaitModeDegraded {
		t.Fatalf("mode=%q want %q", mode, multiagentWaitModeDegraded)
	}
	if reason != "soft_timeout" {
		t.Fatalf("reason=%q", reason)
	}
}

func TestEvaluateMultiagentSlotState_noDegradeBeforeSoftTimeout(t *testing.T) {
	slots := []string{"U1", "U2", "U3"}
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "U1"}},
	}
	mode, reason, ok := evaluateMultiagentSlotState(2, msgs, slots, 5*time.Second, 10*time.Second, true)
	if ok {
		t.Fatalf("unexpected readiness mode=%q reason=%q", mode, reason)
	}
}

func TestExpectedMissingAgentForSlot(t *testing.T) {
	slots := []string{"U1", "U2", "U3", "U4"}
	if got := expectedMissingAgentForSlot(slots, 3, 2); got != "U3" {
		t.Fatalf("got %q want U3", got)
	}
	if got := expectedMissingAgentForSlot(slots, 3, -1); got != "U3" {
		t.Fatalf("got %q want U3", got)
	}
	if got := expectedMissingAgentForSlot(slots, 0, 0); got != "" {
		t.Fatalf("slot 0 should not report missing agent, got %q", got)
	}
}

func testCfgSquadWithJoanne() *config.Config {
	return &config.Config{
		MultiagentEnabled: true,
		MultiagentBotUserIDs: map[string]string{
			"ross":   "UROSS001",
			"tim":    "UTIM002",
			"alex":   "UALEX003",
			"joanne": "UJOANNE004",
			"garth":  "UGARTH005",
		},
		MultiagentOrder: []string{"ross", "tim", "alex", "joanne", "garth"},
	}
}

func TestResolveBroadcastCandidatePool_excludesOnboardingTarget(t *testing.T) {
	cfg := testCfgSquadWithJoanne()
	raw := "@everyone welcome joanne to the team"
	got := resolveBroadcastCandidatePool(raw, cfg)
	if slices.Contains(got, "joanne") {
		t.Fatalf("expected joanne excluded from onboarding welcome pool, got %v", got)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 participants after exclusion, got %d (%v)", len(got), got)
	}
}

func TestResolveBroadcastCandidatePool_keepsPoolWhenNoTarget(t *testing.T) {
	cfg := testCfgSquadWithJoanne()
	raw := "@everyone welcome team"
	got := resolveBroadcastCandidatePool(raw, cfg)
	if len(got) != len(cfg.MultiagentOrder) {
		t.Fatalf("expected full pool for generic welcome, got %v", got)
	}
	for _, key := range cfg.MultiagentOrder {
		if !slices.Contains(got, key) {
			t.Fatalf("expected %q in pool, got %v", key, got)
		}
	}
}

func TestResolveBroadcastCandidatePool_explicitMentionOverridesExclusion(t *testing.T) {
	cfg := testCfgSquadWithJoanne()
	raw := "<!everyone> welcome <@UJOANNE004>"
	got := resolveBroadcastCandidatePool(raw, cfg)
	if !slices.Contains(got, "joanne") {
		t.Fatalf("expected explicit @mention to keep joanne eligible, got %v", got)
	}
}

func TestResolveBroadcastCandidatePool_deterministic(t *testing.T) {
	cfg := testCfgSquadWithJoanne()
	raw := "@everyone welcome joanne to bimross"
	a := resolveBroadcastCandidatePool(raw, cfg)
	b := resolveBroadcastCandidatePool(raw, cfg)
	if !slices.Equal(a, b) {
		t.Fatalf("expected deterministic pool selection, got %v vs %v", a, b)
	}
}

func TestResolveBroadcastCandidatePool_excludesWelcomingPhraseTarget(t *testing.T) {
	cfg := testCfgSquadWithJoanne()
	raw := "@everyone be a little more welcoming to joanne"
	got := resolveBroadcastCandidatePool(raw, cfg)
	if slices.Contains(got, "joanne") {
		t.Fatalf("expected welcoming phrase to exclude joanne, got %v", got)
	}
}

func TestResolveBroadcastCandidatePool_plainAtMentionOverridesExclusion(t *testing.T) {
	cfg := testCfgSquadWithJoanne()
	raw := "@everyone welcome @joanne"
	got := resolveBroadcastCandidatePool(raw, cfg)
	if !slices.Contains(got, "joanne") {
		t.Fatalf("expected plain @joanne mention to override exclusion, got %v", got)
	}
}

func TestRemovedPoolKeys(t *testing.T) {
	before := []string{"ross", "tim", "alex", "joanne"}
	after := []string{"ross", "tim", "alex"}
	removed := removedPoolKeys(before, after)
	if len(removed) != 1 || removed[0] != "joanne" {
		t.Fatalf("expected removed=[joanne], got %v", removed)
	}
}
