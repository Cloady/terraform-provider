package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"nested error", 404, `{"error":{"code":"not_found","message":"gone"}}`, "gone"},
		{"deploy error", 409, `{"error":"pool full","code":"insufficient_pool"}`, "pool full"},
		{"non JSON error", 502, `<html>private diagnostic</html>`, "request was rejected"},
		{"token redacted", 403, `{"error":"secret-token denied"}`, "[REDACTED] denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != "Bearer secret-token" {
					t.Error("missing token")
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Error("missing JSON content type")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c, err := New(srv.URL, "secret-token", "test", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			err = c.Do(context.Background(), "POST", "/api/workspaces", map[string]string{"name": "test"}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
			if calls != 1 {
				t.Fatalf("mutation retried %d times", calls)
			}
			if IsNotFound(err) != (tc.status == 404) {
				t.Fatal("wrong 404 classification")
			}
		})
	}
}
func TestRedirectIsNotFollowed(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("redirect followed with credentials") }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c, _ := New(srv.URL, "token", "test", time.Second)
	if err := c.Do(context.Background(), "POST", "/api/workspaces", nil, nil); err == nil {
		t.Fatal("expected redirect error")
	}
}
func TestCancellation(t *testing.T) {
	c, _ := New("https://cloady.com", "token", "test", time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Do(ctx, "GET", "/api/regions", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation: %v", err)
	}
}
func TestURLValidation(t *testing.T) {
	for _, u := range []string{"", ":bad", "file:///tmp/x", "https://user:pass@example.com", "https://example.com?q=x", "https://example.com#x"} {
		if _, err := New(u, "token", "test", time.Second); err == nil {
			t.Errorf("accepted %q", u)
		}
	}
}
func TestPaths(t *testing.T) {
	got := AppPath("a b", "web", "preview", "eu&1", "/vars")
	if got != "/api/workspaces/a%20b/apps/web/vars?env=preview&region=eu%261" {
		t.Fatal(got)
	}
}
