package provider

import (
	"context"
	"sync"
)

// DefaultConcurrency caps concurrent provider checks.
const DefaultConcurrency = 5

// Manager runs many provider checks concurrently with a bounded concurrency.
type Manager struct {
	concurrency int
}

// NewManager builds a manager with the given concurrency cap.
func NewManager(concurrency int) *Manager {
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	return &Manager{concurrency: concurrency}
}

// CheckAll runs every provider concurrently, returning results in name order.
// A single provider timeout/failure does not affect the others.
func (m *Manager) CheckAll(ctx context.Context, providers map[string]Provider) []BalanceResult {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	results := make([]BalanceResult, len(names))
	sem := make(chan struct{}, m.concurrency)
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = providers[name].Check(ctx)
		}(i, name)
	}
	wg.Wait()
	return results
}