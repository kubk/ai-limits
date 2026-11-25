package limits

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Amp runs `amp usage`, which prints the paid tier's two monthly pools:
//
//	**Amp Megawatt Tier:** agent usage $19.39 of $20.00 remaining (97%), orb usage
//	504h of 750h a1.small orb hours remaining (67%) - period 2026-11-09 to 2026-12-09, …
//
// There is no JSON output, so the line is parsed.
func Amp(ctx context.Context) ([]Window, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	Prepare()
	bin, err := exec.LookPath("amp")
	if err != nil {
		return nil, fmt.Errorf("amp %w", ErrNotInstalled)
	}
	cmd := exec.CommandContext(ctx, bin, "usage")
	cmd.Dir = os.TempDir()
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	stderr := &limitedWriter{n: 4096}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("timed out waiting for the CLI")
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, errors.New(stderr.lastLine())
		}
		return nil, err
	}
	s := string(out)
	if !strings.Contains(s, "Signed in as") {
		return nil, fmt.Errorf("sign in with `amp login` to see its limits")
	}
	return ampWindows(s), nil
}

var (
	ampAgent  = regexp.MustCompile(`agent usage \$([\d,.]+) of \$([\d,.]+) remaining`)
	ampOrb    = regexp.MustCompile(`orb usage ([\d,.]+)h of ([\d,.]+)h .*?remaining`)
	ampPeriod = regexp.MustCompile(`period (\d{4}-\d{2}-\d{2}) to (\d{4}-\d{2}-\d{2})`)
)

func ampWindows(s string) []Window {
	var start, end time.Time
	if m := ampPeriod.FindStringSubmatch(s); m != nil {
		start, _ = time.Parse(time.DateOnly, m[1])
		end, _ = time.Parse(time.DateOnly, m[2])
	}
	window := func(id, label string, m []string, unit string) (Window, bool) {
		if m == nil {
			return Window{}, false
		}
		left, err1 := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
		total, err2 := strconv.ParseFloat(strings.ReplaceAll(m[2], ",", ""), 64)
		if err1 != nil || err2 != nil || total <= 0 {
			return Window{}, false
		}
		w := Window{ID: id, Label: label, UsedPercent: clampPercent(100 - left/total*100), ResetsAt: end}
		// Orb time shows in hours; agent usage as a percent, like the rest.
		if unit == "h" {
			w.Amount = hours(left)
		}
		if !start.IsZero() && end.After(start) {
			w.DurationMins = int(end.Sub(start).Minutes())
		}
		return w, true
	}
	var out []Window
	if w, ok := window("orb", "Orb time", ampOrb.FindStringSubmatch(s), "h"); ok {
		out = append(out, w)
	}
	if w, ok := window("agent", "Agent", ampAgent.FindStringSubmatch(s), "$"); ok {
		out = append(out, w)
	}
	return out
}

// hours renders whole hours, rounded down so as not to promise more than
// is left, and tenths only under an hour: "503h", "0.6h".
func hours(h float64) string {
	if h < 1 {
		return strconv.FormatFloat(math.Floor(h*10)/10, 'f', -1, 64) + "h"
	}
	return strconv.FormatFloat(math.Floor(h), 'f', 0, 64) + "h"
}
