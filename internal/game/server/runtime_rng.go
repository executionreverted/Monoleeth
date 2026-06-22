package server

import (
	"math/rand"
	"sync"
)

type runtimeRNG struct {
	mu  sync.Mutex
	src *rand.Rand
}

func newRuntimeRNG(seed int64) *runtimeRNG {
	return &runtimeRNG{src: rand.New(rand.NewSource(seed))}
}

func (rng *runtimeRNG) Intn(n int) int {
	rng.mu.Lock()
	defer rng.mu.Unlock()
	return rng.src.Intn(n)
}

func (rng *runtimeRNG) Float64() float64 {
	rng.mu.Lock()
	defer rng.mu.Unlock()
	return rng.src.Float64()
}
