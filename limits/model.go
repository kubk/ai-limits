// Package limits reads subscription usage limits of Codex and Claude the way
// T3 Code does (github.com/pingdotgg/t3code, apps/server/src/provider and
// packages/shared/src/usageLimits.ts): by asking each provider's own,
// official CLI, never by calling private HTTP endpoints with stolen tokens.
//
//   - Codex: `codex app-server`, JSON-RPC `account/rateLimits/read`.
//   - Claude: `claude` in stream-json mode, control request `get_usage`
//     (what the Agent SDK's usage() and the /usage command send).
//
// No prompt is ever sent, so no tokens are spent.
package limits

import (
	"fmt"
	"math"
	"time"
)

const (
	sessionMins = 5 * 60
	weekMins    = 7 * 24 * 60
	monthMins   = 30 * 24 * 60
)

// Window is one usage allowance, as T3's ServerProviderUsageWindow.
type Window struct {
	ID          string
	Kind        string // session, weekly, monthly, other
	Label       string
	UsedPercent float64
	// DurationMins is the window's length; 0 when unknown (no pace mark).
	DurationMins int
	// ResetsAt is zero when unknown.
	ResetsAt time.Time
}

// Result is what one provider reported.
type Result struct {
	Provider  string // "codex" or "claude"
	Plan      string
	Windows   []Window
	Err       error `json:"-"`
	CheckedAt time.Time
}

func clampPercent(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Max(0, math.Min(100, v))
}

// Remaining is the percent of the allowance left, rounded.
func (w Window) Remaining() int {
	return int(math.Round(100 - clampPercent(w.UsedPercent)))
}

// ElapsedShare is the part of the window that has gone by, 0..1.
func (w Window) ElapsedShare(now time.Time) (float64, bool) {
	if w.ResetsAt.IsZero() || w.DurationMins <= 0 {
		return 0, false
	}
	length := time.Duration(w.DurationMins) * time.Minute
	left := w.ResetsAt.Sub(now)
	return math.Max(0, math.Min(1, float64(length-left)/float64(length))), true
}

// PaceRemaining is where an even pace would leave the bar: the percent of
// the window's time still left. Bars show quota remaining, so even pace
// tracks time remaining.
func (w Window) PaceRemaining(now time.Time) (float64, bool) {
	e, ok := w.ElapsedShare(now)
	if !ok {
		return 0, false
	}
	return (1 - e) * 100, true
}

type Pace int

const (
	PaceUnknown Pace = iota
	PaceUnder        // headroom left for the rest of the window
	PaceOn
	PaceAhead // spending faster than the window elapses
)

// Pace compares usage with elapsed time, within ±5 points as on pace.
func (w Window) Pace(now time.Time) Pace {
	e, ok := w.ElapsedShare(now)
	if !ok {
		return PaceUnknown
	}
	gap := w.UsedPercent - e*100
	switch {
	case gap > 5:
		return PaceAhead
	case gap < -5:
		return PaceUnder
	default:
		return PaceOn
	}
}

// FormatDuration renders "4d 5h", "3h 12m" or "7m".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d / (24 * time.Hour))
	hours := int(d%(24*time.Hour)) / int(time.Hour)
	mins := int(d%time.Hour) / int(time.Minute)
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

func fromEpochSeconds(v float64) time.Time {
	if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return time.Time{}
	}
	return time.UnixMilli(int64(v * 1000))
}

func fromISO(s *string) time.Time {
	if s == nil || *s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		return time.Time{}
	}
	return t
}
