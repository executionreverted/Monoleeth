package testutil

import (
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

var _ foundation.Clock = (*FakeClock)(nil)

// FakeClock is a deterministic clock for gameplay tests.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock creates a fake clock pinned to start.
func NewFakeClock(start time.Time) *FakeClock {
	return &FakeClock{now: start}
}

// Now returns the fake clock's current time.
func (clock *FakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()

	return clock.now
}

// Advance moves the fake clock forward by duration and returns the new time.
func (clock *FakeClock) Advance(duration time.Duration) time.Time {
	if duration < 0 {
		panic(fmt.Sprintf("fake clock: negative advance %s", duration))
	}

	clock.mu.Lock()
	defer clock.mu.Unlock()

	clock.now = clock.now.Add(duration)
	return clock.now
}
