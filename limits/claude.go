package limits

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type claudeBucket struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    *string  `json:"resets_at"`
}

type claudeMoney struct {
	AmountMinor *float64 `json:"amount_minor"`
	Currency    string   `json:"currency"`
	Exponent    *int     `json:"exponent"`
}

type claudeRateLimits struct {
	FiveHour   *claudeBucket `json:"five_hour"`
	SevenDay   *claudeBucket `json:"seven_day"`
	ExtraUsage *struct {
		IsEnabled    bool     `json:"is_enabled"`
		MonthlyLimit *float64 `json:"monthly_limit"`
		UsedCredits  *float64 `json:"used_credits"`
		Utilization  *float64 `json:"utilization"`
	} `json:"extra_usage"`
	Spend *struct {
		Enabled bool         `json:"enabled"`
		Used    *claudeMoney `json:"used"`
		Limit   *claudeMoney `json:"limit"`
	} `json:"spend"`
	ModelScoped []struct {
		DisplayName *string  `json:"display_name"`
		Utilization *float64 `json:"utilization"`
		ResetsAt    *string  `json:"resets_at"`
	} `json:"model_scoped"`
}

type claudeUsage struct {
	SubscriptionType    string            `json:"subscription_type"`
	RateLimitsAvailable bool              `json:"rate_limits_available"`
	RateLimits          *claudeRateLimits `json:"rate_limits"`
}

// Claude asks the `claude` CLI for usage with the Agent SDK's control
// protocol, as T3 Code's ClaudeProvider does: initialize, then get_usage.
// No user message is sent, so nothing reaches the model.
func Claude(ctx context.Context) Result {
	res := Result{Provider: "claude", CheckedAt: time.Now()}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	s, err := start(ctx, "claude",
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		// Only the user's login matters: no project or user settings,
		// hooks, MCP servers or transcript on disk.
		"--setting-sources", "",
		"--settings", `{"disableAllHooks":true}`,
		"--strict-mcp-config",
		"--no-session-persistence",
	)
	if err != nil {
		res.Err = err
		return res
	}
	defer s.close()

	isResponse := func(id string) func([]byte) bool {
		want := []byte(`"request_id":"` + id + `"`)
		return func(line []byte) bool {
			return bytes.Contains(line, []byte(`"control_response"`)) && bytes.Contains(line, want)
		}
	}
	s.send(map[string]any{"type": "control_request", "request_id": "init", "request": map[string]any{"subtype": "initialize"}})
	if _, err := s.next(ctx, isResponse("init")); err != nil {
		res.Err = err
		return res
	}
	s.send(map[string]any{"type": "control_request", "request_id": "usage", "request": map[string]any{"subtype": "get_usage", "skip_behaviors": true}})
	line, err := s.next(ctx, isResponse("usage"))
	if err != nil {
		res.Err = err
		return res
	}
	var msg struct {
		Response struct {
			Subtype  string      `json:"subtype"`
			Error    string      `json:"error"`
			Response claudeUsage `json:"response"`
		} `json:"response"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		res.Err = err
		return res
	}
	if msg.Response.Subtype != "success" {
		res.Err = fmt.Errorf("Claude could not read usage: %s", msg.Response.Error)
		return res
	}
	u := msg.Response.Response
	res.Plan = u.SubscriptionType
	if !u.RateLimitsAvailable || u.RateLimits == nil {
		res.Err = fmt.Errorf("this Claude login has no subscription limits")
		return res
	}
	res.Windows = claudeWindows(u.RateLimits)
	return res
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// claudeWindows is T3's claudeUsageResponseToLimits. get_usage percentages
// are already 0-100.
func claudeWindows(rl *claudeRateLimits) []Window {
	var out []Window
	if b := rl.FiveHour; b != nil && b.Utilization != nil {
		out = append(out, Window{ID: "five_hour", Kind: "session", Label: "Session",
			DurationMins: sessionMins, UsedPercent: clampPercent(*b.Utilization), ResetsAt: fromISO(b.ResetsAt)})
	}
	if b := rl.SevenDay; b != nil && b.Utilization != nil {
		out = append(out, Window{ID: "seven_day", Kind: "weekly", Label: "Weekly",
			DurationMins: weekMins, UsedPercent: clampPercent(*b.Utilization), ResetsAt: fromISO(b.ResetsAt)})
	}
	for _, m := range rl.ModelScoped {
		if m.DisplayName == nil || m.Utilization == nil {
			continue
		}
		out = append(out, Window{
			ID:   "seven_day_" + nonAlnum.ReplaceAllString(strings.ToLower(*m.DisplayName), "_"),
			Kind: "weekly", Label: "Weekly · " + *m.DisplayName,
			DurationMins: weekMins, UsedPercent: clampPercent(*m.Utilization), ResetsAt: fromISO(m.ResetsAt),
		})
	}
	if p, ok := claudeSpendPercent(rl); ok {
		out = append(out, Window{ID: "monthly_spend", Kind: "monthly", Label: "Monthly spend", UsedPercent: p})
	}
	return out
}

func percentFromAmounts(used, limit *float64) (float64, bool) {
	if used == nil || limit == nil || *used < 0 || *limit <= 0 {
		return 0, false
	}
	return clampPercent(*used / *limit * 100), true
}

func claudeSpendPercent(rl *claudeRateLimits) (float64, bool) {
	// Minor-unit ratios need matching currency and scale; spend.percent is rounded.
	if sp := rl.Spend; sp != nil && sp.Enabled && sp.Used != nil && sp.Limit != nil &&
		strings.TrimSpace(sp.Used.Currency) != "" &&
		strings.TrimSpace(sp.Used.Currency) == strings.TrimSpace(sp.Limit.Currency) &&
		sp.Used.Exponent != nil && sp.Limit.Exponent != nil && *sp.Used.Exponent == *sp.Limit.Exponent {
		if p, ok := percentFromAmounts(sp.Used.AmountMinor, sp.Limit.AmountMinor); ok {
			return p, true
		}
	}
	ex := rl.ExtraUsage
	if ex == nil || !ex.IsEnabled {
		return 0, false
	}
	if p, ok := percentFromAmounts(ex.UsedCredits, ex.MonthlyLimit); ok {
		return p, true
	}
	if ex.Utilization != nil && *ex.Utilization >= 0 {
		return clampPercent(*ex.Utilization), true
	}
	return 0, false
}
