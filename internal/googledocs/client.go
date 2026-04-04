package googledocs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bimross/employee-factory/internal/config"
	"golang.org/x/oauth2"
)

const (
	docsCreateEndpointBase = "https://docs.googleapis.com/v1/documents"
	docsScopeWrite         = "https://www.googleapis.com/auth/documents"
)

type Client struct {
	client *http.Client
}

type CreateInput struct {
	Title string
	Body  string
}

type CreateResult struct {
	DocumentID string
	Title      string
	URL        string
}

func New(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		Scopes:       []string{docsScopeWrite},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}
	tok := &oauth2.Token{RefreshToken: strings.TrimSpace(cfg.GoogleRefreshToken)}
	httpClient := oauthCfg.Client(context.Background(), tok)
	httpClient.Timeout = 20 * time.Second
	return &Client{client: httpClient}, nil
}

func (c *Client) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	if c == nil || c.client == nil {
		return CreateResult{}, fmt.Errorf("google docs client is not initialized")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "Joanne Draft"
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return CreateResult{}, fmt.Errorf("missing document body")
	}

	docID, createdTitle, err := c.createDocument(ctx, title)
	if err != nil {
		return CreateResult{}, err
	}
	if err := c.insertDocumentBody(ctx, docID, body); err != nil {
		return CreateResult{}, err
	}

	return CreateResult{
		DocumentID: docID,
		Title:      createdTitle,
		URL:        fmt.Sprintf("https://docs.google.com/document/d/%s/edit", url.PathEscape(docID)),
	}, nil
}

func (c *Client) createDocument(ctx context.Context, title string) (documentID string, finalTitle string, err error) {
	payload := map[string]string{"title": title}
	b, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, docsCreateEndpointBase, bytes.NewReader(b))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("docs create failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	var out struct {
		DocumentID string `json:"documentId"`
		Title      string `json:"title"`
	}
	if err := json.Unmarshal(rb, &out); err != nil {
		return "", "", err
	}
	out.DocumentID = strings.TrimSpace(out.DocumentID)
	out.Title = strings.TrimSpace(out.Title)
	if out.DocumentID == "" {
		return "", "", fmt.Errorf("docs create returned empty documentId")
	}
	if out.Title == "" {
		out.Title = title
	}
	return out.DocumentID, out.Title, nil
}

func (c *Client) insertDocumentBody(ctx context.Context, documentID, body string) error {
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return fmt.Errorf("missing document id")
	}
	payload := map[string]any{
		"requests": []map[string]any{
			{
				"insertText": map[string]any{
					"location": map[string]any{
						"index": 1,
					},
					"text": body,
				},
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/%s:batchUpdate", docsCreateEndpointBase, url.PathEscape(documentID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("docs batchUpdate failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(rb)))
	}
	return nil
}
