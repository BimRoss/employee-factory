package slackbot

import "strings"

type ResponseKind string

const (
	ResponseKindChat           ResponseKind = "chat"
	ResponseKindOpsMetrics     ResponseKind = "ops_metrics"
	ResponseKindOpsWaitlist    ResponseKind = "ops_waitlist"
	ResponseKindOpsStatus      ResponseKind = "ops_status"
	ResponseKindOpsLogs        ResponseKind = "ops_logs"
	ResponseKindStructuredJSON ResponseKind = "structured_json"
)

type PresentationMode string

const (
	PresentationModePlainText   PresentationMode = "plain_text"
	PresentationModeFencedJSON  PresentationMode = "fenced_json"
	PresentationModeSlackBlocks PresentationMode = "slack_blocks"
)

type PresentationOptions struct {
	EnableBlocks  bool
	JSONMode      string // off | auto | force_for_structured
	MaxBlockItems int
	ForceText     bool
	PreferredMode PresentationMode

	// Payload hints used by the resolver.
	ItemCount     int
	HasStructured bool
	ApproxRunes   int
}

func normalizePresentationJSONMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "auto":
		return "auto"
	case "force_for_structured":
		return "force_for_structured"
	default:
		return "off"
	}
}

func ResolvePresentation(kind ResponseKind, opts PresentationOptions) (PresentationMode, string) {
	if opts.ForceText {
		return PresentationModePlainText, "force_text"
	}
	if opts.PreferredMode != "" {
		switch opts.PreferredMode {
		case PresentationModePlainText, PresentationModeFencedJSON, PresentationModeSlackBlocks:
			return opts.PreferredMode, "preferred_mode"
		}
	}

	jsonMode := normalizePresentationJSONMode(opts.JSONMode)
	maxItems := opts.MaxBlockItems
	if maxItems <= 0 {
		maxItems = 8
	}

	switch kind {
	case ResponseKindChat:
		return PresentationModePlainText, "chat_default_plain"
	case ResponseKindOpsLogs:
		// Keep logs in text so code fences remain compact and copy/paste friendly.
		return PresentationModePlainText, "ops_logs_plain"
	case ResponseKindStructuredJSON:
		if jsonMode == "force_for_structured" || jsonMode == "auto" {
			return PresentationModeFencedJSON, "structured_json_mode"
		}
		return PresentationModePlainText, "structured_json_plain_fallback"
	case ResponseKindOpsMetrics, ResponseKindOpsWaitlist:
		if opts.EnableBlocks && opts.ItemCount > 0 && opts.ItemCount <= maxItems {
			return PresentationModeSlackBlocks, "blocks_enabled_bounded_items"
		}
		if opts.HasStructured && (jsonMode == "force_for_structured" || jsonMode == "auto") {
			return PresentationModeFencedJSON, "structured_fenced_json"
		}
		return PresentationModePlainText, "ops_plain_default"
	case ResponseKindOpsStatus:
		if opts.HasStructured && jsonMode == "force_for_structured" {
			return PresentationModeFencedJSON, "status_forced_json"
		}
		return PresentationModePlainText, "status_plain_default"
	default:
		return PresentationModePlainText, "unknown_kind_plain"
	}
}
