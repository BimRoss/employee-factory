package opsproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	token = strings.TrimSpace(token)
	if baseURL == "" {
		return nil, fmt.Errorf("ops proxy base URL is required")
	}
	if token == "" {
		return nil, fmt.Errorf("ops proxy token is required")
	}
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *Client) Status(ctx context.Context, req StatusRequest) (StatusResponse, error) {
	var out StatusResponse
	if err := c.postJSON(ctx, "/k8s/status", req, &out); err != nil {
		return StatusResponse{}, err
	}
	return out, nil
}

func (c *Client) Logs(ctx context.Context, req LogsRequest) (LogsResponse, error) {
	var out LogsResponse
	if err := c.postJSON(ctx, "/k8s/logs", req, &out); err != nil {
		return LogsResponse{}, err
	}
	return out, nil
}

func (c *Client) RedisRead(ctx context.Context, req RedisReadRequest) (RedisReadResponse, error) {
	var out RedisReadResponse
	if err := c.postJSON(ctx, "/redis/read", req, &out); err != nil {
		return RedisReadResponse{}, err
	}
	return out, nil
}

func (c *Client) postJSON(ctx context.Context, endpoint string, reqBody any, out any) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ops proxy returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
