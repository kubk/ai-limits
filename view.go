package main

import (
	_ "embed"
	"errors"
	"fmt"
	"time"

	"ai-limits/limits"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/sfsymbols"
	"github.com/egoist/mygo/ui"
)

const popoverWidth = 460

// palette is a provider's color in T3 Code (usageProviders.ts): Codex
// draws in the contrast foreground, Claude in its orange. Bars fill with
// it at 35% opacity, as T3's do.
type palette struct {
	color, logo ui.Color
}

func providerPalette(id string, dark bool) palette {
	if id == "claude" {
		return palette{color: ui.Hex("#D97757"), logo: ui.Hex("#D97757")}
	}
	if dark {
		return palette{color: ui.RGB(255, 255, 255), logo: ui.RGB(255, 255, 255)}
	}
	return palette{color: ui.Hex("#18181B"), logo: ui.Hex("#18181B")}
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	now := c.Now()
	c.Root().Background(ui.Transparent)
	if c.Shortcut(ui.Cmd, ui.KeyR) {
		a.refresh()
	}
	c.OnShortcut(ui.Cmd, ui.KeyQ, mygo.App.Quit)

	box := ui.Column(c).FillWidth()
	if !c.Vibrancy() { // the popover draws its own material on macOS
		box.Background(t.Surface).Radius(12).Border(1, t.Border)
	}
	box.Padding(14).Gap(14).Children(func() {
		a.header(c, now)
		for _, p := range providers {
			a.section(c, p.id, p.name, now)
		}
	})
}

func (a *app) header(c *ui.Context, now time.Time) {
	t := c.Theme()
	ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, "Usage limits").FontSize(13).FontWeight(600)
		ui.Spacer(c)
		btn := ui.ButtonBase(c.Key("refresh")).Size(24, 24).Radius(6).
			Label("Refresh").Tooltip("Refresh  ⌘R").Disabled(a.loading)
		if btn.Hovered() {
			btn.Background(t.SurfaceHover)
		}
		btn.OnClick(func() { a.refresh() }).Children(func() {
			if a.loading {
				ui.Spinner(c).Size(13, 13)
			} else {
				sfsymbols.Symbol(c, "arrow.clockwise").Size(13).Color(t.TextMuted).Weight(sfsymbols.Medium)
			}
		})
	})
}

func (a *app) section(c *ui.Context, id, name string, now time.Time) {
	t := c.Theme()
	pal := providerPalette(id, t.Dark)
	r, ok := a.results[id]
	ui.Column(c.Key(id)).Gap(8).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(7).PaddingStart(2).Children(func() {
			logo(c, id, pal.logo)
			ui.Text(c, name).FontSize(13).FontWeight(600)
		})
		switch {
		case !ok:
			placeholder(c)
		case len(r.Windows) == 0 && r.Err == nil:
			note(c, "No limits reported.")
		}
		for _, w := range r.Windows {
			meter(c, w, pal, now)
		}
		if r.Err != nil {
			msg := r.Err.Error()
			if errors.Is(r.Err, limits.ErrNotInstalled) {
				msg = fmt.Sprintf("Install the %s CLI and sign in to see its limits.", id)
			} else if len(r.Windows) > 0 {
				msg = "Showing earlier numbers: " + msg
			}
			note(c, msg)
		}
	})
}

// card is the rounded tile around a meter or a note.
func card(c *ui.Context) ui.Element {
	t := c.Theme()
	bg, border := ui.RGBA(0, 0, 0, 0.04), ui.RGBA(0, 0, 0, 0.06)
	if t.Dark {
		bg, border = ui.RGBA(255, 255, 255, 0.045), ui.RGBA(255, 255, 255, 0.06)
	}
	return ui.Row(c).Radius(10).Background(bg).Border(1, border)
}

func note(c *ui.Context, s string) {
	card(c).Padding(12).Children(func() {
		ui.Text(c, s).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
	})
}

func placeholder(c *ui.Context) {
	card(c).Padding(12).Height(70).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Spinner(c).Size(14, 14)
		ui.Text(c, "Reading limits…").FontSize(11).TextColor(c.Theme().TextMuted)
	})
}

// meter is one allowance: how much is left, and a bar of it with the
// even-pace mark and the time to the reset, as T3 Code's limits view.
func meter(c *ui.Context, w limits.Window, pal palette, now time.Time) {
	t := c.Theme()
	left := w.Remaining()
	card(c.Key(w.ID)).PaddingX(14).PaddingY(12).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Width(118).Gap(1).Children(func() {
			ui.Text(c, w.Label).FontSize(12).SingleLine()
			ui.Row(c).AlignItems(ui.Baseline).Gap(5).Children(func() {
				ui.Textf(c, "%d%%", left).FontSize(26).FontWeight(500).FontFeatures("tnum").LetterSpacing(-0.4)
				ui.Text(c, "left").FontSize(12).TextColor(t.TextMuted)
				paceIcon(c, w.Pace(now))
			})
		})
		bar(c, w, pal, now)
	})
}

// Lucide's trending-down, gauge and trending-up, as T3's PaceIcon.
var (
	trendingDown = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 17h6v-6"/><path d="m22 17-8.5-8.5-5 5L2 7"/></svg>`))
	trendingUp   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 7h6v6"/><path d="m22 7-8.5 8.5-5-5L2 17"/></svg>`))
	gauge        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m12 14 4-4"/><path d="M3.34 19a10 10 0 1 1 17.32 0"/></svg>`))
)

func paceIcon(c *ui.Context, p limits.Pace) {
	var icon *ui.SVG
	var tip string
	switch p {
	case limits.PaceUnder:
		icon, tip = trendingDown, "Under pace: headroom left for the rest of the window"
	case limits.PaceOn:
		icon, tip = gauge, "On pace with the window"
	case limits.PaceAhead:
		icon, tip = trendingUp, "Ahead of pace: spending faster than the window elapses"
	default:
		return
	}
	ui.Icon(c, icon).Size(14, 14).TextColor(c.Theme().TextMuted).Tooltip(tip)
}

// bar is T3 Code's pooled limit bar: the provider's color at 35% for what
// is left, faint hatching over what is used, the even-pace tick in a gap
// cut from the bar, and the reset chip at its end.
func bar(c *ui.Context, w limits.Window, pal palette, now time.Time) {
	t := c.Theme()
	muted, fg, chipBg := ui.Hex("#FAFAFA"), ui.Hex("#18181B"), ui.RGBA(250, 250, 250, 0.85)
	if t.Dark {
		muted, fg, chipBg = ui.RGBA(255, 255, 255, 0.03), ui.Hex("#F5F5F5"), ui.RGBA(0, 0, 0, 0.85)
	}
	remaining := float32(w.Remaining()) / 100
	pace, hasPace := w.PaceRemaining(now)
	tip := fmt.Sprintf("%d%% left", w.Remaining())
	if hasPace {
		tip += fmt.Sprintf(" · %.0f%% of the window left\nThe line is where even spending would be.", pace)
	}
	if !w.ResetsAt.IsZero() {
		tip += "\nResets " + w.ResetsAt.Local().Format("Mon, Jan 2 at 15:04") + " · in " + limits.FormatDuration(w.ResetsAt.Sub(now))
	}
	const radius = 6
	ui.Row(c).Grow(1).Height(32).Radius(radius).PaddingX(8).AlignItems(ui.Center).Tooltip(tip).
		Draw(func(p *ui.Painter, r ui.Rect) {
			at := r.X + r.W*float32(pace)/100
			at = max(r.X+3, min(at, r.X+r.W-3))
			paint := func() {
				p.Fill(r, muted, radius)
				fillW := r.W * remaining
				if fillW > 0 {
					p.Fill(ui.Rect{X: r.X, Y: r.Y, W: fillW, H: r.H}, pal.color.Alpha(0.35), radius)
				}
				// Hatching at 135°, 1px lines every 5px, over the used part.
				if remaining < 1 {
					used := ui.Rect{X: r.X + fillW, Y: r.Y, W: r.W - fillW, H: r.H}
					p.Clip(used, 0, func() {
						var hatch ui.Path
						for x := used.X - r.H; x < used.X+used.W+r.H; x += 5 {
							hatch.MoveTo(x, r.Y+r.H).LineTo(x+r.H, r.Y)
						}
						p.StrokePath(hatch.Cap(ui.ButtCap), 1, pal.color.Alpha(0.2))
					})
				}
			}
			p.Clip(r, radius, func() {
				if !hasPace {
					paint()
					return
				}
				// A transparent slot 6px wide around the tick.
				p.Clip(ui.Rect{X: r.X, Y: r.Y, W: at - 3 - r.X, H: r.H}, 0, paint)
				p.Clip(ui.Rect{X: at + 3, Y: r.Y, W: r.X + r.W - at - 3, H: r.H}, 0, paint)
			})
		}).
		DrawOver(func(p *ui.Painter, r ui.Rect) {
			// As T3's: the tick first, then the reset chip pinned to the
			// bar's end, which covers the tick where they meet.
			if hasPace {
				at := r.X + r.W*float32(pace)/100
				at = max(r.X+3, min(at, r.X+r.W-3))
				p.Fill(ui.Rect{X: at - 1, Y: r.Y + 2, W: 2, H: r.H - 4}, fg, 1)
			}
			if !w.ResetsAt.IsZero() {
				label := ui.Span{Text: "↻ " + limits.FormatDuration(w.ResetsAt.Sub(now)), Size: 11, Weight: 500, Color: fg}
				tw, th := p.MeasureText(0, label)
				cw, ch := tw+12, th+4
				chip := ui.Rect{X: r.X + r.W - 8 - cw, Y: r.Y + (r.H-ch)/2, W: cw, H: ch}
				p.Fill(chip, chipBg, 3)
				p.RichText(chip.X+6, chip.Y+2, 0, label)
			}
		})
}

// The brand marks T3 Code shows (apps/web/src/components/Icons.tsx).
var (
	//go:embed icons/openai.svg
	openaiSVG []byte
	//go:embed icons/claude.svg
	claudeSVG []byte

	openaiIcon = ui.MustParseSVG(openaiSVG)
	claudeIcon = ui.MustParseSVG(claudeSVG)
)

// logo shows the provider's mark: OpenAI's in the text color, as T3 does,
// and Claude's in its orange.
func logo(c *ui.Context, id string, col ui.Color) {
	icon := openaiIcon
	if id == "claude" {
		icon = claudeIcon
	}
	ui.Icon(c, icon).Size(15, 15).TextColor(col)
}
