package minimax

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

func newTest(t *testing.T, handler http.HandlerFunc) *quotaProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &quotaProvider{key: "k", timeout: 3 * time.Second, endpoint: srv.URL}
}

func TestCheckOK(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth=%q", got)
		}
		w.Write([]byte(`{
			"base_resp":{"status_code":0,"status_msg":"success"},
			"model_remains":[
				{"model_name":"MiniMax-M2","current_interval_total_count":100000,"current_interval_remaining_percent":23},
				{"model_name":"abab6.5","current_interval_total_count":50000,"current_interval_remaining_percent":80}
			]
		}`))
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status=%q %+v", r.Status, r.Error)
	}
	// Should prefer MiniMax-M row -> 23%
	if r.RemainingPercent == nil || *r.RemainingPercent != 23 {
		t.Errorf("remaining=%v want 23", r.RemainingPercent)
	}
	if r.Kind != provider.KindQuota {
		t.Errorf("kind=%q want quota", r.Kind)
	}
}

func TestFallsBackToFirstRow(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"base_resp":{"status_code":0},
			"model_remains":[{"model_name":"other","current_interval_remaining_percent":40}]
		}`))
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK || r.RemainingPercent == nil || *r.RemainingPercent != 40 {
		t.Errorf("expected remaining 40, got status=%q remaining=%v", r.Status, r.RemainingPercent)
	}
}

func TestAuthCode(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"base_resp":{"status_code":1008},"model_remains":[]}`))
	})
	if got := p.Check(context.Background()).Status; got != provider.StatusAuthError {
		t.Errorf("got %q want auth_error", got)
	}
}

func TestAuthHTTP(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
	if got := p.Check(context.Background()).Status; got != provider.StatusAuthError {
		t.Errorf("got %q want auth_error", got)
	}
}

func TestRateLimited(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) })
	if got := p.Check(context.Background()).Status; got != provider.StatusRateLimited {
		t.Errorf("got %q want rate_limited", got)
	}
}

func TestNoRemains(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"base_resp":{"status_code":0},"model_remains":[]}`))
	})
	if got := p.Check(context.Background()).Status; got != provider.StatusParseError {
		t.Errorf("got %q want parse_error", got)
	}
}

func TestMissingPercent(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"base_resp":{"status_code":0},"model_remains":[{"model_name":"M"}]}`))
	})
	if got := p.Check(context.Background()).Status; got != provider.StatusParseError {
		t.Errorf("got %q want parse_error", got)
	}
}

func TestTimeout(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) { time.Sleep(5 * time.Second) })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if got := p.Check(ctx).Status; got != provider.StatusTimeout {
		t.Errorf("got %q want timeout", got)
	}
}