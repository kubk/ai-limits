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
	for _, p := range []struct {
		name string
		read func(context.Context) ([]limits.Window, error)
	}{{"codex", limits.Codex}, {"claude", limits.Claude}, {"copilot", limits.Copilot}, {"amp", limits.Amp}} {
		windows, err := p.read(context.Background())
		fmt.Printf("%s err=%v\n", p.name, err)
		for _, w := range windows {
			pace, _ := w.PaceRemaining(now)
			fmt.Printf("  %-18s %3d%% left %-6s pace-mark %.0f%%  pace %d  resets in %s\n", w.Label, w.Remaining(), w.Amount, pace, w.Pace(now), limits.FormatDuration(w.ResetsAt.Sub(now)))
		}
	}
}
