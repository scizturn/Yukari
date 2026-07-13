package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// discordContentLimit is Discord's hard cap on a webhook message body. Anything
// longer is rejected with a 400, so callers must truncate rather than discover
// it in production.
const discordContentLimit = 2000

type DiscordLogger struct {
	WebhookURL string
	Enabled    bool
	HTTPClient *http.Client
}

func (l DiscordLogger) Log(ctx context.Context, content string) error {
	if !l.Enabled || l.WebhookURL == "" {
		return nil
	}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(map[string]string{"content": Truncate(content)}); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.WebhookURL, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := l.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook failed: status=%d", resp.StatusCode)
	}
	return nil
}

// Truncate cuts content to Discord's message limit, counting runes so a cut
// never lands inside a multi-byte character.
func Truncate(content string) string {
	runes := []rune(content)
	if len(runes) <= discordContentLimit {
		return content
	}
	const ellipsis = "\n… (dipotong)"
	keep := discordContentLimit - len([]rune(ellipsis))
	return string(runes[:keep]) + ellipsis
}
