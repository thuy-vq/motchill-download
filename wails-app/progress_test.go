package main

import (
	"testing"
	"time"
)

func TestClockSecondsParsesFFmpegStamps(t *testing.T) {
	cases := map[string]float64{
		"00:00:12.340000":  12.34,
		"01:02:03.00":      3723,
		"N/A":              -1,
		"-577014:32:22.71": -1,
		"":                 -1,
		"12.34":            -1,
	}
	for value, expected := range cases {
		if got := clockSeconds(value); got != expected {
			t.Fatalf("clockSeconds(%q) = %v, want %v", value, got, expected)
		}
	}
}

func TestProgressPercentFallsBackWhenDurationUnknown(t *testing.T) {
	if got := progressPercent(30, 0); got != -1 {
		t.Fatalf("percent without duration = %v, want -1", got)
	}
	if got := progressPercent(-1, 120); got != -1 {
		t.Fatalf("percent without position = %v, want -1", got)
	}
	if got := progressPercent(30, 120); got != 25 {
		t.Fatalf("percent = %v, want 25", got)
	}
	if got := progressPercent(130, 120); got != 100 {
		t.Fatalf("percent past the end = %v, want 100", got)
	}
}

func TestHumanClockFormatsPositions(t *testing.T) {
	cases := map[float64]string{0: "", -1: "", 65: "1:05", 3723: "1:02:03"}
	for seconds, expected := range cases {
		if got := humanClock(seconds); got != expected {
			t.Fatalf("humanClock(%v) = %q, want %q", seconds, got, expected)
		}
	}
}

func TestStallGuardReportsOnlyAfterAKill(t *testing.T) {
	guard := &stallGuard{last: time.Now(), done: make(chan struct{})}
	if guard.stop() {
		t.Fatal("a guard that never fired must not report a stall")
	}
	// stop is called once per download but must stay safe if reached twice.
	if guard.stop() {
		t.Fatal("second stop must keep reporting no stall")
	}
}
