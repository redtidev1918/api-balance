// Package provider defines the core abstractions for balance/quota providers.
// The CLI, monitor, and notification layers only depend on this package —
// they never see provider-specific HTTP/JSON details.
package provider

import (
	"context"
	"time"
)

// Status is the categorical outcome of a provider check.
type Status string

const (
	StatusOK          Status = "ok"
	StatusAuthError   Status = "auth_error"
	StatusRateLimited Status = "rate_limited"
	StatusTimeout     Status = "timeout"
	StatusHTTPError   Status = "http_error"
	StatusParseError  Status = "parse_error"
	StatusUnsupported Status = "unsupported"
	StatusUnknown     Status = "unknown_error"
)

// Kind distinguishes what a result actually represents.
// A balance (spendable money) is NOT the same as a quota (usage allowance).
type Kind string

const (
	KindBalance Kind = "balance"
	KindQuota   Kind = "quota"
	KindUsage   Kind = "usage"
)

// Stability marks how reliable a provider's endpoint is.
type Stability string

const (
	StabilityStable       Stability = "stable"       // public, documented, versioned
	StabilityExperimental Stability = "experimental" // undocumented/private, may break
	StabilityUnsupported  Stability = "unsupported"  // cannot be reliably queried
)

// ProviderError is a compact, loggable error. It deliberately carries no URL
// and no Authorization details so it is safe to print.
type ProviderError struct {
	Status Status
	Detail string
}

func (e *ProviderError) Error() string { return string(e.Status) + ": " + e.Detail }

// BalanceResult is the unified outcome of a single provider check.
// Pointer fields are nil when unknown — a provider may not report everything.
type BalanceResult struct {
	Provider         string         `json:"provider"`
	Status           Status         `json:"status"`
	Kind             Kind           `json:"kind"`
	Currency         *string        `json:"currency,omitempty"`
	Balance          *float64       `json:"balance,omitempty"`
	Used             *float64       `json:"used,omitempty"`
	Total            *float64       `json:"total,omitempty"`
	RemainingPercent *float64       `json:"remaining_percent,omitempty"`
	ResetAt          *time.Time     `json:"reset_at,omitempty"`
	CheckedAt        time.Time      `json:"checked_at"`
	Error            *ProviderError `json:"error,omitempty"`
}

// Metadata describes a provider's identity and stability.
type Metadata struct {
	Name      string
	Kind      Kind
	Stability Stability
}

// Provider is the single interface every native and custom provider implements.
type Provider interface {
	Name() string
	Check(ctx context.Context) BalanceResult
}