package provider

import (
	"time"
)

// DefaultTimeout is the default per-provider HTTP request timeout. Individual
// providers fall back to this when the caller does not supply a shorter one.
const DefaultTimeout = 10 * time.Second

// BuildResult wraps a successful value into a fully-populated BalanceResult.
func BuildResult(name string, kind Kind, currency *string, balance, used, total *float64, remainingPct *float64, resetAt *time.Time, checkedAt time.Time) BalanceResult {
	return BalanceResult{
		Provider:         name,
		Status:           StatusOK,
		Kind:             kind,
		Currency:         currency,
		Balance:          balance,
		Used:             used,
		Total:            total,
		RemainingPercent: remainingPct,
		ResetAt:          resetAt,
		CheckedAt:        checkedAt,
	}
}

// ErrResult wraps a classified failure into a BalanceResult. It deliberately
// carries only the classified Status and a short, URL-free, secret-free detail
// so the result is always safe to log and display.
func ErrResult(name string, status Status, detail string, checkedAt time.Time) BalanceResult {
	return BalanceResult{
		Provider:  name,
		Status:    status,
		Kind:      "",
		CheckedAt: checkedAt,
		Error:     &ProviderError{Status: status, Detail: detail},
	}
}