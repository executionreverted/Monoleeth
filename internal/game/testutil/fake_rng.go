package testutil

import (
	"fmt"
	"sync"

	"gameproject/internal/game/foundation"
)

var _ foundation.RNG = (*FakeRNG)(nil)

// FakeRNG returns caller-configured random values in deterministic order.
type FakeRNG struct {
	mu     sync.Mutex
	ints   []int
	floats []float64
}

// NewFakeRNG creates a deterministic RNG with queued Intn and Float64 values.
func NewFakeRNG(ints []int, floats []float64) *FakeRNG {
	return &FakeRNG{
		ints:   append([]int(nil), ints...),
		floats: append([]float64(nil), floats...),
	}
}

// Intn returns the next configured integer value.
func (rng *FakeRNG) Intn(n int) int {
	if n <= 0 {
		panic("fake rng: Intn called with non-positive bound")
	}

	rng.mu.Lock()
	defer rng.mu.Unlock()

	if len(rng.ints) == 0 {
		panic("fake rng: no configured Intn values")
	}

	value := rng.ints[0]
	rng.ints = rng.ints[1:]
	if value < 0 || value >= n {
		panic(fmt.Sprintf("fake rng: configured Intn value %d outside [0,%d)", value, n))
	}
	return value
}

// Float64 returns the next configured float value.
func (rng *FakeRNG) Float64() float64 {
	rng.mu.Lock()
	defer rng.mu.Unlock()

	if len(rng.floats) == 0 {
		panic("fake rng: no configured Float64 values")
	}

	value := rng.floats[0]
	rng.floats = rng.floats[1:]
	if value < 0 || value >= 1 {
		panic(fmt.Sprintf("fake rng: configured Float64 value %g outside [0,1)", value))
	}
	return value
}
