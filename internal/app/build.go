// Package app wires config + registry into concrete, runnable providers.
// It is the one place that knows how a config.ProviderConfig becomes a
// provider.Provider. The CLI and monitor both call into it.
package app

import (
	"fmt"
	"time"

	"github.com/redtidev1918/api-balance/internal/config"
	"github.com/redtidev1918/api-balance/internal/provider"
	"github.com/redtidev1918/api-balance/internal/provider/custom"
	"github.com/redtidev1918/api-balance/internal/provider/deepseek"
	"github.com/redtidev1918/api-balance/internal/provider/minimax"
	"github.com/redtidev1918/api-balance/internal/provider/moonshot"
	"github.com/redtidev1918/api-balance/internal/provider/openrouter"
	"github.com/redtidev1918/api-balance/internal/provider/siliconflow"
	"github.com/redtidev1918/api-balance/internal/provider/volcengine"
)

// BuiltinNames lists natives with their stability for the `providers` command.
var BuiltinNames = map[string]provider.Stability{
	deepseek.Name:            provider.StabilityStable,
	openrouter.Name:          provider.StabilityStable,
	siliconflow.Name:         provider.StabilityStable,
	moonshot.Name:            provider.StabilityStable,
	minimax.Name:             provider.StabilityExperimental,
	volcengine.CodingPlanName: provider.StabilityExperimental,
	volcengine.AgentPlanName:  provider.StabilityExperimental,
}

// BuildProviders turns configuration into a set of runnable providers.
// Only providers present in cfg are built; built-in names not configured are
// skipped. Returns a map keyed by the config provider name.
func BuildProviders(cfg *config.Config, timeout time.Duration) (map[string]provider.Provider, error) {
	out := map[string]provider.Provider{}

	for name, pc := range cfg.Providers {
		key := config.KeyFor(pc)
		switch {
		case pc.Type == "custom":
			spec := custom.Spec{
				Name:     name,
				Endpoint: pc.Endpoint,
				Method:   pc.Method,
				Auth:     authMap(pc.Auth),
				Headers:  pc.Headers,
				Query:    pc.Query,
				Body:     pc.Body,
				Balance:  pc.Extract.Balance,
				Currency: pc.Extract.Currency,
				Used:     pc.Extract.Used,
				Total:    pc.Extract.Total,
				Remaining: pc.Extract.Remaining,
				ResetAt:  pc.Extract.ResetAt,
				StatusPath: pc.Extract.StatusPath,
				StatusOK: pc.Extract.StatusOK,
			}
			out[name] = custom.New(spec, key, timeout)

		case name == deepseek.Name:
			out[name] = mustBuild(deepseek.New, provider.Options{Name: name, APIKey: key, Timeout: timeout})
		case name == openrouter.Name:
			out[name] = mustBuild(openrouter.New, provider.Options{Name: name, APIKey: key, Timeout: timeout})
		case name == siliconflow.Name:
			out[name] = mustBuild(siliconflow.New, provider.Options{Name: name, APIKey: key, Timeout: timeout, Extra: extraFrom(pc)})
		case name == moonshot.Name:
			out[name] = mustBuild(moonshot.New, provider.Options{Name: name, APIKey: key, Timeout: timeout, Extra: extraFrom(pc)})
		case name == minimax.Name:
			out[name] = mustBuild(minimax.New, provider.Options{Name: name, APIKey: key, Timeout: timeout, Extra: extraFrom(pc)})

		case name == volcengine.CodingPlanName:
			out[name] = mustBuild(volcengine.NewCoding, provider.Options{
				Name: name, AccessKey: config.AccessKeyFor(pc), SecretKey: config.SecretAccessKeyFor(pc),
				Timeout: timeout, Extra: extraFrom(pc),
			})
		case name == volcengine.AgentPlanName:
			out[name] = mustBuild(volcengine.NewAgent, provider.Options{
				Name: name, AccessKey: config.AccessKeyFor(pc), SecretKey: config.SecretAccessKeyFor(pc),
				Timeout: timeout, Extra: extraFrom(pc),
			})

		case pc.Type != "":
			return nil, fmt.Errorf("provider %q has unknown type %q", name, pc.Type)
		default:
			return nil, fmt.Errorf("provider %q is not a built-in provider", name)
		}
	}
	return out, nil
}

// mustBuild panics only on programming error (builder validates key presence).
func mustBuild(newFn func(provider.Options) (provider.Provider, error), opts provider.Options) provider.Provider {
	p, err := newFn(opts)
	if err != nil {
		panic("api-balance: " + err.Error())
	}
	return p
}

// extraFrom passes through any provider-specific settings.
// Reserved for future provider knobs; currently empty.
func extraFrom(pc config.ProviderConfig) map[string]interface{} {
	_ = pc
	return map[string]interface{}{}
}

// authMap converts config.CustomAuth to the opaque map custom.Spec expects.
func authMap(a config.CustomAuth) map[string]interface{} {
	m := map[string]interface{}{}
	if a.Type != "" {
		m["type"] = a.Type
	}
	if a.TokenEnv != "" {
		m["token_env"] = a.TokenEnv
	}
	if a.HeaderName != "" {
		m["header_name"] = a.HeaderName
	}
	if a.QueryName != "" {
		m["query_name"] = a.QueryName
	}
	for k, v := range a.Credentials {
		m[k] = v
	}
	return m
}