package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

func newTest(t *testing.T, handler http.HandlerFunc) *creditProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &creditProvider{key: "k", timeout: 3 * time.Second, endpoint: srv.URL}
}

func TestCheckOK(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth = %q", got)
		}
		w.Write([]byte(`{"data":{"label":"Main","limit":100,"limit_remaining":12.43,"usage":87.57,"is_free_tier":false}}`))
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status=%q %+v", r.Status, r.Error)
	}
	if r.Balance == nil || *r.Balance != 12.43 {
		t.Errorf("balance=%v want 12.43", r.Balance)
	}
	if r.Currency == nil || *r.Currency != "USD" {
		t.Errorf("currency=%v want USD", r.Currency)
	}
}

func TestCheckFallbackLimitMinusUsage(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"limit":100,"usage":80}}`))
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status=%q", r.Status)
	}
	if r.Balance == nil || *r.Balance != 20 {
		t.Errorf("balance=%v want 20", r.Balance)
	}
}

func TestAuthError(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) })
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
		w.Write([]byte(`nope`))
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