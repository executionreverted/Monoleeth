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

func TestFakeRNGDoesNotConsumeInvalidIntValue(t *testing.T) {
	rng := NewFakeRNG([]int{5, 2}, nil)

	assertPanics(t, func() {
		rng.Intn(3)
	})
	if got := rng.Intn(6); got != 5 {
		t.Fatalf("Intn after invalid bound panic = %d, want original queued value 5", got)
	}
	if got := rng.Intn(3); got != 2 {
		t.Fatalf("next Intn = %d, want 2", got)
	}
}

func TestFakeRNGDoesNotConsumeInvalidFloatValue(t *testing.T) {
	rng := NewFakeRNG(nil, []float64{1.5})

	assertPanics(t, func() {
		rng.Float64()
	})
	assertPanics(t, func() {
		rng.Float64()
	})
}
