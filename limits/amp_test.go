package limits

import "testing"

func TestHours(t *testing.T) {
	for in, want := range map[float64]string{503.6: "503h", 750: "750h", 1: "1h", 0.66: "0.6h", 0.04: "0h"} {
		if got := hours(in); got != want {
			t.Errorf("hours(%v) = %q, want %q", in, got, want)
		}
	}
}
