package spatial

import "testing"

func TestCellCoordUsesFloorForPositiveZeroAndNegativePositions(t *testing.T) {
	tests := []struct {
		name     string
		position Position
		cellSize float64
		want     Cell
	}{
		{
			name:     "zero",
			position: Position{X: 0, Y: 0},
			cellSize: 10,
			want:     Cell{X: 0, Y: 0},
		},
		{
			name:     "positive inside first cell",
			position: Position{X: 9.99, Y: 0.01},
			cellSize: 10,
			want:     Cell{X: 0, Y: 0},
		},
		{
			name:     "positive boundary",
			position: Position{X: 10, Y: 20},
			cellSize: 10,
			want:     Cell{X: 1, Y: 2},
		},
		{
			name:     "negative inside previous cell",
			position: Position{X: -0.01, Y: -9.99},
			cellSize: 10,
			want:     Cell{X: -1, Y: -1},
		},
		{
			name:     "negative boundary",
			position: Position{X: -10, Y: -20},
			cellSize: 10,
			want:     Cell{X: -1, Y: -2},
		},
		{
			name:     "negative past boundary",
			position: Position{X: -10.01, Y: -20.01},
			cellSize: 10,
			want:     Cell{X: -2, Y: -3},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CellCoord(tt.position, tt.cellSize); got != tt.want {
				t.Fatalf("CellCoord(%+v, %v) = %+v, want %+v", tt.position, tt.cellSize, got, tt.want)
			}
		})
	}
}
