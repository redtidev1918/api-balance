// Package state tracks per-provider alert state for alert deduplication and
// recovery detection. Persisted to a JSON file; no database required.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AlertState records whether a provider is currently in an alert condition.
type AlertState struct {
	mu      sync.Mutex
	active  map[string]bool // provider -> currently below threshold
	file    string
}

// New loads state from file (if any) and returns a manager.
func New(file string) *AlertState {
	s := &AlertState{active: map[string]bool{}, file: file}
	if file != "" {
		s.load()
	}
	return s
}

// IsActive reports whether the provider is currently flagged.
func (s *AlertState) IsActive(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[name]
}

// Enter marks a provider as alerting. Returns true the first time (transition),
// false for repeats (dedupe).
func (s *AlertState) Enter(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[name] {
		return false // already alerting, don't re-notify
	}
	s.active[name] = true
	s.save()
	return true
}

// Exit clears a provider's alert flag. Returns true if it was previously active
// (i.e. a recovery happened).
func (s *AlertState) Exit(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active[name] {
		return false
	}
	delete(s.active, name)
	s.save()
	return true
}

// Reset clears all alert state (used by tests / recovery reset).
func (s *AlertState) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = map[string]bool{}
	s.save()
}

func (s *AlertState) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	var m map[string]bool
	if err := json.Unmarshal(data, &m); err != nil {
		return
	}
	if m != nil {
		s.active = m
	}
}

func (s *AlertState) save() {
	if s.file == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.file), 0o700); err != nil {
		return
	}
	data, err := json.MarshalIndent(s.active, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(s.file, data, 0o600)
}

// LastCheck is a lightweight timestamp helper for future use.
type LastCheck struct {
	CheckedAt time.Time `json:"checked_at"`
}