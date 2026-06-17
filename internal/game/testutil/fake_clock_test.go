package testutil

import (
	"testing"
	"time"
)

func TestFakeClockCanAdvanceDeterministically(t *testing.T) {
	start := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	clock := NewFakeClock(start)

	if got := clock.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %s, want %s", got, start)
	}

	first := clock.Advance(250 * time.Millisecond)
	wantFirst := start.Add(250 * time.Millisecond)
	if !first.Equal(wantFirst) {
		t.Fatalf("Advance() = %s, want %s", first, wantFirst)
	}
	if got := clock.Now(); !got.Equal(wantFirst) {
		t.Fatalf("Now() after first advance = %s, want %s", got, wantFirst)
	}

	second := clock.Advance(2 * time.Second)
	wantSecond := wantFirst.Add(2 * time.Second)
	if !second.Equal(wantSecond) {
		t.Fatalf("second Advance() = %s, want %s", second, wantSecond)
	}
	if got := clock.Now(); !got.Equal(wantSecond) {
		t.Fatalf("Now() after second advance = %s, want %s", got, wantSecond)
	}
}
