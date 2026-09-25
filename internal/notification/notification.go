// Package notification sends alerts to Telegram and/or a webhook.
// A notifier failure must never break the watch loop.
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Config mirrors the notification section of config.yaml.
type Config struct {
	Telegram TelegramConfig
	Webhook  WebhookConfig
	Recovery bool
}

type TelegramConfig struct {
	Enabled     bool
	BotTokenEnv string
	ChatIDEnv   string
}

type WebhookConfig struct {
	Enabled bool
	URL     string
}

// Notifier sends messages through configured channels.
type Notifier struct {
	cfg Config
	hc  *http.Client
}

// New builds a Notifier from config.
func New(cfg Config) *Notifier {
	return &Notifier{cfg: cfg, hc: &http.Client{Timeout: 10 * time.Second}}
}

// Send delivers a text message to all enabled channels. Never returns an error
// that the caller must treat as fatal; all failures are swallowed.
func (n *Notifier) Send(ctx context.Context, text string) {
	var errs []string
	if n.cfg.Telegram.Enabled {
		if err := n.sendTelegram(ctx, text); err != nil {
			errs = append(errs, "telegram: "+err.Error())
		}
	}
	if n.cfg.Webhook.Enabled {
		if err := n.sendWebhook(ctx, text); err != nil {
			errs = append(errs, "webhook: "+err.Error())
		}
	}
	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "notification failure:", e)
	}
}

func (n *Notifier) sendTelegram(ctx context.Context, text string) error {
	token := os.Getenv(n.cfg.Telegram.BotTokenEnv)
	chatID := os.Getenv(n.cfg.Telegram.ChatIDEnv)
	if token == "" || chatID == "" {
		return fmt.Errorf("telegram token/chat_id env not set")
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	body, _ := json.Marshal(map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.hc.Do(req)
	if err != nil {
		// Do NOT return err: net/http's *url.Error includes the full request
		// URL, which for Telegram carries the bot token. Return a fixed,
		// secret-free message instead.
		return fmt.Errorf("telegram transport error")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telegram HTTP %d", resp.StatusCode)
	}
	return nil
}

func (n *Notifier) sendWebhook(ctx context.Context, text string) error {
	if n.cfg.Webhook.URL == "" {
		return fmt.Errorf("webhook url empty")
	}
	body, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.cfg.Webhook.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.hc.Do(req)
	if err != nil {
		// Do NOT return err: net/http's *url.Error includes the full URL, which
		// may carry credentials (?token=...). Return a secret-free message.
		return fmt.Errorf("webhook transport error")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook HTTP %d", resp.StatusCode)
	}
	return nil
}