package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"ai-limits/limits"

	"github.com/egoist/mygo/ui"
)

func sample() *app {
	now := time.Now()
	return &app{
		updated: now.Add(-90 * time.Second),
		results: map[string]limits.Result{
			"codex": {Provider: "codex", Plan: "free", Windows: []limits.Window{
				{ID: "primary", Label: "Monthly", UsedPercent: 4, DurationMins: 43200, ResetsAt: now.Add(101*time.Hour + 30*time.Minute)},
			}},
			"claude": {Provider: "claude", Plan: "max", Windows: []limits.Window{
				{ID: "seven_day", Label: "Weekly", UsedPercent: 2, DurationMins: 10080, ResetsAt: now.Add(141*time.Hour + 30*time.Minute)},
				{ID: "seven_day_fable", Label: "Weekly · Fable", UsedPercent: 1, DurationMins: 10080, ResetsAt: now.Add(141*time.Hour + 30*time.Minute)},
			}},
		},
	}
}

func TestView(t *testing.T) {
	for _, dark := range []bool{true, false} {
		tt := ui.NewTester(sample().view, popoverWidth, 400)
		tt.SetDark(dark)
		for _, want := range []string{"Codex", "Claude", "96%", "98%", "99%", "Weekly · Fable"} {
			if !tt.HasText(want) {
				t.Errorf("missing %q in %q", want, tt.Texts())
			}
		}
		if dir := os.Getenv("SNAP_DIR"); dir != "" {
			name := "light.png"
			if dark {
				name = "dark.png"
			}
			f, _ := os.Create(dir + "/" + name)
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}

func TestLoadingSnap(t *testing.T) {
	a := sample()
	a.loading = true
	tt := ui.NewTester(a.view, popoverWidth, 400)
	tt.SetDark(true)
	if dir := os.Getenv("SNAP_DIR"); dir != "" {
		f, _ := os.Create(dir + "/loading.png")
		png.Encode(f, tt.Image())
		f.Close()
	}
}
