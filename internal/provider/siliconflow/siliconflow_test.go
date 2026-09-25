package siliconflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

func newTest(t *testing.T, handler http.HandlerFunc) *userProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &userProvider{key: "k", timeout: 3 * time.Second, endpoint: srv.URL}
}

func TestCheckOK(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth=%q", got)
		}
		w.Write([]byte(`{"data":{"balance":8.61,"totalUsage":120.4}}`))
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status=%q %+v", r.Status, r.Error)
	}
	if r.Balance == nil || *r.Balance != 8.61 {
		t.Errorf("balance=%v want 8.61", r.Balance)
	}
	if r.Currency == nil || *r.Currency != "CNY" {
		t.Errorf("currency=%v want CNY", r.Currency)
	}
}

func TestFallbackChargeBalance(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"chargeBalance":5.0}}`))
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK || r.Balance == nil || *r.Balance != 5.0 {
		t.Errorf("expected balance 5.0, got status=%q balance=%v", r.Status, r.Balance)
	}
}

func TestAuthError(t *testing.T) {
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

func TestHTTPError(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	if got := p.Check(context.Background()).Status; got != provider.StatusHTTPError {
		t.Errorf("got %q want http_error", got)
	}
}

func TestMissingBalance(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{}}`))
	})
	if got := p.Check(context.Background()).Status; got != provider.StatusParseError {
		t.Errorf("got %q want parse_error", got)
	}
}

func TestInvalidJSON(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>`))
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