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
	week := now.Add(141*time.Hour + 30*time.Minute)
	return &app{
		errs: map[string]error{},
		windows: map[string][]limits.Window{
			"codex": {{ID: "primary", Label: "Monthly", UsedPercent: 4, DurationMins: 43200, ResetsAt: now.Add(101*time.Hour + 30*time.Minute)}},
			"claude": {
				{ID: "seven_day", Label: "Weekly", UsedPercent: 2, DurationMins: 10080, ResetsAt: week},
				{ID: "seven_day_fable", Label: "Weekly · Fable", UsedPercent: 1, DurationMins: 10080, ResetsAt: week},
			},
		},
	}
}

// snap saves the view's image to $SNAP_DIR, to look at by eye.
func snap(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SNAP_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(dir + "/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
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
		name := "light.png"
		if dark {
			name = "dark.png"
		}
		snap(t, tt, name)
	}
}

func TestPlaceholderAndErrors(t *testing.T) {
	a := &app{windows: map[string][]limits.Window{}, errs: map[string]error{"codex": limits.ErrNotInstalled}}
	tt := ui.NewTester(a.view, popoverWidth, 400)
	if !tt.HasText("Reading limits…") || !tt.HasText("Install the codex CLI and sign in to see its limits.") {
		t.Errorf("texts %q", tt.Texts())
	}
}
