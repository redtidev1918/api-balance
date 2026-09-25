// Package siliconflow implements the SiliconFlow balance provider.
// Endpoint: GET https://api.siliconflow.cn/v1/user/info (community-documented),
// override with Extra["endpoint"] to use the international https://api.siliconflow.com/v1/user/info.
// Status: stable.
package siliconflow

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
	Name = "siliconflow"
	cnEndpoint   = "https://api.siliconflow.cn/v1/user/info"
	globalEndpoint = "https://api.siliconflow.com/v1/user/info"
)

type userData struct {
	Balance        *float64 `json:"balance"`
	TotalBalance   *float64 `json:"totalBalance"`
	ChargeBalance  *float64 `json:"chargeBalance"`
	TotalUsage     *float64 `json:"totalUsage"`
}

type userResponse struct {
	Data userData `json:"data"`
}

type userProvider struct {
	key     string
	timeout time.Duration
	endpoint string
}

func New(opts provider.Options) (provider.Provider, error) {
	if opts.APIKey == "" {
		return nil, &provider.ProviderError{Status: provider.StatusAuthError, Detail: "siliconflow: empty API key"}
	}
	ep := cnEndpoint
	if v, ok := opts.Extra["endpoint"].(string); ok && v != "" {
		ep = v
	}
	return &userProvider{key: opts.APIKey, timeout: opts.Timeout, endpoint: ep}, nil
}

func (p *userProvider) Name() string { return Name }

func (p *userProvider) Check(ctx context.Context) provider.BalanceResult {
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
	var ur userResponse
	if err := json.Unmarshal(body, &ur); err != nil {
		return provider.ErrResult(Name, provider.StatusParseError, "invalid JSON", checkedAt)
	}

	// Prefer data.balance (committed spendable), fall back to chargeBalance, totalBalance.
	bal := ur.Data.Balance
	has := false
	if bal != nil {
		has = true
	} else if ur.Data.ChargeBalance != nil {
		bal = ur.Data.ChargeBalance
		has = true
	} else if ur.Data.TotalBalance != nil {
		bal = ur.Data.TotalBalance
		has = true
	}
	if !has {
		return provider.ErrResult(Name, provider.StatusParseError, "missing balance", checkedAt)
	}
	cny := "CNY"
	return provider.BuildResult(Name, provider.KindBalance, &cny, bal, ur.Data.TotalUsage, nil, nil, nil, checkedAt)
}