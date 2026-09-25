// Package minimax implements the MiniMax quota provider (quota, not balance).
// Endpoint: GET https://api.minimaxi.com/v1/token_plan/remains (undocumented,
// response shape has changed across versions). Status: experimental.
package minimax

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
	Name          = "minimax"
	cnEndpoint    = "https://api.minimaxi.com/v1/token_plan/remains"
	globalEndpoint = "https://api.minimax.io/v1/token_plan/remains"
)

type baseResp struct {
	StatusCode *int   `json:"status_code"`
	StatusMsg  string `json:"status_msg"`
}

type modelRemain struct {
	ModelName                       string  `json:"model_name"`
	CurrentIntervalTotalCount       *float64 `json:"current_interval_total_count"`
	CurrentIntervalRemainingPercent *float64 `json:"current_interval_remaining_percent"`
	CurrentWeeklyTotalCount         *float64 `json:"current_weekly_total_count"`
	CurrentWeeklyRemainingPercent   *float64 `json:"current_weekly_remaining_percent"`
}

type remainsResponse struct {
	BaseResp    baseResp     `json:"base_resp"`
	ModelRemains []modelRemain `json:"model_remains"`
}

type quotaProvider struct {
	key     string
	timeout time.Duration
	endpoint string
}

func New(opts provider.Options) (provider.Provider, error) {
	if opts.APIKey == "" {
		return nil, &provider.ProviderError{Status: provider.StatusAuthError, Detail: "minimax: empty API key"}
	}
	ep := cnEndpoint
	if v, ok := opts.Extra["endpoint"].(string); ok && v != "" {
		ep = v
	}
	return &quotaProvider{key: opts.APIKey, timeout: opts.Timeout, endpoint: ep}, nil
}

func (p *quotaProvider) Name() string { return Name }

func (p *quotaProvider) Check(ctx context.Context) provider.BalanceResult {
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
	var rr remainsResponse
	if err := json.Unmarshal(body, &rr); err != nil {
		return provider.ErrResult(Name, provider.StatusParseError, "invalid JSON", checkedAt)
	}

	// Business auth errors surface via base_resp.status_code.
	if rr.BaseResp.StatusCode != nil {
		sc := *rr.BaseResp.StatusCode
		if sc == 1004 || sc == 1008 || sc == 10013 || sc == 10014 {
			return provider.ErrResult(Name, provider.StatusAuthError, "api auth code="+strconv.Itoa(sc), checkedAt)
		}
	}

	// Prefer a coding-capable model row (MiniMax-M); fall back to first row.
	var pick *modelRemain
	for i := range rr.ModelRemains {
		m := &rr.ModelRemains[i]
		if contains(m.ModelName, "MiniMax-M") {
			pick = m
			break
		}
	}
	if pick == nil && len(rr.ModelRemains) > 0 {
		pick = &rr.ModelRemains[0]
	}
	if pick == nil {
		return provider.ErrResult(Name, provider.StatusParseError, "no model remains", checkedAt)
	}
	if pick.CurrentIntervalRemainingPercent == nil {
		return provider.ErrResult(Name, provider.StatusParseError, "missing remaining percent", checkedAt)
	}
	pct := *pick.CurrentIntervalRemainingPercent

	// Remaining % interpreted directly as quota remaining.
	result := provider.BuildResult(Name, provider.KindQuota, nil, nil, nil, nil, &pct, nil, checkedAt)
	return result
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}