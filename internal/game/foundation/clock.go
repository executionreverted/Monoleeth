package foundation

import "time"

// Clock provides server-owned time for gameplay code.
type Clock interface {
	Now() time.Time
}

// RealClock reads the current wall clock time.
type RealClock struct{}

// Now returns the current UTC time.
func (RealClock) Now() time.Time {
	return time.Now().UTC()
}
