package limits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type copilotQuota struct {
	Unlimited   bool     `json:"unlimited"`
	CreditsUsed *float64 `json:"credits_used"`
	Entitlement *float64 `json:"entitlement"`
	// PercentRemaining is rounded to whole percents.
	PercentRemaining *float64 `json:"percent_remaining"`
}

type copilotUser struct {
	QuotaResetDateUTC *string                 `json:"quota_reset_date_utc"`
	QuotaSnapshots    map[string]copilotQuota `json:"quota_snapshots"`
}

// Copilot asks the `gh` CLI, with its own login, for the Copilot quota the
// editors show: `gh api /copilot_internal/user`. The token never leaves gh.
func Copilot(ctx context.Context) ([]Window, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	Prepare()
	bin, err := exec.LookPath("gh")
	if err != nil {
		return nil, fmt.Errorf("gh %w", ErrNotInstalled)
	}
	cmd := exec.CommandContext(ctx, bin, "api", "/copilot_internal/user")
	stderr := &limitedWriter{n: 4096}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("timed out waiting for the CLI")
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if strings.Contains(string(out), "Not Found") {
				return nil, fmt.Errorf("this GitHub account has no Copilot")
			}
			return nil, errors.New(stderr.lastLine())
		}
		return nil, err
	}
	var u copilotUser
	if err := json.Unmarshal(out, &u); err != nil {
		return nil, fmt.Errorf("Copilot returned unreadable usage")
	}
	return copilotWindows(u), nil
}

// copilotWindows keeps the metered quota: premium requests, now AI credits.
// Chat and completions are unlimited on paid plans.
func copilotWindows(u copilotUser) []Window {
	q, ok := u.QuotaSnapshots["premium_interactions"]
	if !ok || q.Unlimited {
		return nil
	}
	used, ok := percentFromAmounts(q.CreditsUsed, q.Entitlement)
	if !ok {
		if q.PercentRemaining == nil {
			return nil
		}
		used = clampPercent(100 - *q.PercentRemaining)
	}
	w := Window{ID: "premium", Label: "Monthly", UsedPercent: used, ResetsAt: fromISO(u.QuotaResetDateUTC)}
	if !w.ResetsAt.IsZero() {
		// The quota runs a calendar month up to the reset.
		w.DurationMins = int(w.ResetsAt.Sub(w.ResetsAt.AddDate(0, -1, 0)).Minutes())
	}
	return []Window{w}
}
