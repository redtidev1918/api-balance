// Package openrouter implements the OpenRouter credit provider.
// Endpoint: GET https://openrouter.ai/api/v1/key (community-documented).
// Status: stable.
package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

const (
	Name     = "openrouter"
	endpoint = "https://openrouter.ai/api/v1/key"
)

type keyData struct {
	Label         string  `json:"label"`
	Limit         *float64 `json:"limit"`
	LimitRemaining *float64 `json:"limit_remaining"`
	Usage         *float64 `json:"usage"`
	IsFreeTier    *bool   `json:"is_free_tier"`
}

type keyResponse struct {
	Data keyData `json:"data"`
}

type creditProvider struct {
	key     string
	timeout time.Duration
	endpoint string
}

func New(opts provider.Options) (provider.Provider, error) {
	if opts.APIKey == "" {
		return nil, &provider.ProviderError{Status: provider.StatusAuthError, Detail: "openrouter: empty API key"}
	}
	ep := endpoint
	if v, ok := opts.Extra["endpoint"].(string); ok && v != "" {
		ep = v
	}
	return &creditProvider{key: opts.APIKey, timeout: opts.Timeout, endpoint: ep}, nil
}

func (p *creditProvider) Name() string { return Name }

func (p *creditProvider) Check(ctx context.Context) provider.BalanceResult {
	checkedAt := time.Now().UTC()
	timeout := p.timeout
	if timeout <= 0 {
		timeout = provider.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return provider.ErrResult(Name, provider.StatusUnknown, "build request failed", checkedAt)
	}
	req.Header.Set("Authorization", "Bearer "+p.key)
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return provider.ErrResult(Name, provider.StatusTimeout, "request failed", checkedAt)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return provider.ErrResult(Name, provider.StatusAuthError, "invalid API key", checkedAt)
	case resp.StatusCode == http.StatusTooManyRequests:
		return provider.ErrResult(Name, provider.StatusRateLimited, "rate limited", checkedAt)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return provider.ErrResult(Name, provider.StatusHTTPError, "HTTP "+strconv.Itoa(resp.StatusCode), checkedAt)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return provider.ErrResult(Name, provider.StatusParseError, "read body failed", checkedAt)
	}
	var kr keyResponse
	if err := json.Unmarshal(body, &kr); err != nil {
		return provider.ErrResult(Name, provider.StatusParseError, "invalid JSON", checkedAt)
	}

	// Prefer limit_remaining; fall back to limit - usage when available.
	var balance float64
	has := false
	d := kr.Data
	if d.LimitRemaining != nil {
		balance = *d.LimitRemaining
		has = true
	} else if d.Limit != nil && d.Usage != nil {
		balance = *d.Limit - *d.Usage
		has = true
	}
	if !has {
		return provider.ErrResult(Name, provider.StatusParseError, "missing balance fields", checkedAt)
	}
	usd := "USD"
	return provider.BuildResult(Name, provider.KindBalance, &usd, &balance, nil, nil, nil, nil, checkedAt)
}