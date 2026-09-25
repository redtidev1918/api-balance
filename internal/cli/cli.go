// Package cli implements the api-balance command-line interface.
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/redtidev1918/api-balance/internal/app"
	"github.com/redtidev1918/api-balance/internal/config"
	"github.com/redtidev1918/api-balance/internal/monitor"
	"github.com/redtidev1918/api-balance/internal/notification"
	"github.com/redtidev1918/api-balance/internal/output"
	"github.com/redtidev1918/api-balance/internal/provider"
	"github.com/redtidev1918/api-balance/internal/state"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

// Run dispatches subcommands.
func Run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "version":
		fmt.Println("api-balance " + Version)
		return 0
	case "help", "-h", "--help":
		usage()
		return 0
	case "check":
		return cmdCheck(rest)
	case "watch":
		return cmdWatch(rest)
	case "providers":
		return cmdProviders(rest)
	case "config":
		return cmdConfig(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		return 2
	}
}
func usage() {
	fmt.Fprint(os.Stderr, `api-balance - multi-provider AI API balance/quota CLI

Usage:
  api-balance check [--json] [--config PATH]
  api-balance watch [--interval DURATION] [--once] [--config PATH]
  api-balance providers
  api-balance config validate [--config PATH]
  api-balance version
  api-balance help

Config precedence: CLI flag > env > ~/.config/api-balance/config.yaml > /etc/api-balance/config.yaml
Env vars for keys are set per-provider via api_key_env.
`)
}

// loadConfig is the shared loader for most commands.
func loadConfig(path string) (*config.Config, error) {
	// Load systemd-style env file if present (for service mode).
	loadEnvFile("/etc/api-balance/api-balance.env")
	return config.Load(path)
}

// loadEnvFile sources KEY=VALUE lines from a file into the process env.
// Best-effort; missing files are ignored. Line values are not logged.
func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.IndexByte(line, '='); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			// skip shell-style quotes wrapping the value
			val = strings.Trim(val, `"'`)
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "output as JSON")
	cfgPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	timeout := 10 * time.Second
	provs, err := app.BuildProviders(cfg, timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "provider error:", err)
		return 1
	}
	if len(provs) == 0 {
		fmt.Fprintln(os.Stderr, "no providers configured; edit config or run 'api-balance providers'")
		return 1
	}

	ctx := context.Background()
	mgr := provider.NewManager(cfg.Concurrency)
	results := mgr.CheckAll(ctx, provs)
	checkedAt := time.Now().UTC()
	for i := range results {
		if results[i].CheckedAt.IsZero() {
			results[i].CheckedAt = checkedAt
		}
	}

	if *jsonOut {
		if err := output.RenderJSON(os.Stdout, results, checkedAt); err != nil {
			fmt.Fprintln(os.Stderr, "output error:", err)
			return 1
		}
	} else {
		if err := output.RenderPlain(os.Stdout, results); err != nil {
			fmt.Fprintln(os.Stderr, "output error:", err)
			return 1
		}
	}
	return 0
}

func cmdWatch(args []string) int {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	intervalStr := fs.String("interval", "", "watch interval (e.g. 30m). defaults to config or 30m")
	once := fs.Bool("once", false, "run a single check+evaluate then exit (for testing)")
	cfgPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		return 1
	}
	timeout := 10 * time.Second
	provs, err := app.BuildProviders(cfg, timeout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "provider error:", err)
		return 1
	}
	if len(provs) == 0 {
		fmt.Fprintln(os.Stderr, "no providers configured")
		return 1
	}

	// thresholds from config
	thresholds := map[string]config.Threshold{}
	for name, pc := range cfg.Providers {
		if pc.Threshold != nil {
			thresholds[name] = *pc.Threshold
		}
	}

	notifCfg := notification.Config{
		Telegram: notification.TelegramConfig{
			Enabled:     cfg.Notifications.Telegram.Enabled,
			BotTokenEnv: cfg.Notifications.Telegram.BotTokenEnv,
			ChatIDEnv:   cfg.Notifications.Telegram.ChatIDEnv,
		},
		Webhook: notification.WebhookConfig{
			Enabled: cfg.Notifications.Webhook.Enabled,
			URL:     cfg.Notifications.Webhook.URL,
		},
		Recovery: cfg.Notifications.Recovery,
	}
	notifier := notification.New(notifCfg)

	stateFile := cfg.StateFile
	if stateFile == "" {
		stateFile = config.DefaultStatePath()
	}
	st := state.New(stateFile)

	m := monitor.New(provs, thresholds, notifier, st, notifCfg.Recovery, cfg.Concurrency)

	interval := time.Duration(0)
	if *intervalStr != "" {
		d, err := time.ParseDuration(*intervalStr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bad interval:", err)
			return 1
		}
		interval = d
	}
	if interval <= 0 {
		interval, _ = time.ParseDuration(cfg.Interval)
	}

	if *once {
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		mgr := provider.NewManager(cfg.Concurrency)
		results := mgr.CheckAll(ctx, provs)
		m.Evaluate(ctx, results)
		return 0
	}

	fmt.Fprintf(os.Stderr, "watching every %s (ctrl-c to stop)\n", interval)
	ctx := context.Background()
	m.RunWatch(ctx, interval)
	return 0
}

func cmdProviders(args []string) int {
	fs := flag.NewFlagSet("providers", flag.ExitOnError)
	fs.Parse(args)

	names := make([]string, 0, len(app.BuiltinNames))
	stabilities := map[string]string{}
	for n := range app.BuiltinNames {
		names = append(names, n)
		stabilities[n] = string(app.BuiltinNames[n])
	}
	sort.Strings(names)
	fmt.Println("built-in providers:")
	for _, n := range names {
		fmt.Printf("  %-14s %s\n", n, stabilities[n])
	}
	fmt.Println("\ncustom providers: configure type: custom in config.yaml")
	return 0
}

func cmdConfig(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: api-balance config validate [--config PATH]")
		return 2
	}
	switch args[0] {
	case "validate":
		fs := flag.NewFlagSet("config validate", flag.ExitOnError)
		cfgPath := fs.String("config", "", "path to config file")
		fs.Parse(args[1:])
		cfg, err := loadConfig(*cfgPath) // loads env file then validates
		if err != nil {
			fmt.Fprintln(os.Stderr, "invalid config:", err)
			return 1
		}
		_ = cfg
		fmt.Println("config OK")
		return 0
	case "path", "show":
		fmt.Println("user:", config.UserConfigPath)
		fmt.Println("system:", config.SystemConfigPath)
		return 0
	default:
		fmt.Fprintln(os.Stderr, "unknown config subcommand:", args[0])
		return 2
	}
}