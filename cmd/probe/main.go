// Probe prints the limits once, for checking the data without the app.
package main

import (
	"context"
	"fmt"
	"time"

	"ai-limits/limits"
)

func main() {
	now := time.Now()
	for _, r := range []limits.Result{limits.Codex(context.Background()), limits.Claude(context.Background())} {
		fmt.Printf("%s (plan %q) err=%v\n", r.Provider, r.Plan, r.Err)
		for _, w := range r.Windows {
			pace, _ := w.PaceRemaining(now)
			fmt.Printf("  %-18s %3d%% left  pace-mark %.0f%%  pace %d  resets in %s\n", w.Label, w.Remaining(), pace, w.Pace(now), limits.FormatDuration(w.ResetsAt.Sub(now)))
		}
	}
}
