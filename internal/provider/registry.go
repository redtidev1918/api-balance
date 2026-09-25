package provider

import (
	"fmt"
	"os"
	"time"
)

// Registry is the source of truth for wiring configured provider names to
// concrete implementations. It is populated by each provider package's
// Register, then consumed by the CLI/monitor.
type Registry struct {
	builders map[string]Builder
}

// Builder constructs a provider from its metadata + config + resolved key.
// Config is deliberately an opaque map so native providers can receive their
// own settings without the registry knowing their shape.
type Builder func(opts Options) (Provider, error)

// Options is the normalized input to Builder.
type Options struct {
	Name       string
	Kind       Kind
	Stability  Stability
	APIKey     string
	AccessKey  string // Volcengine AccessKeyId
	SecretKey  string // Volcengine SecretAccessKey
	Extra      map[string]interface{} // provider-specific settings
	Timeout    time.Duration
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{builders: map[string]Builder{}}
}

// Register binds a provider name to its builder.
func (r *Registry) Register(name string, b Builder) {
	if r.builders == nil {
		r.builders = map[string]Builder{}
	}
	r.builders[name] = b
}

// Builtin exposes the built-in names (for `providers` command).
func (r *Registry) Builtin() []string {
	out := make([]string, 0, len(r.builders))
	for k := range r.builders {
		out = append(out, k)
	}
	return out
}

// Has reports whether a name is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.builders[name]
	return ok
}

// Build instantiates a provider by name. Returns ErrNotRegistered.
func (r *Registry) Build(name string, opts Options) (Provider, error) {
	b, ok := r.builders[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not registered (built-in: %v)", name, r.Builtin())
	}
	return b(opts)
}

// ErrNotRegistered is returned by Build for unknown names.
var ErrNotRegistered = fmt.Errorf("provider not registered")

// ---- result helpers ----

// RemainingPercentOf computes remaining % = remaining/total*100.
func RemainingPercentOf(remaining, total float64) float64 {
	if total == 0 {
		return 0
	}
	return (remaining / total) * 100
}

// formattedBalance renders a balance text for a result (used by watch summary).
func formattedBalance(r BalanceResult) string {
	if r.Status != StatusOK {
		return string(r.Status)
	}
	return FormatValue(r)
}

// FormatValue renders a successful result as text. (duplicate of output.FormatValue
// kept here so provider package has a standalone impl the manager can use.)
func FormatValue(r BalanceResult) string {
	if r.Currency != nil && r.Balance != nil {
		return fmt.Sprintf("%s %.2f", *r.Currency, *r.Balance)
	}
	if r.RemainingPercent != nil {
		return fmt.Sprintf("%.0f%% remaining", *r.RemainingPercent)
	}
	return ""
}

// envOr returns env var by name or fallback.
func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}