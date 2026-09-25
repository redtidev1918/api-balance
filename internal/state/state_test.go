package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnterExitTransition(t *testing.T) {
	s := New("")
	// first Enter -> true (new alert)
	if !s.Enter("deepseek") {
		t.Error("first Enter should return true")
	}
	// second Enter -> false (dedupe)
	if s.Enter("deepseek") {
		t.Error("second Enter should return false")
	}
	if !s.IsActive("deepseek") {
		t.Error("should be active")
	}
	// Exit -> true (recovery)
	if !s.Exit("deepseek") {
		t.Error("Exit should return true when it was active")
	}
	// Exit again -> false
	if s.Exit("deepseek") {
		t.Error("Exit again should return false")
	}
	if s.IsActive("deepseek") {
		t.Error("should not be active after exit")
	}
}

func TestExitWhenNotActive(t *testing.T) {
	s := New("")
	if s.Exit("x") {
		t.Error("Exit on inactive should return false")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "state.json")

	s1 := New(file)
	s1.Enter("deepseek")
	s1.Enter("openrouter")

	// reload from disk
	s2 := New(file)
	if !s2.IsActive("deepseek") {
		t.Error("deepseek should persist as active")
	}
	if !s2.IsActive("openrouter") {
		t.Error("openrouter should persist as active")
	}
	if s2.IsActive("nonexistent") {
		t.Error("nonexistent should not be active")
	}

	// verify file perms are 0600 (secrets sensitivity: state has no secrets,
	// but keeping tight perms is good hygiene)
	fi, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("state file perms = %o, want 600", fi.Mode().Perm())
	}
}

func TestReset(t *testing.T) {
	s := New("")
	s.Enter("a")
	s.Reset()
	if s.IsActive("a") {
		t.Error("Reset should clear all")
	}
}