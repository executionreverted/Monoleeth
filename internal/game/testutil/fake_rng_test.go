package testutil

import "testing"

func TestFakeRNGReturnsConfiguredDeterministicValues(t *testing.T) {
	rng := NewFakeRNG([]int{2, 0, 4}, []float64{0.25, 0.75})

	if got := rng.Intn(3); got != 2 {
		t.Fatalf("first Intn() = %d, want 2", got)
	}
	if got := rng.Intn(1); got != 0 {
		t.Fatalf("second Intn() = %d, want 0", got)
	}
	if got := rng.Float64(); got != 0.25 {
		t.Fatalf("first Float64() = %g, want 0.25", got)
	}
	if got := rng.Intn(5); got != 4 {
		t.Fatalf("third Intn() = %d, want 4", got)
	}
	if got := rng.Float64(); got != 0.75 {
		t.Fatalf("second Float64() = %g, want 0.75", got)
	}
}
