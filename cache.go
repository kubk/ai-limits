package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"ai-limits/limits"
)

// The last good numbers, kept on disk so that a fresh launch shows them at
// once while the first read runs.
type cached struct {
	SavedAt time.Time                `json:"savedAt"`
	Results map[string]limits.Result `json:"results"`
}

func cachePath() string {
	dir, err := os.UserConfigDir() // ~/Library/Application Support
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "AI Limits", "last.json")
}

func loadCache() (cached, bool) {
	var c cached
	b, err := os.ReadFile(cachePath())
	if err != nil || json.Unmarshal(b, &c) != nil || len(c.Results) == 0 {
		return c, false
	}
	return c, true
}

// saveCache writes only results that have numbers, errors left out.
func saveCache(results map[string]limits.Result, at time.Time) {
	good := map[string]limits.Result{}
	for id, r := range results {
		if len(r.Windows) > 0 {
			r.Err = nil
			good[id] = r
		}
	}
	if len(good) == 0 {
		return
	}
	b, err := json.Marshal(cached{SavedAt: at, Results: good})
	if err != nil {
		return
	}
	p := cachePath()
	if os.MkdirAll(filepath.Dir(p), 0o700) != nil {
		return
	}
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, p)
	}
}
