package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiscordLoggerPostsContent(t *testing.T) {
	var body map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := DiscordLogger{WebhookURL: server.URL, Enabled: true, HTTPClient: server.Client()}
	err := logger.Log(context.Background(), "yukari cron run ok")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if body["content"] != "yukari cron run ok" {
		t.Fatalf("expected discord content, got %#v", body)
	}
}

func TestDiscordLoggerSkipsWhenNotConfigured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("expected no request when the webhook is not configured")
	}))
	defer server.Close()

	for _, logger := range []DiscordLogger{
		{WebhookURL: "", Enabled: true, HTTPClient: server.Client()},
		{WebhookURL: server.URL, Enabled: false, HTTPClient: server.Client()},
	} {
		if err := logger.Log(context.Background(), "ignored"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	}
}

// Discord rejects a body over 2000 characters with a 400, so a run that skipped
// thousands of users must not lose its whole report to an oversized message.
func TestDiscordLoggerTruncatesOversizedContent(t *testing.T) {
	var body map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := DiscordLogger{WebhookURL: server.URL, Enabled: true, HTTPClient: server.Client()}
	if err := logger.Log(context.Background(), strings.Repeat("a", 5000)); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	content := []rune(body["content"])
	if len(content) != discordContentLimit {
		t.Fatalf("expected content truncated to %d runes, got %d", discordContentLimit, len(content))
	}
	if !strings.HasSuffix(body["content"], "(dipotong)") {
		t.Fatalf("expected truncation marker, got %q", string(content[len(content)-20:]))
	}
}

func TestDiscordLoggerReportsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := DiscordLogger{WebhookURL: server.URL, Enabled: true, HTTPClient: server.Client()}
	if err := logger.Log(context.Background(), "boom"); err == nil {
		t.Fatal("expected an error when discord returns 500")
	}
}
