package volcengine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

func newTest(t *testing.T, product Product, period Period, handler http.HandlerFunc) *volcProvider {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &volcProvider{
		ak: "AKLT-test", sk: "secret", product: product, period: period,
		endpoint: srv.URL + "/", timeout: 3 * time.Second,
	}
}

func TestCodingPlanOK(t *testing.T) {
	p := newTest(t, ProductCoding, PeriodWeekly, func(w http.ResponseWriter, r *http.Request) {
		// verify signed headers present
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		if r.Header.Get("X-Date") == "" {
			t.Error("missing X-Date")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ResponseMetadata": map[string]any{"RequestId": "r1", "Action": "GetCodingPlanUsage"},
			"Result": map[string]any{
				"Status":          "Running",
				"UpdateTimestamp": 1790375515,
				"QuotaUsage": []any{
					map[string]any{"Level": "session", "Percent": 0.3576455, "ResetTimestamp": 1790393448, "Cap": 100},
					map[string]any{"Level": "weekly", "Percent": 88.54450446666667, "ResetTimestamp": 1790524800, "Cap": 100},
					map[string]any{"Level": "monthly", "Percent": 75.0711095, "ResetTimestamp": 1791907199, "Cap": 100},
				},
				"HasReward": false,
			},
		})
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status=%q want ok: %+v", r.Status, r.Error)
	}
	if r.Kind != provider.KindQuota {
		t.Errorf("kind=%q want quota", r.Kind)
	}
	if r.RemainingPercent == nil || *r.RemainingPercent < 0 || *r.RemainingPercent > 100 {
		t.Fatalf("remaining=%v want in (0,100)", r.RemainingPercent)
	}
	// weekly used 88.54 => remaining ~11.46
	rem := *r.RemainingPercent
	if rem < 11.4 || rem > 11.5 {
		t.Errorf("remaining=%v want ~11.46", rem)
	}
	if r.ResetAt == nil {
		t.Errorf("reset_at=nil want non-nil")
	}
	if r.Used == nil || *r.Used != 88.54450446666667 {
		t.Errorf("used=%v want 88.54", r.Used)
	}
}

func TestCodingPlanNotSubscribed(t *testing.T) {
	p := newTest(t, ProductCoding, PeriodWeekly, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ResponseMetadata": map[string]any{"RequestId": "r2"},
			"Result":           map[string]any{"Status": "NotExist", "QuotaUsage": []any{}},
		})
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusParseError {
		t.Errorf("status=%q want parse_error (not subscribed)", r.Status)
	}
}

func TestAuthError(t *testing.T) {
	p := newTest(t, ProductCoding, PeriodWeekly, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{"ResponseMetadata": map[string]any{"Error": map[string]any{"CodeN": 100010, "Code": "SignatureDoesNotMatch", "Message": "bad sig"}}})
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusAuthError {
		t.Errorf("status=%q want auth_error", r.Status)
	}
}

func TestAgentPlanOK(t *testing.T) {
	p := newTest(t, ProductAgent, PeriodWeekly, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"ResponseMetadata": map[string]any{"RequestId": "r3"},
			"Result": map[string]any{
				"AFPWeekly": map[string]any{"Quota": 1000, "Used": 250},
			},
		})
	})
	r := p.Check(context.Background())
	if r.Status != provider.StatusOK {
		t.Fatalf("status=%q want ok: %+v", r.Status, r.Error)
	}
	if r.Used == nil || *r.Used != 250 {
		t.Errorf("used=%v want 250", r.Used)
	}
	if r.Total == nil || *r.Total != 1000 {
		t.Errorf("total=%v want 1000", r.Total)
	}
	if r.RemainingPercent == nil || *r.RemainingPercent < 74.9 || *r.RemainingPercent > 75.1 {
		t.Errorf("remaining=%v want ~75", r.RemainingPercent)
	}
}

func TestTimeout(t *testing.T) {
	p := newTest(t, ProductCoding, PeriodWeekly, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	r := p.Check(ctx)
	if r.Status != provider.StatusTimeout {
		t.Errorf("status=%q want timeout", r.Status)
	}
}