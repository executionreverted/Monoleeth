package spatial

import (
	"errors"
	"fmt"
	"math"
)

var (
	// ErrInvalidCellSize reports a zero, negative, NaN, or infinite cell size.
	ErrInvalidCellSize = errors.New("invalid spatial cell size")
	// ErrInvalidPosition reports a NaN or infinite spatial position.
	ErrInvalidPosition = errors.New("invalid spatial position")
	// ErrNegativeRadius reports a radius query with a negative radius.
	ErrNegativeRadius = errors.New("negative spatial query radius")
	// ErrNegativeHalfExtent reports a square window query with a negative half extent.
	ErrNegativeHalfExtent = errors.New("negative spatial query half extent")
)

// Position is a minimal 2D world coordinate used by the spatial package.
type Position struct {
	X float64
	Y float64
}

// Validate reports whether position contains finite coordinates.
func (pos Position) Validate() error {
	if math.IsNaN(pos.X) || math.IsNaN(pos.Y) || math.IsInf(pos.X, 0) || math.IsInf(pos.Y, 0) {
		return fmt.Errorf("position (%v, %v): %w", pos.X, pos.Y, ErrInvalidPosition)
	}
	return nil
}

// Cell identifies a fixed spatial hash bucket.
type Cell struct {
	X int
	Y int
}

// CellCoord returns the fixed-grid cell containing pos.
//
// It uses math.Floor so negative world coordinates map to the expected lower
// cell, for example x=-0.1 with cell size 10 maps to cell x=-1.
func CellCoord(pos Position, cellSize float64) Cell {
	return Cell{
		X: int(math.Floor(pos.X / cellSize)),
		Y: int(math.Floor(pos.Y / cellSize)),
	}
}

func validateCellSize(cellSize float64) error {
	if cellSize <= 0 || math.IsNaN(cellSize) || math.IsInf(cellSize, 0) {
		return fmt.Errorf("cell size %v: %w", cellSize, ErrInvalidCellSize)
	}
	return nil
}

func cellsForRadius(center Position, radius float64, cellSize float64) ([]Cell, error) {
	if err := center.Validate(); err != nil {
		return nil, err
	}
	if radius < 0 || math.IsNaN(radius) || math.IsInf(radius, 0) {
		return nil, fmt.Errorf("radius %v: %w", radius, ErrNegativeRadius)
	}
	if err := validateCellSize(cellSize); err != nil {
		return nil, err
	}

	minCell := CellCoord(Position{X: center.X - radius, Y: center.Y - radius}, cellSize)
	maxCell := CellCoord(Position{X: center.X + radius, Y: center.Y + radius}, cellSize)

	cells := make([]Cell, 0, (maxCell.X-minCell.X+1)*(maxCell.Y-minCell.Y+1))
	for y := minCell.Y; y <= maxCell.Y; y++ {
		for x := minCell.X; x <= maxCell.X; x++ {
			cells = append(cells, Cell{X: x, Y: y})
		}
	}
	return cells, nil
}

func cellsForWindow(center Position, halfExtent float64, cellSize float64) ([]Cell, error) {
	if err := center.Validate(); err != nil {
		return nil, err
	}
	if halfExtent < 0 || math.IsNaN(halfExtent) || math.IsInf(halfExtent, 0) {
		return nil, fmt.Errorf("half extent %v: %w", halfExtent, ErrNegativeHalfExtent)
	}
	if err := validateCellSize(cellSize); err != nil {
		return nil, err
	}

	minCell := CellCoord(Position{X: center.X - halfExtent, Y: center.Y - halfExtent}, cellSize)
	maxCell := CellCoord(Position{X: center.X + halfExtent, Y: center.Y + halfExtent}, cellSize)

	cells := make([]Cell, 0, (maxCell.X-minCell.X+1)*(maxCell.Y-minCell.Y+1))
	for y := minCell.Y; y <= maxCell.Y; y++ {
		for x := minCell.X; x <= maxCell.X; x++ {
			cells = append(cells, Cell{X: x, Y: y})
		}
	}
	return cells, nil
}

func distanceSquared(a Position, b Position) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return dx*dx + dy*dy
}
