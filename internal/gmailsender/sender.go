package gmailsender

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bimross/employee-factory/internal/config"
	"golang.org/x/oauth2"
)

const gmailSendEndpoint = "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"

// Sender sends Gmail messages using OAuth refresh-token auth.
type Sender struct {
	client      *http.Client
	senderEmail string
	senderName  string
}

type SendResult struct {
	MessageID string
	ThreadID  string
}

type SendInput struct {
	To      string
	Subject string
	Body    string
}

func New(cfg *config.Config) (*Sender, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		Scopes:       []string{"https://www.googleapis.com/auth/gmail.send"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}
	tok := &oauth2.Token{RefreshToken: strings.TrimSpace(cfg.GoogleRefreshToken)}
	client := oauthCfg.Client(context.Background(), tok)
	client.Timeout = 20 * time.Second
	return &Sender{
		client:      client,
		senderEmail: strings.TrimSpace(cfg.GoogleSenderEmail),
		senderName:  resolveSenderName(cfg),
	}, nil
}

func (s *Sender) Send(ctx context.Context, in SendInput) (SendResult, error) {
	if s == nil || s.client == nil {
		return SendResult{}, fmt.Errorf("gmail sender is not initialized")
	}
	to := strings.TrimSpace(in.To)
	subject := strings.TrimSpace(in.Subject)
	body := strings.TrimSpace(in.Body)
	if to == "" {
		return SendResult{}, fmt.Errorf("missing recipient email")
	}
	if subject == "" {
		subject = "Message from Joanne"
	}
	if body == "" {
		return SendResult{}, fmt.Errorf("missing email body")
	}

	raw := buildRawMessage(s.senderName, s.senderEmail, to, subject, body)
	payload := map[string]string{"raw": raw}
	b, err := json.Marshal(payload)
	if err != nil {
		return SendResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, gmailSendEndpoint, bytes.NewReader(b))
	if err != nil {
		return SendResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return SendResult{}, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return SendResult{}, fmt.Errorf("gmail send failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	result := parseSendResultJSON(rb)
	return result, nil
}

func buildRawMessage(fromName, fromEmail, to, subject, body string) string {
	from := strings.TrimSpace(fromEmail)
	if n := strings.TrimSpace(fromName); n != "" {
		quoted := strings.ReplaceAll(n, "\"", "\\\"")
		from = fmt.Sprintf("\"%s\" <%s>", quoted, from)
	}
	msg := "From: " + from + "\r\n" +
		"To: " + strings.TrimSpace(to) + "\r\n" +
		"Subject: " + strings.TrimSpace(subject) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=\"UTF-8\"\r\n" +
		"\r\n" +
		strings.TrimSpace(body)
	return base64.RawURLEncoding.EncodeToString([]byte(msg))
}

func resolveSenderName(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if n := strings.TrimSpace(cfg.GoogleSenderName); n != "" {
		return n
	}
	if strings.EqualFold(strings.TrimSpace(cfg.EmployeeID), "joanne") {
		return "Joanne"
	}
	return ""
}

func parseSendResultJSON(body []byte) SendResult {
	if len(body) == 0 {
		return SendResult{}
	}
	var payload struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return SendResult{}
	}
	return SendResult{
		MessageID: strings.TrimSpace(payload.ID),
		ThreadID:  strings.TrimSpace(payload.ThreadID),
	}
}
