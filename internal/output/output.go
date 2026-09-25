// Package output renders balance results as aligned plain text or stable JSON.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/redtidev1918/api-balance/internal/provider"
)

// plainRenderer renders results as aligned, key–value lines.
type plainRenderer struct {
	w io.Writer
}

// RenderPlain writes a human-friendly aligned dump.
func RenderPlain(w io.Writer, results []provider.BalanceResult) error {
	// order by provider name for stable, deterministic output
	sorted := make([]provider.BalanceResult, len(results))
	copy(sorted, results)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Provider < sorted[j].Provider })

	for _, r := range sorted {
		line := r.Provider
		if r.Status != provider.StatusOK {
			line += "  " + strings.ToUpper(string(r.Status))
		} else {
			line += "  " + FormatValue(r)
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// FormatValue renders a single successful result's monetary/percent value.
func FormatValue(r provider.BalanceResult) string {
	switch r.Kind {
	case provider.KindBalance:
		if r.Currency != nil && r.Balance != nil {
			return fmt.Sprintf("%s %.2f", *r.Currency, *r.Balance)
		}
	case provider.KindQuota:
		if r.RemainingPercent != nil {
			return fmt.Sprintf("%.0f%% remaining", *r.RemainingPercent)
		}
		if r.Used != nil && r.Total != nil {
			return fmt.Sprintf("%.0f/%.0f used", *r.Used, *r.Total)
		}
	case provider.KindUsage:
		if r.Used != nil && r.Total != nil {
			return fmt.Sprintf("%.2f / %.2f", *r.Used, *r.Total)
		}
	}
	return "n/a"
}

// JSONOutput is the stable machine-readable schema.
type JSONOutput struct {
	CheckedAt time.Time                   `json:"checked_at"`
	Results   []provider.BalanceResult    `json:"results"`
}

// RenderJSON writes a stable JSON document to w. It marshals once and writes
// with a trailing newline for CLI friendliness.
func RenderJSON(w io.Writer, results []provider.BalanceResult, checkedAt time.Time) error {
	out := JSONOutput{CheckedAt: checkedAt, Results: results}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(&out)
}