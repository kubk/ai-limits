package limits

import (
	"bytes"
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
func Codex(ctx context.Context) Result {
	res := Result{Provider: "codex", CheckedAt: time.Now()}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	s, err := start(ctx, "codex", "app-server")
	if err != nil {
		res.Err = err
		return res
	}
	defer s.close()

	isID := func(id int) func([]byte) bool {
		want := []byte(fmt.Sprintf(`"id":%d`, id))
		return func(line []byte) bool { return bytes.Contains(line, want) }
	}
	if err := s.send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"clientInfo": map[string]any{
			"name": "ai_limits", "title": "AI Limits", "version": "0.1.0",
		}},
	}); err != nil {
		res.Err = err
		return res
	}
	if _, err := s.next(ctx, isID(1)); err != nil {
		res.Err = err
		return res
	}
	s.send(map[string]any{"jsonrpc": "2.0", "method": "initialized"})
	s.send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "account/rateLimits/read", "params": nil})
	line, err := s.next(ctx, isID(2))
	if err != nil {
		res.Err = err
		return res
	}
	var msg struct {
		Result *codexRateLimitsResponse `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		res.Err = err
		return res
	}
	if msg.Error != nil {
		res.Err = fmt.Errorf("Codex could not read usage: %s", msg.Error.Message)
		return res
	}
	if msg.Result == nil {
		res.Err = fmt.Errorf("Codex returned no usage")
		return res
	}
	snap := msg.Result.RateLimits
	if byID, ok := msg.Result.RateLimitsByLimitID["codex"]; ok {
		snap = byID
	}
	if snap.PlanType != nil {
		res.Plan = *snap.PlanType
	}
	res.Windows = codexWindows(snap)
	return res
}

func kindForDuration(mins int) string {
	switch {
	case mins >= monthMins:
		return "monthly"
	case mins >= weekMins:
		return "weekly"
	default:
		return "session"
	}
}

func labelForKind(kind string) string {
	switch kind {
	case "session":
		return "Session"
	case "weekly":
		return "Weekly"
	default:
		return "Monthly"
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
		kind := kindForDuration(mins)
		w := Window{ID: p.id, Kind: kind, Label: labelForKind(kind), UsedPercent: clampPercent(*p.w.UsedPercent), DurationMins: mins}
		if p.w.ResetsAt != nil {
			w.ResetsAt = fromEpochSeconds(*p.w.ResetsAt)
		}
		out = append(out, w)
	}
	if il := s.IndividualLimit; il != nil {
		out = append(out, Window{
			ID: "individual", Kind: "monthly", Label: "Monthly credit limit",
			UsedPercent: clampPercent(100 - il.RemainingPercent),
			ResetsAt:    fromEpochSeconds(il.ResetsAt),
		})
	}
	return out
}
