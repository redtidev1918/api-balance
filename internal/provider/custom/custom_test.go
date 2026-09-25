package custom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

func newSrv(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestCustomBalanceBearer(t *testing.T) {
	url := newSrv(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("auth=%q want Bearer secret", got)
		}
		w.Write([]byte(`{"data":{"balance":42.18,"currency":"CNY"}}`))
	})
	spec := Spec{
		Name:     "my-api",
		Endpoint: url,
		Balance:  "data.balance",
		Currency: "data.currency",
	}
	// buildRequest will default to bearer auth when apiKey set and no auth type
	p := New(spec, "secret", 3*time.Second)
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status=%q %+v", r.Status, r.Error)
	}
	if r.Balance == nil || *r.Balance != 42.18 {
		t.Errorf("balance=%v want 42.18", r.Balance)
	}
	if r.Currency == nil || *r.Currency != "CNY" {
		t.Errorf("currency=%v", r.Currency)
	}
	if r.Kind != provider.KindBalance {
		t.Errorf("kind=%q want balance", r.Kind)
	}
}

func TestCustomQuotaRemainingTotal(t *testing.T) {
	url := newSrv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"used":400,"total":1000,"available":600}}`))
	})
	spec := Spec{
		Name:      "quota-api",
		Endpoint:  url,
		Used:      "data.used",
		Total:     "data.total",
		Remaining: "data.available",
	}
	p := New(spec, "", 3*time.Second)
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status=%q %+v", r.Status, r.Error)
	}
	// remaining 600 / total 1000 => 60%
	if r.RemainingPercent == nil || *r.RemainingPercent != 60 {
		t.Errorf("remaining%%=%v want 60", r.RemainingPercent)
	}
	if r.Kind != provider.KindQuota {
		t.Errorf("kind=%q want quota", r.Kind)
	}
}

func TestCustomQueryAuth(t *testing.T) {
	url := newSrv(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("api_key"); got != "tok" {
			t.Errorf("query api_key=%q want tok", got)
		}
		w.Write([]byte(`{"balance":99.0}`))
	})
	spec := Spec{
		Name:     "query-api",
		Endpoint: url,
		Auth:     map[string]interface{}{"type": "query", "query_name": "api_key"},
		Balance:  "balance",
	}
	p := New(spec, "tok", 3*time.Second)
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK || r.Balance == nil || *r.Balance != 99 {
		t.Errorf("status=%q balance=%v", r.Status, r.Balance)
	}
}

func TestCustomHeaderAuth(t *testing.T) {
	url := newSrv(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "hval" {
			t.Errorf("header=%q", got)
		}
		w.Write([]byte(`{"balance":5}`))
	})
	spec := Spec{
		Name:     "header-api",
		Endpoint: url,
		Auth:     map[string]interface{}{"type": "header", "header_name": "X-API-Key"},
		Balance:  "balance",
	}
	p := New(spec, "hval", 3*time.Second)
	if r := p.Check(context.Background()); r.Status != provider.StatusOK || r.Balance == nil || *r.Balance != 5 {
		t.Errorf("status=%q balance=%v", r.Status, r.Balance)
	}
}

func TestCustomAuthError(t *testing.T) {
	url := newSrv(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) })
	spec := Spec{Name: "a", Endpoint: url, Balance: "balance"}
	p := New(spec, "k", 3*time.Second)
	if got := p.Check(context.Background()).Status; got != provider.StatusAuthError {
		t.Errorf("got %q want auth_error", got)
	}
}

func TestCustomStatusCheck(t *testing.T) {
	url := newSrv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"error","balance":10}`))
	})
	spec := Spec{
		Name:       "s",
		Endpoint:   url,
		Balance:    "balance",
		StatusPath: "status",
		StatusOK:   "ok",
	}
	p := New(spec, "", 3*time.Second)
	if got := p.Check(context.Background()).Status; got == provider.StatusOK {
		t.Errorf("expected non-ok for status=error, got ok")
	}
}

func TestCustomInvalidJSON(t *testing.T) {
	url := newSrv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`bad{`))
	})
	spec := Spec{Name: "j", Endpoint: url, Balance: "balance"}
	p := New(spec, "", 3*time.Second)
	if got := p.Check(context.Background()).Status; got != provider.StatusParseError {
		t.Errorf("got %q want parse_error", got)
	}
}

func TestCustomNoExtractor(t *testing.T) {
	url := newSrv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"a":1}`))
	})
	spec := Spec{Name: "n", Endpoint: url}
	p := New(spec, "", 3*time.Second)
	if got := p.Check(context.Background()).Status; got != provider.StatusUnsupported {
		t.Errorf("got %q want unsupported", got)
	}
}