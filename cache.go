package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"ai-limits/limits"

	"github.com/egoist/mygo"
)

// The last numbers read, kept on disk so that a fresh launch shows them at
// once while the first read runs.

// cachePath is in the app's cache directory, which MyGo creates.
func cachePath() string {
	dir, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "last.json")
}

func loadCache() map[string][]limits.Window {
	windows := map[string][]limits.Window{}
	if b, err := os.ReadFile(cachePath()); err == nil {
		json.Unmarshal(b, &windows)
	}
	return windows
}

func saveCache(windows map[string][]limits.Window) {
	b, err := json.Marshal(windows)
	if err != nil {
		return
	}
	p := cachePath()
	if p == "" {
		return
	}
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, p)
	}
}
