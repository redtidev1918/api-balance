// Package moonshot implements the Moonshot / Kimi balance provider.
// Endpoint: GET https://api.moonshot.cn/v1/users/me/balance (community-documented).
// Status: stable.
package moonshot

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
	Name   = "moonshot"
	cnEndpoint   = "https://api.moonshot.cn/v1/users/me/balance"
	globalEndpoint = "https://api.moonshot.ai/v1/users/me/balance"
)

type balanceData struct {
	AvailableBalance *float64 `json:"available_balance"`
	VoucherBalance   *float64 `json:"voucher_balance"`
	CashBalance      *float64 `json:"cash_balance"`
	TotalBalance     *float64 `json:"total_balance"`
}

type balanceResponse struct {
	Code *int         `json:"code"`
	Data balanceData  `json:"data"`
}

type balanceProvider struct {
	key      string
	timeout  time.Duration
	endpoint string
}

func New(opts provider.Options) (provider.Provider, error) {
	if opts.APIKey == "" {
		return nil, &provider.ProviderError{Status: provider.StatusAuthError, Detail: "moonshot: empty API key"}
	}
	ep := cnEndpoint
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
	var br balanceResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return provider.ErrResult(Name, provider.StatusParseError, "invalid JSON", checkedAt)
	}

	// Some responses carry a top-level code (0=ok). Non-zero code => error.
	if br.Code != nil && *br.Code != 0 {
		return provider.ErrResult(Name, provider.StatusUnknown, "api code="+strconv.Itoa(*br.Code), checkedAt)
	}

	bal := br.Data.AvailableBalance
	if bal == nil {
		bal = br.Data.TotalBalance
	}
	if bal == nil {
		bal = br.Data.CashBalance
	}
	if bal == nil {
		return provider.ErrResult(Name, provider.StatusParseError, "missing balance", checkedAt)
	}
	cny := "CNY"
	return provider.BuildResult(Name, provider.KindBalance, &cny, bal, nil, nil, nil, nil, checkedAt)
}