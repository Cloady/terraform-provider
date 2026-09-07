// Package client implements only the HTTP transport needed by the provider.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 16 << 20

type Client struct {
	baseURL string
	token   string
	version string
	http    *http.Client
}

func New(baseURL, token, version string, timeout time.Duration) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("base_url must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("an API token is required")
	}
	if timeout <= 0 {
		return nil, errors.New("request timeout must be positive")
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, version: version,
		http: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Cloady API returned HTTP %d (%s): %s", e.StatusCode, e.Code, e.Message)
}
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// Do never retries mutations: the API has no idempotency-key contract. Context
// cancellation and the configured timeout bound all calls. Bodies are never logged.
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	if !strings.HasPrefix(path, "/api/") {
		return errors.New("API path must start with /api/")
	}
	var input io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		input = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, input)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "terraform-provider-cloady/"+c.version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Cloady request failed: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return errors.New("Cloady response exceeds 16 MiB")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		apiErr := &APIError{StatusCode: res.StatusCode, Code: http.StatusText(res.StatusCode), Message: "request was rejected"}
		var envelope struct {
			Error json.RawMessage `json:"error"`
			Code  string          `json:"code"`
		}
		if json.Unmarshal(raw, &envelope) == nil {
			var nested struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			var message string
			if json.Unmarshal(envelope.Error, &nested) == nil && nested.Message != "" {
				apiErr.Code, apiErr.Message = nested.Code, nested.Message
			} else if json.Unmarshal(envelope.Error, &message) == nil && message != "" {
				apiErr.Code, apiErr.Message = envelope.Code, message
			}
		}
		apiErr.Message = strings.ReplaceAll(apiErr.Message, c.token, "[REDACTED]")
		return apiErr
	}
	if out == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode Cloady response: %w", err)
	}
	return nil
}

func WorkspacePath(slug string) string { return "/api/workspaces/" + url.PathEscape(slug) }
func AppPath(workspace, app, environment, region, suffix string) string {
	return WorkspacePath(workspace) + "/apps/" + url.PathEscape(app) + suffix + "?" + url.Values{"env": {environment}, "region": {region}}.Encode()
}
