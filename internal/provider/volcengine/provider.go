// Package volcengine implements the Volcengine Ark Coding Plan / Agent Plan
// quota providers via the Volcengine OpenAPI.
package volcengine

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

const (
	CodingPlanName = "volcengine-coding"
	AgentPlanName  = "volcengine-agent"
)

// Product selects which plan API to call.
type Product string

const (
	ProductCoding Product = "coding"
	ProductAgent  Product = "agent"
)

// Period is the quota window to report (session / weekly / monthly).
type Period string

const (
	PeriodSession Period = "session"
	PeriodWeekly  Period = "weekly"
	PeriodMonthly Period = "monthly"
)

// endpoint for each product.
func actionFor(p Product) string {
	if p == ProductAgent {
		return "GetAgentPlanAFPUsage"
	}
	return "GetCodingPlanUsage"
}

// volcProvider checks one product's quota for a given period window.
type volcProvider struct {
	ak       string
	sk       string
	product  Product
	period   Period
	endpoint string // injectable for tests
	timeout  time.Duration
}

// NewCoding builds the Coding Plan provider.
func NewCoding(opts provider.Options) (provider.Provider, error) {
	return New(ProductCoding, opts)
}

// NewAgent builds the Agent Plan provider.
func NewAgent(opts provider.Options) (provider.Provider, error) {
	return New(ProductAgent, opts)
}

// New builds a volcengine provider for a product; binds a period (default weekly).
func New(product Product, opts provider.Options) (provider.Provider, error) {
	if opts.AccessKey == "" || opts.SecretKey == "" {
		return nil, &provider.ProviderError{Status: provider.StatusAuthError, Detail: "volcengine: access_key and secret_access_key required"}
	}
	period := PeriodWeekly
	if v, ok := opts.Extra["period"].(string); ok && v != "" {
		period = Period(v)
	}
	ep := "https://" + host + "/"
	if e, ok := opts.Extra["endpoint"].(string); ok && e != "" {
		ep = e
	}
	return &volcProvider{ak: opts.AccessKey, sk: opts.SecretKey, product: product, period: period, endpoint: ep, timeout: opts.Timeout}, nil
}

func (p *volcProvider) Name() string {
	if p.product == ProductAgent {
		return AgentPlanName
	}
	return CodingPlanName
}

// Check returns the quota for the configured period window.
// On success: Kind=quota, RemainingPercent = 100 - usedPercent,
// ResetAt = reset timestamp.
func (p *volcProvider) Check(ctx context.Context) provider.BalanceResult {
	checkedAt := time.Now().UTC()
	timeout := p.timeout
	if timeout <= 0 {
		timeout = provider.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	action := actionFor(p.product)
	body := []byte("{}")
	now := time.Now()
	auth, bodyHash := signRequest(p.ak, p.sk, now, action, "2024-01-01", body)

	url2 := p.endpoint + "?Action=" + action + "&Version=2024-01-01"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url2, bytes.NewReader(body))
	if err != nil {
		return provider.ErrResult(p.Name(), provider.StatusUnknown, "build request failed", checkedAt)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", host)
	req.Header.Set("X-Content-Sha256", bodyHash)
	req.Header.Set("X-Date", now.UTC().Format("20060102T150405Z"))
	req.Header.Set("Authorization", auth)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return provider.ErrResult(p.Name(), provider.StatusTimeout, "request failed", checkedAt)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return provider.ErrResult(p.Name(), provider.StatusAuthError, "invalid access key", checkedAt)
	case resp.StatusCode == http.StatusTooManyRequests:
		return provider.ErrResult(p.Name(), provider.StatusRateLimited, "rate limited", checkedAt)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return provider.ErrResult(p.Name(), provider.StatusHTTPError, "HTTP "+strconv.Itoa(resp.StatusCode), checkedAt)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return provider.ErrResult(p.Name(), provider.StatusParseError, "read body failed", checkedAt)
	}

	// parse
	var envelope struct {
		ResponseMetadata struct {
			Error *struct {
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
		Result *struct {
			Status       string `json:"Status"`
			QuotaUsage   []struct {
				Level           string  `json:"Level"`
				Percent         float64 `json:"Percent"`
				ResetTimestamp  int64   `json:"ResetTimestamp"`
				Cap             float64 `json:"Cap"`
				RewardTotalPercent float64 `json:"RewardTotalPercent"`
			} `json:"QuotaUsage"`
			// Agent Plan variant
			AFPFiveHour *struct {
				Quota float64 `json:"Quota"`
				Used  float64 `json:"Used"`
			} `json:"AFPFiveHour"`
			AFPWeekly *struct {
				Quota float64 `json:"Quota"`
				Used  float64 `json:"Used"`
			} `json:"AFPWeekly"`
			AFPMonthly *struct {
				Quota float64 `json:"Quota"`
				Used  float64 `json:"Used"`
			} `json:"AFPMonthly"`
		} `json:"Result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return provider.ErrResult(p.Name(), provider.StatusParseError, "invalid JSON", checkedAt)
	}
	if envelope.ResponseMetadata.Error != nil {
		return provider.ErrResult(p.Name(), provider.StatusAuthError, "api error: "+envelope.ResponseMetadata.Error.Message, checkedAt)
	}
	if envelope.Result == nil {
		return provider.ErrResult(p.Name(), provider.StatusParseError, "missing Result", checkedAt)
	}

	if p.product == ProductAgent {
		// Agent Plan: used/total/quota in AFRewards; percent = used/quota*100.
		var w *struct {
			Quota float64 `json:"Quota"`
			Used  float64 `json:"Used"`
		}
		switch p.period {
		case PeriodSession, "5h":
			w = envelope.Result.AFPFiveHour
		case PeriodMonthly:
			w = envelope.Result.AFPMonthly
		default:
			w = envelope.Result.AFPWeekly
		}
		if w == nil {
			return provider.ErrResult(p.Name(), provider.StatusParseError, "period window not present", checkedAt)
		}
		used := w.Used
		total := w.Quota
		var pctUsed float64
		if total > 0 {
			pctUsed = used / total * 100
		}
		remaining := 100.0 - pctUsed
		if remaining < 0 {
			remaining = 0
		}
		return provider.BuildResult(p.Name(), provider.KindQuota, nil, nil, &used, &total, &remaining, nil, checkedAt)
	}

	// Coding Plan: QuotaUsage[] with Percent (used%) per Level.
	var found *struct {
		Level           string  `json:"Level"`
		Percent         float64 `json:"Percent"`
		ResetTimestamp  int64   `json:"ResetTimestamp"`
		Cap             float64 `json:"Cap"`
		RewardTotalPercent float64 `json:"RewardTotalPercent"`
	}
	for i := range envelope.Result.QuotaUsage {
		q := &envelope.Result.QuotaUsage[i]
		if Period(q.Level) == p.period {
			found = q
			break
		}
	}
	if found == nil {
		// fall back to first available
		if len(envelope.Result.QuotaUsage) > 0 {
			found = &envelope.Result.QuotaUsage[0]
		} else {
			return provider.ErrResult(p.Name(), provider.StatusParseError, "no quota usage returned (not subscribed to plan?)", checkedAt)
		}
	}
	remaining := 100.0 - found.Percent
	if remaining < 0 {
		remaining = 0
	}
	var reset *time.Time
	if found.ResetTimestamp != 0 {
		rt := time.Unix(found.ResetTimestamp, 0).UTC()
		reset = &rt
	}
	used := found.Percent // percent used (no absolute)
	return provider.BuildResult(p.Name(), provider.KindQuota, nil, nil, &used, nil, &remaining, reset, checkedAt)
}