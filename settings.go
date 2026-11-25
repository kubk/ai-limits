package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo"
)

// settings are the choices kept between launches, in the app's user data
// directory.
type settings struct {
	// Order is the provider ids, top first, as set on the settings screen.
	Order []string `json:"order,omitempty"`
}

func settingsPath() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "settings.json")
}

func loadSettings() settings {
	var s settings
	if b, err := os.ReadFile(settingsPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func saveSettings(s settings) {
	p := settingsPath()
	if p == "" {
		return
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, p)
	}
}

// ordered is the providers in the order set, with any the order does not
// name (one added since) after the rest, in their default order.
func ordered(order []string) []provider {
	out := make([]provider, 0, len(providers))
	for _, id := range order {
		if i := slices.IndexFunc(providers, func(p provider) bool { return p.id == id }); i >= 0 &&
			!slices.ContainsFunc(out, func(p provider) bool { return p.id == id }) {
			out = append(out, providers[i])
		}
	}
	for _, p := range providers {
		if !slices.ContainsFunc(out, func(q provider) bool { return q.id == p.id }) {
			out = append(out, p)
		}
	}
	return out
}

// move swaps the provider at i with its neighbor by delta (-1 up, +1 down)
// and saves the new order through a.save.
func (a *app) move(i, delta int) {
	list := ordered(a.settings.Order)
	j := i + delta
	if i < 0 || j < 0 || i >= len(list) || j >= len(list) {
		return
	}
	list[i], list[j] = list[j], list[i]
	order := make([]string, len(list))
	for k, p := range list {
		order[k] = p.id
	}
	a.settings.Order = order
	if a.save != nil {
		a.save(settings{Order: slices.Clone(order)})
	}
}
