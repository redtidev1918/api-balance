package deepseek

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

// newTest builds a provider pointed at an httptest server.
func newTest(t *testing.T, handler http.HandlerFunc) (*balanceProvider, string) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &balanceProvider{key: "test-key", timeout: 3 * time.Second, endpoint: srv.URL}, srv.URL
}

func TestCheckOK(t *testing.T) {
	p, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"is_available": true,
			"balance_infos": [
				{"currency":"CNY","total_balance":"42.18","granted_balance":"10","topped_up_balance":"32.18"}
			]
		}`))
	})

	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status = %q, want ok: %+v", r.Status, r.Error)
	}
	if r.Balance == nil || *r.Balance != 42.18 {
		t.Errorf("balance = %v, want 42.18", r.Balance)
	}
	if r.Currency == nil || *r.Currency != "CNY" {
		t.Errorf("currency = %v, want CNY", r.Currency)
	}
	if r.Kind != provider.KindBalance {
		t.Errorf("kind = %q, want balance", r.Kind)
	}
}

func TestAuthError(t *testing.T) {
	p, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusAuthError {
		t.Errorf("status = %q, want auth_error", r.Status)
	}
}

func TestHTTPError(t *testing.T) {
	p, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusHTTPError {
		t.Errorf("status = %q, want http_error", r.Status)
	}
}

func TestRateLimited(t *testing.T) {
	p, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusRateLimited {
		t.Errorf("status = %q, want rate_limited", r.Status)
	}
}

func TestInvalidJSON(t *testing.T) {
	p, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not json`))
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusParseError {
		t.Errorf("status = %q, want parse_error", r.Status)
	}
}

func TestMissingBalance(t *testing.T) {
	p, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"is_available":true,"balance_infos":[]}`))
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusParseError {
		t.Errorf("status = %q, want parse_error", r.Status)
	}
}

func TestTimeout(t *testing.T) {
	p, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r := p.Check(ctx)
	if r.Status != provider.StatusTimeout {
		t.Errorf("status = %q, want timeout", r.Status)
	}
}