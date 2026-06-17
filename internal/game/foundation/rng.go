package foundation

// RNG provides server-owned random values for gameplay code.
type RNG interface {
	Intn(n int) int
	Float64() float64
}
