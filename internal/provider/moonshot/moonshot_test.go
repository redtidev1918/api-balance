package moonshot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

func newTest(t *testing.T, handler http.HandlerFunc) *balanceProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &balanceProvider{key: "k", timeout: 3 * time.Second, endpoint: srv.URL}
}

func TestCheckOK(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth=%q", got)
		}
		w.Write([]byte(`{"code":0,"data":{"available_balance":17.32,"voucher_balance":2,"cash_balance":15.32}}`))
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status=%q %+v", r.Status, r.Error)
	}
	if r.Balance == nil || *r.Balance != 17.32 {
		t.Errorf("balance=%v want 17.32", r.Balance)
	}
	if r.Currency == nil || *r.Currency != "CNY" {
		t.Errorf("currency=%v want CNY", r.Currency)
	}
}

func TestNonZeroCode(t *testing.T) {
	p := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"code":1,"data":{}}`))
	})
	if got := p.Check(context.Background()).Status; got != provider.StatusUnknown {
		t.Errorf("got %q want unknown_error", got)
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
		w.Write([]byte(`{"code":0,"data":{}}`))
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