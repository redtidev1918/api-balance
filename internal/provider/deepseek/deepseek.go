// Package deepseek implements the DeepSeek balance provider.
// Endpoint: GET https://api.deepseek.com/user/balance (official, documented).
// Status: stable.
package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

const (
	Name     = "deepseek"
	endpoint = "https://api.deepseek.com/user/balance"
)

type balanceInfo struct {
	Currency       string `json:"currency"`
	TotalBalance   string `json:"total_balance"`
	GrantedBalance string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

type balanceResponse struct {
	IsAvailable  *bool         `json:"is_available"`
	BalanceInfos []balanceInfo `json:"balance_infos"`
}

// balanceProvider is a DeepSeek balance checker.
type balanceProvider struct {
	key     string
	timeout time.Duration
	// endpoint is the base URL; defaults to the official endpoint but is
	// injectable for tests.
	endpoint string
}

func New(opts provider.Options) (provider.Provider, error) {
	if opts.APIKey == "" {
		return nil, &provider.ProviderError{Status: provider.StatusAuthError, Detail: "deepseek: empty API key"}
	}
	ep := endpoint
	if v, ok := opts.Extra["endpoint"].(string); ok && v != "" {
		ep = v
	}
	return &balanceProvider{key: opts.APIKey, timeout: opts.Timeout, endpoint: ep}, nil
}

func (p *balanceProvider) Name() string { return Name }

func (p *balanceProvider) Check(ctx context.Context) provider.BalanceResult {
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

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
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
	var br balanceResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return provider.ErrResult(Name, provider.StatusParseError, "invalid JSON", checkedAt)
	}
	if len(br.BalanceInfos) == 0 {
		return provider.ErrResult(Name, provider.StatusParseError, "no balance info", checkedAt)
	}
	info := br.BalanceInfos[0]
	total, err := strconv.ParseFloat(info.TotalBalance, 64)
	if err != nil {
		return provider.ErrResult(Name, provider.StatusParseError, "bad balance value", checkedAt)
	}
	currency := info.Currency
	if currency == "" {
		currency = "CNY"
	} else {
		currency = strings.ToUpper(currency)
	}
	return provider.BuildResult(Name, provider.KindBalance, &currency, &total, nil, nil, nil, nil, checkedAt)
}