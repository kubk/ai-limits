package limits

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type codexWindow struct {
	UsedPercent        *float64 `json:"usedPercent"`
	ResetsAt           *float64 `json:"resetsAt"`
	WindowDurationMins *int     `json:"windowDurationMins"`
}

type codexSnapshot struct {
	LimitID         *string      `json:"limitId"`
	PlanType        *string      `json:"planType"`
	Primary         *codexWindow `json:"primary"`
	Secondary       *codexWindow `json:"secondary"`
	IndividualLimit *struct {
		RemainingPercent float64 `json:"remainingPercent"`
		ResetsAt         float64 `json:"resetsAt"`
	} `json:"individualLimit"`
}

type codexRateLimitsResponse struct {
	RateLimits          codexSnapshot            `json:"rateLimits"`
	RateLimitsByLimitID map[string]codexSnapshot `json:"rateLimitsByLimitId"`
}

// Codex asks `codex app-server` for the account's rate limits, as T3 Code's
// CodexProvider does.
func Codex(ctx context.Context) ([]Window, error) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	s, err := start(ctx, "codex", "app-server")
	if err != nil {
		return nil, err
	}
	defer s.close()

	reply := func(id int) func(message) bool {
		return func(m message) bool { return m.ID != nil && *m.ID == id && m.Method == "" }
	}
	var init json.RawMessage
	if err := s.call(ctx, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]any{
			"name": "ai_limits", "title": "AI Limits", "version": "0.1.0",
		}},
	}, reply(1), &init); err != nil {
		return nil, err
	}
	if err := s.call(ctx, map[string]any{"jsonrpc": "2.0", "method": "initialized"}, nil, nil); err != nil {
		return nil, err
	}
	var msg struct {
		Result *codexRateLimitsResponse `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := s.call(ctx, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "account/rateLimits/read", "params": nil}, reply(2), &msg); err != nil {
		return nil, err
	}
	if msg.Error != nil {
		return nil, fmt.Errorf("Codex could not read usage: %s", msg.Error.Message)
	}
	if msg.Result == nil {
		return nil, fmt.Errorf("Codex returned no usage")
	}
	snap := msg.Result.RateLimits
	if byID, ok := msg.Result.RateLimitsByLimitID["codex"]; ok {
		snap = byID
	}
	return codexWindows(snap), nil
}

// labelForMins names a window by its length, as T3's kindForDuration and
// labelForKind.
func labelForMins(mins int) string {
	switch {
	case mins >= monthMins:
		return "Monthly"
	case mins >= weekMins:
		return "Weekly"
	default:
		return "Session"
	}
}

// codexWindows is T3's codexRateLimitsToWindows: primary/secondary are
// positions; without windowDurationMins, paid plans use 5-hour/weekly
// windows and Free/Go use monthly.
func codexWindows(s codexSnapshot) []Window {
	// Model-specific limits stay out of the main allowance rows.
	if s.LimitID != nil && *s.LimitID != "" && *s.LimitID != "codex" {
		return nil
	}
	monthlyPlan := s.PlanType != nil && (*s.PlanType == "free" || *s.PlanType == "go")
	primaryFallback := sessionMins
	if monthlyPlan {
		primaryFallback = monthMins
	}
	positions := []struct {
		id       string
		w        *codexWindow
		fallback int
	}{
		{"primary", s.Primary, primaryFallback},
		{"secondary", s.Secondary, weekMins},
	}
	var out []Window
	for _, p := range positions {
		if p.w == nil || p.w.UsedPercent == nil {
			continue
		}
		mins := p.fallback
		if p.w.WindowDurationMins != nil {
			mins = *p.w.WindowDurationMins
		}
		w := Window{ID: p.id, Label: labelForMins(mins), UsedPercent: clampPercent(*p.w.UsedPercent), DurationMins: mins}
		if p.w.ResetsAt != nil {
			w.ResetsAt = fromEpochSeconds(*p.w.ResetsAt)
		}
		out = append(out, w)
	}
	if il := s.IndividualLimit; il != nil {
		out = append(out, Window{
			ID: "individual", Label: "Monthly credit limit",
			UsedPercent: clampPercent(100 - il.RemainingPercent),
			ResetsAt:    fromEpochSeconds(il.ResetsAt),
		})
	}
	return out
}
