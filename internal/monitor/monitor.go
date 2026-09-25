// Package monitor evaluates thresholds, dedupes alerts, and runs the watch loop.
package monitor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redtidev1918/api-balance/internal/config"
	"github.com/redtidev1918/api-balance/internal/provider"
	"github.com/redtidev1918/api-balance/internal/state"
)

// Monitor drives watch: check -> evaluate -> notify -> sleep.
type Monitor struct {
	providers    map[string]provider.Provider
	thresholds   map[string]config.Threshold
	notifier     Sender
	state        *state.AlertState
	recovery     bool
	manager      *provider.Manager
}

// Sender abstracts message delivery so tests can capture without network.
type Sender interface {
	Send(ctx context.Context, text string)
}

// New builds a Monitor.
func New(
	providers map[string]provider.Provider,
	thresholds map[string]config.Threshold,
	notifier Sender,
	state *state.AlertState,
	recovery bool,
	concurrency int,
) *Monitor {
	return &Monitor{
		providers:  providers,
		thresholds: thresholds,
		notifier:   notifier,
		state:      state,
		recovery:   recovery,
		manager:    provider.NewManager(concurrency),
	}
}

// Evaluate compares results against thresholds and emits alerts for providers
// that cross below. Returns the list of alerted providers (names) this round.
// Deduplication: once alerted, no re-notify until the value recovers above the
// threshold (which clears the alert state).
func (m *Monitor) Evaluate(ctx context.Context, results []provider.BalanceResult) []string {
	var alerted []string
	var builder strings.Builder
	checked := ""
	if len(results) > 0 {
		checked = results[0].CheckedAt.Format("2006-01-02 15:04:05")
	}
	first := true

	for _, r := range results {
		if r.Status != provider.StatusOK {
			// non-OK providers are not threshold-eligible; skip
			continue
		}
		th, ok := m.thresholds[r.Provider]
		if !ok {
			continue
		}
		tripped, msg := breakThreshold(r, th)
		if tripped {
			if m.state.IsActive(r.Provider) {
				continue // already alerting, dedupe
			}
			// transition into alert
			if !m.state.Enter(r.Provider) {
				continue
			}
			if !first {
				builder.WriteString("\n")
			}
			builder.WriteString(msg)
			first = false
			alerted = append(alerted, r.Provider)
		} else {
			// recovered
			if m.state.Exit(r.Provider) && m.recovery {
				if !first {
					builder.WriteString("\n")
				}
				builder.WriteString(recoveryMsg(r, th))
				first = false
				// recovery is informational, do not add to alerted
			}
		}
	}

	if builder.Len() == 0 {
		return alerted
	}

	// build message
	var msg strings.Builder
	if len(alerted) > 0 {
		msg.WriteString("API Balance Alert\n")
	} else {
		msg.WriteString("API Balance Recovery\n")
	}
	msg.WriteString(builder.String())
	if checked != "" {
		msg.WriteString("\nChecked: " + checked)
	}
	m.notifier.Send(ctx, msg.String())
	return alerted
}

// breakThreshold decides whether r violates th.
func breakThreshold(r provider.BalanceResult, th config.Threshold) (bool, string) {
	var line string
	switch {
	case th.Balance != nil && r.Balance != nil && r.Currency != nil:
		val := *r.Balance
		if val <= *th.Balance {
			line = fmt.Sprintf("%s %s %.2f <= %s %.2f",
				r.Provider, *r.Currency, val, *r.Currency, *th.Balance)
			return true, line
		}
	case th.RemainingPercent != nil && r.RemainingPercent != nil:
		if *r.RemainingPercent <= *th.RemainingPercent {
			line = fmt.Sprintf("%s %.0f%% remaining <= %.0f%%",
				r.Provider, *r.RemainingPercent, *th.RemainingPercent)
			return true, line
		}
	}
	return false, ""
}

func recoveryMsg(r provider.BalanceResult, th config.Threshold) string {
	switch {
	case th.Balance != nil && r.Balance != nil && r.Currency != nil:
		return fmt.Sprintf("%s Current: %s %.2f Threshold: %s %.2f",
			r.Provider, *r.Currency, *r.Balance, *r.Currency, *th.Balance)
	case th.RemainingPercent != nil && r.RemainingPercent != nil:
		return fmt.Sprintf("%s Current: %.0f%% Threshold: %.0f%%",
			r.Provider, *r.RemainingPercent, *th.RemainingPercent)
	}
	return r.Provider + " recovered"
}

// RunWatch executes the check→evaluate→notify loop on an interval.
// The first check runs immediately (does not wait for the interval).
func (m *Monitor) RunWatch(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		m.runOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (m *Monitor) runOnce(ctx context.Context) {
	// each cycle: check all providers concurrently (bounded by manager)
	results := m.manager.CheckAll(ctx, m.providers)
	m.Evaluate(ctx, results)
}