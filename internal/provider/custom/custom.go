// Package custom implements the declarative custom provider.
// Users define endpoint, auth, and JMESPath extractors in config; no recompile.
package custom

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jmespath/go-jmespath"
	"github.com/redtidev1918/api-balance/internal/provider"
)

// Spec is the declarative custom-provider definition (mirrors config shape).
type Spec struct {
	Name     string
	Endpoint string
	Method   string
	// auth
	Auth     map[string]interface{} // type, token_env, header_name, query_name, credentials
	Headers  map[string]string
	Query    map[string]string
	Body     map[string]interface{}
	// extractors (JMESPath into response JSON)
	Balance     string
	Currency    string
	Used        string
	Total       string
	Remaining   string
	ResetAt     string
	StatusPath  string
	StatusOK    string
}

type customProvider struct {
	spec     Spec
	apiKey   string
	timeout  time.Duration
}

func New(spec Spec, apiKey string, timeout time.Duration) provider.Provider {
	return &customProvider{spec: spec, apiKey: apiKey, timeout: timeout}
}

func (p *customProvider) Name() string { return p.spec.Name }

func (p *customProvider) Check(ctx context.Context) provider.BalanceResult {
	checkedAt := time.Now().UTC()
	timeout := p.timeout
	if timeout <= 0 {
		timeout = provider.DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := p.buildRequest(ctx)
	if err != nil {
		return provider.ErrResult(p.Name(), provider.StatusUnknown, "build request failed", checkedAt)
	}

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return provider.ErrResult(p.Name(), provider.StatusTimeout, "request failed", checkedAt)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return provider.ErrResult(p.Name(), provider.StatusAuthError, "invalid credentials", checkedAt)
	case resp.StatusCode == http.StatusTooManyRequests:
		return provider.ErrResult(p.Name(), provider.StatusRateLimited, "rate limited", checkedAt)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return provider.ErrResult(p.Name(), provider.StatusHTTPError, "HTTP "+strconv.Itoa(resp.StatusCode), checkedAt)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return provider.ErrResult(p.Name(), provider.StatusParseError, "read body failed", checkedAt)
	}

	// Parse JSON into a generic structure for JMESPath.
	var doc interface{}
	if err := json.Unmarshal(body, &doc); err != nil {
		return provider.ErrResult(p.Name(), provider.StatusParseError, "invalid JSON", checkedAt)
	}

	// Optional status check.
	if p.spec.StatusPath != "" {
		okVal := p.spec.StatusOK
		if okVal == "" {
			okVal = "ok"
		}
		got := p.extractString(p.spec.StatusPath, doc)
		if got != "" && !strings.EqualFold(got, okVal) {
			return provider.ErrResult(p.Name(), provider.StatusUnknown, "status="+got, checkedAt)
		}
	}

	// Validate at least one extractor present.
	if p.spec.Balance == "" && p.spec.Used == "" && p.spec.Total == "" && p.spec.Remaining == "" {
		// Treated as config error.
		return provider.ErrResult(p.Name(), provider.StatusUnsupported, "custom: no extractor configured", checkedAt)
	}

	balance := p.extractFloat(p.spec.Balance, doc)
	used := p.extractFloat(p.spec.Used, doc)
	total := p.extractFloat(p.spec.Total, doc)
	remaining := p.extractFloat(p.spec.Remaining, doc)

	var currencyStr *string
	if c := p.extractString(p.spec.Currency, doc); c != "" {
		currencyStr = &c
	}
	reset := p.extractTime(p.spec.ResetAt, doc)

	// Determine kind: if a balance extractor is set => balance; else quota/usage.
	switch {
	case p.spec.Balance != "":
		return provider.BuildResult(p.Name(), provider.KindBalance, currencyStr, &balance, nil, nil, nil, reset, checkedAt)
	case p.spec.Remaining != "" && p.spec.Total != "":
		// remaining + total => percentage
		pct := 0.0
		if total > 0 {
			pct = (remaining / total) * 100
		}
		return provider.BuildResult(p.Name(), provider.KindQuota, nil, nil, &used, &total, &pct, reset, checkedAt)
	case p.spec.Used != "" && p.spec.Total != "":
		return provider.BuildResult(p.Name(), provider.KindUsage, nil, nil, &used, &total, nil, reset, checkedAt)
	default:
		return provider.ErrResult(p.Name(), provider.StatusUnsupported, "custom: cannot map extractors to a kind", checkedAt)
	}
}

func (p *customProvider) buildRequest(ctx context.Context) (*http.Request, error) {
	method := p.spec.Method
	if method == "" {
		method = http.MethodGet
	}
	u := p.spec.Endpoint
	// append query params (URL-encoded via url.Values)
	if len(p.spec.Query) > 0 {
		q := url.Values{}
		for k, v := range p.spec.Query {
			q.Set(k, v)
		}
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + q.Encode()
	}

	var bodyReader io.Reader
	if p.spec.Body != nil {
		b, _ := json.Marshal(p.spec.Body)
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return nil, err
	}

	// default headers
	if p.spec.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range p.spec.Headers {
		req.Header.Set(k, v)
	}

	// auth
	switch authType(p.spec.Auth) {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	case "header":
		name := mapString(p.spec.Auth, "header_name")
		if name == "" {
			name = "Authorization"
		}
		req.Header.Set(name, p.apiKey)
	case "query":
		name := mapString(p.spec.Auth, "query_name")
		if name != "" {
			q := req.URL.Query()
			q.Set(name, p.apiKey)
			req.URL.RawQuery = q.Encode()
		}
	case "basic":
		user := mapString(p.spec.Auth, "username")
		pass := mapString(p.spec.Auth, "password")
		req.SetBasicAuth(user, pass)
	}
	// default to bearer if auth type empty but key present
	if p.apiKey != "" && req.Header.Get("Authorization") == "" && authType(p.spec.Auth) == "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

func authType(a map[string]interface{}) string {
	if a == nil {
		return ""
	}
	if v, ok := a["type"].(string); ok {
		return strings.ToLower(v)
	}
	return ""
}

func mapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// extractFloat evaluates a JMESPath expr; returns 0 if absent/invalid.
func (p *customProvider) extractFloat(expr string, doc interface{}) float64 {
	if expr == "" {
		return 0
	}
	v, err := jmespath.Search(expr, doc)
	if err != nil || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

func (p *customProvider) extractString(expr string, doc interface{}) string {
	if expr == "" {
		return ""
	}
	v, err := jmespath.Search(expr, doc)
	if err != nil || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func (p *customProvider) extractTime(expr string, doc interface{}) *time.Time {
	if expr == "" {
		return nil
	}
	v, err := jmespath.Search(expr, doc)
	if err != nil || v == nil {
		return nil
	}
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case int:
		s = strconv.FormatInt(int64(t), 10)
	case float64:
		s = strconv.FormatInt(int64(t), 10)
	default:
		return nil
	}
	// Try RFC3339, then RFC3339Nano, then unix seconds.
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	// epoch seconds
	if secs, err := strconv.ParseInt(s, 10, 64); err == nil && secs > 1e9 {
		t := time.Unix(secs, 0).UTC()
		return &t
	}
	if secs, err := strconv.ParseInt(s, 10, 64); err == nil && secs < 1e9 {
		t := time.Unix(secs, 0).UTC()
		return &t
	}
	return nil
}