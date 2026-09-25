// Package config loads and validates api-balance configuration.
// Precedence (highest wins): CLI flags > env > user config > system config.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProviderConfig configures a single provider (native or custom).
type ProviderConfig struct {
	Type string `yaml:"type,omitempty"` // "native" (default) or "custom"

	// API key sources. Exactly one is typically set.
	APIKey     string `yaml:"api_key,omitempty"`     // inline (discouraged, warn)
	APIKeyEnv  string `yaml:"api_key_env,omitempty"` // env var name holding the key

	// Custom provider spec (only for type: custom).
	Endpoint string                 `yaml:"endpoint,omitempty"`
	Method   string                 `yaml:"method,omitempty"`
	Auth     CustomAuth             `yaml:"auth,omitempty"`
	Headers  map[string]string      `yaml:"headers,omitempty"`
	Query    map[string]string      `yaml:"query,omitempty"`
	Body     map[string]interface{} `yaml:"body,omitempty"`
	Extract  CustomExtract          `yaml:"extract,omitempty"`

	// Thresholds (in this provider's own currency/units).
	Threshold *Threshold `yaml:"threshold,omitempty"`
}

// CustomAuth describes how to authenticate a custom provider request.
type CustomAuth struct {
	Type        string            `yaml:"type,omitempty"` // bearer | header | query
	TokenEnv    string            `yaml:"token_env,omitempty"`
	HeaderName  string            `yaml:"header_name,omitempty"`
	QueryName   string            `yaml:"query_name,omitempty"`
	Credentials map[string]string `yaml:"credentials,omitempty"`
}

// CustomExtract maps a JSON response to the unified model using JMESPath/JSONPath.
type CustomExtract struct {
	Balance     string `yaml:"balance,omitempty"`
	Currency    string `yaml:"currency,omitempty"`
	Used        string `yaml:"used,omitempty"`
	Total       string `yaml:"total,omitempty"`
	Remaining   string `yaml:"remaining,omitempty"`
	ResetAt     string `yaml:"reset_at,omitempty"`
	StatusPath  string `yaml:"status,omitempty"`
	StatusOK    string `yaml:"status_ok,omitempty"`
}

// Threshold is a per-provider alert threshold.
type Threshold struct {
	Balance          *float64 `yaml:"balance,omitempty"`
	RemainingPercent *float64 `yaml:"remaining_percent,omitempty"`
}

// TelegramConfig configures the Telegram notifier.
type TelegramConfig struct {
	Enabled      bool   `yaml:"enabled"`
	BotTokenEnv  string `yaml:"bot_token_env,omitempty"`
	ChatIDEnv    string `yaml:"chat_id_env,omitempty"`
}

// WebhookConfig configures a generic webhook notifier.
type WebhookConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url,omitempty"`
}

// NotificationConfig groups all notifiers.
type NotificationConfig struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Webhook  WebhookConfig  `yaml:"webhook"`
	Recovery bool           `yaml:"recovery"`
}

// Config is the top-level configuration document.
type Config struct {
	Providers     map[string]ProviderConfig `yaml:"providers"`
	Notifications NotificationConfig         `yaml:"notifications"`
	Interval      string                     `yaml:"watch_interval,omitempty"`
	Concurrency   int                        `yaml:"concurrency,omitempty"`
	StateFile     string                     `yaml:"state_file,omitempty"`
}

// DefaultConcurrency caps concurrent provider checks.
const DefaultConcurrency = 5

// DefaultWatchInterval is used when watch runs without --interval/config.
const DefaultWatchInterval = "30m"

// Paths for config search.
const (
	UserConfigPath   = "~/.config/api-balance/config.yaml"
	SystemConfigPath = "/etc/api-balance/config.yaml"
)

// Load merges user + system config per precedence and applies defaults.
// "path" is optional; when empty the standard search paths are used.
func Load(path string) (*Config, error) {
	cfg := &Config{Concurrency: DefaultConcurrency, Interval: DefaultWatchInterval}

	// system config (lowest)
	if err := mergeFile(cfg, SystemConfigPath); err != nil {
		return nil, err
	}
	// user config (overrides)
	userPath := expandHome(UserConfigPath)
	if path != "" {
		userPath = path
	}
	if err := mergeFile(cfg, userPath); err != nil {
		return nil, err
	}

	// defaults
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = DefaultConcurrency
	}
	if cfg.Interval == "" {
		cfg.Interval = DefaultWatchInterval
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func mergeFile(cfg *Config, path string) error {
	path = expandHome(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // missing file is fine
		}
		return err
	}
	var parsed Config
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	// merge parsed into cfg (parsed wins on non-zero fields)
	if parsed.Providers != nil {
		if cfg.Providers == nil {
			cfg.Providers = map[string]ProviderConfig{}
		}
		for k, v := range parsed.Providers {
			cfg.Providers[k] = v
		}
	}
	if parsed.Notifications.Telegram.Enabled || parsed.Notifications.Telegram.BotTokenEnv != "" ||
		parsed.Notifications.Telegram.ChatIDEnv != "" {
		cfg.Notifications.Telegram = parsed.Notifications.Telegram
	}
	if parsed.Notifications.Webhook.Enabled || parsed.Notifications.Webhook.URL != "" {
		cfg.Notifications.Webhook = parsed.Notifications.Webhook
	}
	if parsed.Notifications.Recovery {
		cfg.Notifications.Recovery = true
	}
	if parsed.Interval != "" {
		cfg.Interval = parsed.Interval
	}
	if parsed.Concurrency > 0 {
		cfg.Concurrency = parsed.Concurrency
	}
	if parsed.StateFile != "" {
		cfg.StateFile = parsed.StateFile
	}
	return nil
}

// Validate performs structural checks on the merged config.
func (c *Config) Validate() error {
	for name, p := range c.Providers {
		if p.Type == "" {
			p.Type = "native"
		}
		if p.Type == "custom" {
			if p.Endpoint == "" {
				return fmt.Errorf("provider %q: custom type requires endpoint", name)
			}
			// need at least an extractor
			e := p.Extract
			if e.Balance == "" && e.Used == "" && e.Total == "" && e.Remaining == "" {
				return fmt.Errorf("provider %q: custom extract needs at least one of balance/used/total/remaining", name)
			}
		}
		// key resolution
		if p.APIKey == "" && p.APIKeyEnv == "" {
			return fmt.Errorf("provider %q: no api_key or api_key_env set", name)
		}
	}
	if err := c.validateAuthKeys(); err != nil {
		return err
	}
	return nil
}

// validateAuthKeys resolves env var indirections without leaking values.
func (c *Config) validateAuthKeys() error {
	for name, p := range c.Providers {
		if p.APIKeyEnv != "" {
			if os.Getenv(p.APIKeyEnv) == "" {
				return fmt.Errorf("provider %q: env %q is set but empty", name, p.APIKeyEnv)
			}
		}
		if p.APIKey == "" && p.APIKeyEnv == "" && p.Type != "custom" {
			return errors.New("provider " + name + " missing key")
		}
		// custom auth env
		if p.Type == "custom" && p.Auth.TokenEnv != "" && os.Getenv(p.Auth.TokenEnv) == "" {
			return fmt.Errorf("provider %q: auth token_env %q empty", name, p.Auth.TokenEnv)
		}
	}
	return nil
}

// KeyFor returns the resolved API key for a provider, or "" if configured via
// an env that is absent. It never logs the value.
func KeyFor(p ProviderConfig) string {
	if p.APIKey != "" {
		return p.APIKey
	}
	if p.APIKeyEnv != "" {
		return os.Getenv(p.APIKeyEnv)
	}
	if p.Type == "custom" && p.Auth.TokenEnv != "" {
		return os.Getenv(p.Auth.TokenEnv)
	}
	return ""
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

// DefaultStatePath returns the default alert-state file location.
func DefaultStatePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/api-balance-state.json"
	}
	return filepath.Join(home, ".config", "api-balance", "state.json")
}