package main

import (
	"image/png"
	"os"
	"slices"
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
			"codex":   {{ID: "primary", Label: "Monthly", UsedPercent: 4, DurationMins: 43200, ResetsAt: now.Add(101*time.Hour + 30*time.Minute)}},
			"copilot": {{ID: "premium", Label: "Monthly", UsedPercent: 84.93, DurationMins: 43200, ResetsAt: now.Add(14*24*time.Hour + 19*time.Hour)}},
			"amp": {
				{ID: "orb", Label: "Orb time", UsedPercent: 32.8, DurationMins: 43200, ResetsAt: now.Add(14 * 24 * time.Hour), Amount: "504h"},
				{ID: "agent", Label: "Agent", UsedPercent: 3.05, DurationMins: 43200, ResetsAt: now.Add(14 * 24 * time.Hour)},
			},
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
		tt := ui.NewTester(sample().view, popoverWidth, 720)
		tt.SetDark(dark)
		for _, want := range []string{"Codex", "Claude", "96%", "98%", "99%", "Weekly · Fable", "Copilot", "15%", "Amp", "504h", "Orb time"} {
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

func TestBadgeText(t *testing.T) {
	now := time.Now()
	in := func(d time.Duration) map[string][]limits.Window {
		return map[string][]limits.Window{
			"codex": {{ResetsAt: now.Add(29 * 24 * time.Hour)}},
			"amp": {
				{ID: "orb", Label: "Orb time", UsedPercent: 32.8, DurationMins: 43200, ResetsAt: now.Add(14 * 24 * time.Hour), Amount: "504h"},
				{ID: "agent", Label: "Agent", UsedPercent: 3.05, DurationMins: 43200, ResetsAt: now.Add(14 * 24 * time.Hour)},
			},
			"claude": {{ResetsAt: now.Add(d)}, {}},
		}
	}
	for d, want := range map[time.Duration]string{
		13*time.Hour + 40*time.Minute: "13h",
		time.Hour:                     "1h",
		45 * time.Minute:              "45m",
		25 * time.Hour:                "",
		-time.Minute:                  "",
	} {
		if got := badgeText(in(d), now); got != want {
			t.Errorf("%v: got %q, want %q", d, got, want)
		}
	}
}

func TestReorder(t *testing.T) {
	a := sample()
	var saved []settings
	a.save = func(s settings) { saved = append(saved, s) }
	tt := ui.NewTester(a.view, popoverWidth, 720)
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("96%") || !tt.HasText("Order") {
		t.Fatalf("settings screen texts %q", tt.Texts())
	}
	snap(t, tt, "settings.png")
	if err := tt.Click("Move Copilot up"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Move Codex down"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	want := []string{"copilot", "codex", "claude", "amp"}
	if !slices.Equal(a.settings.Order, want) || len(saved) != 2 || !slices.Equal(saved[1].Order, want) {
		t.Fatalf("order %v, saved %v", a.settings.Order, saved)
	}
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	texts := tt.Texts()
	if i, j := slices.Index(texts, "Copilot"), slices.Index(texts, "Codex"); i < 0 || j < 0 || i > j {
		t.Errorf("limits not in the new order: %q", texts)
	}
}

func TestOrdered(t *testing.T) {
	var ids []string
	for _, p := range ordered([]string{"copilot", "gone", "copilot"}) {
		ids = append(ids, p.id)
	}
	if want := []string{"copilot", "codex", "claude", "amp"}; !slices.Equal(ids, want) {
		t.Errorf("got %v, want %v", ids, want)
	}
}
