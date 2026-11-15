// AI Limits is a menu bar app: a click on its icon opens a popover with the
// usage limits of Codex and Claude, read through their own CLIs.
package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"maps"
	"sync"
	"time"

	"ai-limits/limits"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// minRefreshGap stops quick repeated opens or Refresh clicks from
// hammering the CLIs. Limits are read only when the popover opens or on
// Refresh: there is no timer and no read at launch.
const minRefreshGap = 10 * time.Second

type app struct {
	tray  *mygo.Tray
	panel *mygo.Window // the open popover, or nil

	// Read and written on the main thread only.
	windows  map[string][]limits.Window // the last numbers read, by provider
	errs     map[string]error           // the last read's failure, by provider
	loading  bool
	lastLoad time.Time // when the last read started, for minRefreshGap
}

// provider is one service the popover shows, with its look in T3 Code
// (usageProviders.ts, Icons.tsx).
type provider struct {
	id, name    string
	read        func(context.Context) ([]limits.Window, error)
	light, dark ui.Color // bars fill with it at 35%, as T3's
	icon        *ui.SVG
}

var providers = []provider{
	// Codex draws in the contrast foreground.
	{"codex", "Codex", limits.Codex, ui.Hex("#18181B"), ui.RGB(255, 255, 255), openaiIcon},
	{"claude", "Claude", limits.Claude, ui.Hex("#D97757"), ui.Hex("#D97757"), claudeIcon},
}

func (p provider) color(dark bool) ui.Color {
	if dark {
		return p.dark
	}
	return p.light
}

// refresh reads both providers in parallel, off the main thread.
func (a *app) refresh() {
	if a.loading {
		return
	}
	if time.Since(a.lastLoad) < minRefreshGap {
		return
	}
	a.loading = true
	a.lastLoad = time.Now()
	a.redraw()
	go func() {
		type read struct {
			windows []limits.Window
			err     error
		}
		out := make([]read, len(providers))
		var wg sync.WaitGroup
		for i, p := range providers {
			wg.Go(func() {
				w, err := p.read(context.Background())
				out[i] = read{w, err}
			})
		}
		wg.Wait()
		var snapshot map[string][]limits.Window
		mygo.RunOnMain(func() {
			for i, p := range providers {
				if r := out[i]; r.err != nil {
					// Earlier numbers stay through a failure, unless the
					// CLI is gone.
					a.errs[p.id] = r.err
					if errors.Is(r.err, limits.ErrNotInstalled) {
						delete(a.windows, p.id)
					}
				} else {
					a.windows[p.id] = r.windows
					delete(a.errs, p.id)
				}
			}
			a.loading = false
			a.redraw()
			snapshot = maps.Clone(a.windows)
		})
		saveCache(snapshot) // off the main thread
	}()
}

func (a *app) redraw() {
	if a.panel != nil {
		a.panel.Invalidate()
	}
}

func (a *app) toggle() {
	if a.panel != nil {
		a.panel.Close()
		return
	}
	a.panel = mygo.NewFlyout(mygo.FlyoutOptions{
		Tray:       a.tray,
		Placement:  mygo.PlacementBottom,
		Width:      popoverWidth,
		Height:     300,
		FitContent: true,
		MinWidth:   popoverWidth,
		MaxWidth:   popoverWidth,
		Focusable:  true,
		Popover:    true,
		Content:    ui.View(a.view),
	})
	a.panel.OnClosed(func() { a.panel = nil })
	a.refresh()
}

func (a *app) ready() {
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{
		Icon:           trayIcon(),
		IconIsTemplate: true,
		ToolTip:        "AI Limits",
	})
	if err != nil {
		log.Fatal("tray: ", err)
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Label: "Show Limits", Click: func(*mygo.MenuItem, *mygo.Window) { a.toggle() }},
		{Label: "Refresh", Click: func(*mygo.MenuItem, *mygo.Window) { a.refresh() }},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	})
	a.tray.OnClick(a.toggle)
	a.tray.OnRightClick(func() {
		if a.panel != nil {
			a.panel.Close()
		}
		// The menu for this popup only, so that left clicks keep toggling
		// the popover.
		a.tray.SetMenu(menu)
		defer a.tray.SetMenu(nil)
		a.tray.PopUpMenu()
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.tray.Destroy() })
	// The login shell's PATH, ready before the first click needs it.
	go limits.Prepare()
}

// trayIcon draws three bars of different lengths, black on transparent
// for macOS to tint: the three meters of the popover.
func trayIcon() []byte {
	const s = 36 // 18pt at 2x
	img := image.NewNRGBA(image.Rect(0, 0, s, s))
	ink := image.NewUniform(color.NRGBA{A: 255})
	for i, w := range []int{28, 20, 24} {
		y := 7 + i*9
		draw.Draw(img, image.Rect(4, y, 4+w, y+5), ink, image.Point{}, draw.Src)
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func main() {
	mygo.App.SetName("AI Limits") // before loadCache, which is under the app's name
	a := &app{windows: loadCache(), errs: map[string]error{}}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	// The popover is the app's only window: keep running as it closes.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
