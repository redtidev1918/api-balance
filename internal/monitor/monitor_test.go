package monitor

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redtidev1918/api-balance/internal/config"
	"github.com/redtidev1918/api-balance/internal/provider"
	"github.com/redtidev1918/api-balance/internal/state"
)

// fakeSender captures messages.
type fakeSender struct {
	mu     sync.Mutex
	messages []string
}
func (f *fakeSender) Send(_ context.Context, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, text)
}
func (f *fakeSender) count() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.messages) }
func (f *fakeSender) last() string { f.mu.Lock(); defer f.mu.Unlock(); if len(f.messages) == 0 { return "" }; return f.messages[len(f.messages)-1] }

// fakeProvider returns a fixed result.
type fakeProvider struct {
	name string
	res  provider.BalanceResult
}
func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Check(context.Context) provider.BalanceResult { return f.res }

func bal(name string, currency string, v float64) provider.BalanceResult {
	c := currency
	r := provider.BalanceResult{
		Provider: name, Status: provider.StatusOK, Kind: provider.KindBalance,
		Currency: &c, Balance: &v, CheckedAt: time.Now().UTC(),
	}
	return r
}

func threshold(v float64) *config.Threshold { return &config.Threshold{Balance: &v} }

func TestAlertThenDedupe(t *testing.T) {
	sender := &fakeSender{}
	m := New(
		map[string]provider.Provider{
			"deepseek": &fakeProvider{"deepseek", bal("deepseek", "CNY", 8.0)},
		},
		map[string]config.Threshold{"deepseek": *threshold(10)},
		sender, state.New(""), false, 4,
	)

	ctx := context.Background()
	results := []provider.BalanceResult{bal("deepseek", "CNY", 8.0)}
	m.Evaluate(ctx, results) // below threshold -> alert
	if sender.count() != 1 {
		t.Fatalf("expected 1 alert, got %d", sender.count())
	}
	if !strings.Contains(sender.last(), "Alert") {
		t.Errorf("message should be an alert, got: %q", sender.last())
	}

	// again below threshold -> dedupe, no new message
	m.Evaluate(ctx, results)
	if sender.count() != 1 {
		t.Errorf("expected dedupe (still 1), got %d", sender.count())
	}
}

func TestRecoveryThenRealert(t *testing.T) {
	sender := &fakeSender{}
	m := New(
		map[string]provider.Provider{},
		map[string]config.Threshold{"deepseek": *threshold(10)},
		sender, state.New(""), true, 4,
	)
	ctx := context.Background()

	// below -> alert
	m.Evaluate(ctx, []provider.BalanceResult{bal("deepseek", "CNY", 8.0)})
	if sender.count() != 1 {
		t.Fatalf("want 1 after alert, got %d", sender.count())
	}

	// above -> recovery message (if recovery enabled)
	m.Evaluate(ctx, []provider.BalanceResult{bal("deepseek", "CNY", 15.0)})
	if sender.count() != 2 {
		t.Fatalf("want 2 after recovery, got %d", sender.count())
	}
	if !strings.Contains(sender.last(), "Recovery") {
		t.Errorf("message should be recovery, got %q", sender.last())
	}

	// below again -> re-alert (count 3)
	m.Evaluate(ctx, []provider.BalanceResult{bal("deepseek", "CNY", 6.0)})
	if sender.count() != 3 {
		t.Fatalf("want 3 after re-alert, got %d", sender.count())
	}
	if !strings.Contains(sender.last(), "Alert") {
		t.Errorf("message should be alert, got %q", sender.last())
	}
}

func TestNoThresholdNoAlert(t *testing.T) {
	sender := &fakeSender{}
	// no thresholds configured
	m := New(map[string]provider.Provider{}, map[string]config.Threshold{}, sender, state.New(""), false, 4)
	m.Evaluate(context.Background(), []provider.BalanceResult{bal("deepseek", "CNY", 1.0)})
	if sender.count() != 0 {
		t.Errorf("expected no alert without threshold, got %d", sender.count())
	}
}

func TestQuotaThreshold(t *testing.T) {
	sender := &fakeSender{}
	pct := 15.0
	r := provider.BalanceResult{
		Provider: "minimax", Status: provider.StatusOK, Kind: provider.KindQuota,
		RemainingPercent: &pct, CheckedAt: time.Now().UTC(),
	}
	pctTh := 20.0
	m := New(map[string]provider.Provider{}, map[string]config.Threshold{
		"minimax": {RemainingPercent: &pctTh},
	}, sender, state.New(""), false, 4)
	m.Evaluate(context.Background(), []provider.BalanceResult{r})
	if sender.count() != 1 {
		t.Errorf("expected 1 quota alert, got %d", sender.count())
	}
}

func TestCurrencyNotMixed(t *testing.T) {
	// balance in different currency with numeric equality should NOT trigger
	// if threshold is provider-scoped. Each provider's threshold is separate.
	sender := &fakeSender{}
	usd := bal("openrouter", "USD", 5.0)
	cny := bal("deepseek", "CNY", 10.0)
	m := New(map[string]provider.Provider{}, map[string]config.Threshold{
		// only deepseek has a CNY threshold of 10
		"deepseek": *threshold(10),
	}, sender, state.New(""), false, 4)
	m.Evaluate(context.Background(), []provider.BalanceResult{usd, cny})
	// deepseek 10 <= 10 -> alert
	if sender.count() != 1 {
		t.Errorf("expected 1 alert for deepseek, got %d", sender.count())
	}
}