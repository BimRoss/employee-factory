package slackbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bimross/employee-factory/internal/docsaction"
	"github.com/bimross/employee-factory/internal/googledocs"
	"github.com/sashabaranov/go-openai/jsonschema"
)

type joanneDocsActionExtract struct {
	Intent          string  `json:"intent"`
	Title           string  `json:"title,omitempty"`
	BodyInstruction string  `json:"body_instruction,omitempty"`
	BodyText        string  `json:"body_text,omitempty"`
	Confidence      float64 `json:"confidence,omitempty"`
	Reason          string  `json:"reason,omitempty"`
}

func joanneDocsActionSchema() jsonschema.Definition {
	return jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"intent": {
				Type:        jsonschema.String,
				Enum:        []string{docsaction.IntentCreateDoc, "none"},
				Description: "Action intent. Use create_doc only for explicit requests to create or draft a Google doc.",
			},
			"title": {
				Type:        jsonschema.String,
				Description: "Document title if provided.",
			},
			"body_instruction": {
				Type:        jsonschema.String,
				Description: "Instruction for drafting doc body when direct body text is not provided.",
			},
			"body_text": {
				Type:        jsonschema.String,
				Description: "Final document body text if explicitly provided.",
			},
			"confidence": {
				Type:        jsonschema.Number,
				Description: "Confidence score from 0.0 to 1.0.",
			},
			"reason": {
				Type:        jsonschema.String,
				Description: "One short reason for the selected intent.",
			},
		},
		Required: []string{"intent"},
	}
}

func (b *Bot) tryHandleJoanneCreateDoc(ctx context.Context, channel, rawText, requestUserID, messageTS, threadTS string) bool {
	if b == nil || b.cfg == nil {
		return false
	}
	if !b.cfg.JoanneGoogleDocsEnabled {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(b.cfg.EmployeeID), "joanne") {
		return false
	}
	if !shouldHandleTargetedSquadMessage(rawText, b.cfg, "joanne") {
		log.Printf("joanne_docs: skip message_ts=%s reason=targeted_to_other_employee", strings.TrimSpace(messageTS))
		return false
	}
	cmdText := strings.TrimSpace(rawText)
	if b.botUserID != "" {
		cmdText = strings.TrimSpace(strings.ReplaceAll(cmdText, "<@"+b.botUserID+">", ""))
	}

	extract, extractErr := b.extractJoanneDocsAction(ctx, cmdText)
	action, matched, parseErr, source := resolveJoanneDocsAction(cmdText, extract, extractErr)
	if !matched {
		return false
	}
	log.Printf(
		"joanne_docs: accepted message_ts=%s source=%s confidence=%.2f parse_err=%t extract_err=%t reason=%q",
		strings.TrimSpace(messageTS),
		source,
		extract.Confidence,
		parseErr != nil,
		extractErr != nil,
		strings.TrimSpace(extract.Reason),
	)
	go b.handleJoanneCreateDocSafely(ctx, channel, requestUserID, messageTS, threadTS, action, parseErr, source)
	return true
}

func resolveJoanneDocsAction(raw string, extract joanneDocsActionExtract, extractErr error) (docsaction.CreateDocAction, bool, error, string) {
	if extractErr == nil && strings.EqualFold(strings.TrimSpace(extract.Intent), docsaction.IntentCreateDoc) {
		action := docsaction.CreateDocAction{
			Intent:          docsaction.IntentCreateDoc,
			Title:           strings.TrimSpace(extract.Title),
			BodyInstruction: strings.TrimSpace(extract.BodyInstruction),
			BodyText:        strings.TrimSpace(extract.BodyText),
		}
		if action.BodyText == "" && action.BodyInstruction == "" {
			return action, true, fmt.Errorf("missing doc content"), "extractor"
		}
		return action, true, nil, "extractor"
	}
	action, matched, err := docsaction.ParseCreateDocAction(raw)
	if !matched {
		return docsaction.CreateDocAction{}, false, nil, "none"
	}
	return action, true, err, "parser"
}

func (b *Bot) extractJoanneDocsAction(ctx context.Context, cmdText string) (joanneDocsActionExtract, error) {
	var out joanneDocsActionExtract
	extractCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	systemPrompt := "You classify whether a Slack message is a create-Google-doc command for Joanne. Respond only with schema-compliant JSON."
	userPrompt := "Message:\n" + strings.TrimSpace(cmdText) + "\n\nRules:\n- intent=create_doc only for explicit requests to create/draft/make a Google doc.\n- If unsure or not a Google doc request, set intent=none.\n- Map subject as title.\n- If direct body text is provided, place it in body_text.\n- If only drafting guidance is provided, place it in body_instruction."
	err := b.llm.ExtractStructured(extractCtx, systemPrompt, userPrompt, joanneDocsActionSchema(), &out, "joanne_create_doc")
	if err != nil {
		return joanneDocsActionExtract{}, err
	}
	out.Intent = strings.ToLower(strings.TrimSpace(out.Intent))
	out.Title = strings.TrimSpace(out.Title)
	out.BodyText = strings.TrimSpace(out.BodyText)
	out.BodyInstruction = strings.TrimSpace(out.BodyInstruction)
	out.Reason = strings.TrimSpace(out.Reason)
	if out.Confidence < 0 {
		out.Confidence = 0
	}
	if out.Confidence > 1 {
		out.Confidence = 1
	}
	return out, nil
}

func (b *Bot) handleJoanneCreateDocSafely(parent context.Context, channel, requestUserID, messageTS, threadTS string, action docsaction.CreateDocAction, parseErr error, source string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("joanne_docs: panic recovered message_ts=%s source=%s panic=%v", strings.TrimSpace(messageTS), source, r)
		}
	}()
	b.handleJoanneCreateDoc(parent, channel, requestUserID, messageTS, threadTS, action, parseErr, source)
}

func (b *Bot) handleJoanneCreateDoc(parent context.Context, channel, requestUserID, messageTS, threadTS string, action docsaction.CreateDocAction, parseErr error, actionSource string) {
	commandCtx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	_ = requestUserID

	if parseErr != nil {
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I can create the doc once you give me content via `instruction:` or `body:`.")
		return
	}
	if b.googleDocsClient == nil {
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "Google Docs tooling is enabled but OAuth/runtime setup is not ready yet.")
		return
	}

	title := strings.TrimSpace(action.Title)
	if title == "" {
		title = "Joanne Draft " + time.Now().UTC().Format("2006-01-02")
	}
	body := strings.TrimSpace(action.BodyText)
	var err error
	if body == "" {
		body, err = b.generateJoanneDocBody(commandCtx, action.BodyInstruction, title)
		if err != nil {
			log.Printf("joanne_docs: body generation failed message_ts=%s err=%v", strings.TrimSpace(messageTS), err)
			b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I hit a drafting error while composing that doc. Please retry with `body:` text.")
			return
		}
	}

	result, err := b.googleDocsClient.Create(commandCtx, googledocs.CreateInput{
		Title: title,
		Body:  body,
	})
	if err != nil {
		log.Printf("joanne_docs: create failed message_ts=%s action_source=%s err=%v", strings.TrimSpace(messageTS), actionSource, err)
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I couldn't create that Google Doc. Please check OAuth/scopes and retry.")
		return
	}

	log.Printf(
		"joanne_docs: create success message_ts=%s action_source=%s doc_id=%s",
		strings.TrimSpace(messageTS),
		actionSource,
		strings.TrimSpace(result.DocumentID),
	)
	b.postJoanneEmailStatus(commandCtx, channel, threadTS, fmt.Sprintf("Google Doc created: *%s* %s", strings.TrimSpace(result.Title), strings.TrimSpace(result.URL)))
}

func (b *Bot) generateJoanneDocBody(ctx context.Context, instruction, title string) (string, error) {
	persona := b.persona.String()
	if strings.TrimSpace(persona) == "" {
		persona = "You are Joanne."
	}
	prompt := strings.TrimSpace(instruction)
	if prompt == "" {
		return "", fmt.Errorf("missing doc instruction")
	}
	userText := fmt.Sprintf(
		"Draft document body text in Joanne's voice.\nTitle: %s\nInstruction: %s\n\nReturn only the body text (plain text, no markdown fences).",
		title,
		prompt,
	)
	reply, err := b.llm.Reply(ctx, persona, "Write concise, plain-text document body only.", userText)
	if err != nil {
		return "", err
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "", fmt.Errorf("empty generated doc body")
	}
	return reply, nil
}
