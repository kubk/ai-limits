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
	results  map[string]limits.Result
	loading  bool
	lastLoad time.Time // when the last read started, for minRefreshGap
	updated  time.Time // when the numbers shown were read
}

var providers = []struct {
	id, name string
	read     func(context.Context) limits.Result
}{
	{"codex", "Codex", limits.Codex},
	{"claude", "Claude", limits.Claude},
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
		var wg sync.WaitGroup
		out := make([]limits.Result, len(providers))
		for i, p := range providers {
			wg.Go(func() { out[i] = p.read(context.Background()) })
		}
		wg.Wait()
		mygo.RunOnMain(func() {
			for _, r := range out {
				prev := a.results[r.Provider]
				// Keep the last good numbers through a transient failure.
				if r.Err != nil && len(prev.Windows) > 0 && !errors.Is(r.Err, limits.ErrNotInstalled) {
					prev.Err = r.Err
					r = prev
				}
				a.results[r.Provider] = r
			}
			a.loading = false
			a.updated = time.Now()
			saveCache(a.results, a.updated)
			a.redraw()
		})
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
	a := &app{results: map[string]limits.Result{}}
	if c, ok := loadCache(); ok {
		a.results, a.updated = c.Results, c.SavedAt
	}
	mygo.App.SetName("AI Limits")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	// The popover is the app's only window: keep running as it closes.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(a.ready)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
