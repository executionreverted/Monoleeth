package discovery

import (
	"errors"
	"fmt"
	"math"

	"gameproject/internal/game/world"
)

const (
	// DefaultChunkSize is the phase-08 procedural generation chunk size.
	DefaultChunkSize = 5_000
	// DefaultScanCellSize is the phase-08 scan pulse cell size.
	DefaultScanCellSize = 500
)

var (
	// ErrInvalidGridSize reports a zero, negative, NaN, or infinite grid size.
	ErrInvalidGridSize = errors.New("invalid discovery grid size")
)

// ChunkCoord identifies a procedural generation and overlay lookup chunk.
type ChunkCoord struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

// ScanCellCoord identifies a smaller scanner pulse cell.
type ScanCellCoord struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

// ChunkCoordForPosition returns the chunk containing pos.
func ChunkCoordForPosition(pos world.Vec2, chunkSize float64) (ChunkCoord, error) {
	if err := validateGridPosition(pos); err != nil {
		return ChunkCoord{}, err
	}
	if err := validateGridSize(chunkSize); err != nil {
		return ChunkCoord{}, err
	}
	return ChunkCoord{
		X: int64(math.Floor(pos.X / chunkSize)),
		Y: int64(math.Floor(pos.Y / chunkSize)),
	}, nil
}

// ScanCellCoordForPosition returns the scanner cell containing pos.
func ScanCellCoordForPosition(pos world.Vec2, scanCellSize float64) (ScanCellCoord, error) {
	if err := validateGridPosition(pos); err != nil {
		return ScanCellCoord{}, err
	}
	if err := validateGridSize(scanCellSize); err != nil {
		return ScanCellCoord{}, err
	}
	return ScanCellCoord{
		X: int64(math.Floor(pos.X / scanCellSize)),
		Y: int64(math.Floor(pos.Y / scanCellSize)),
	}, nil
}

// Center returns the world-space center of cell.
func (cell ScanCellCoord) Center(scanCellSize float64) (world.Vec2, error) {
	if err := validateGridSize(scanCellSize); err != nil {
		return world.Vec2{}, err
	}
	return world.Vec2{
		X: (float64(cell.X) + 0.5) * scanCellSize,
		Y: (float64(cell.Y) + 0.5) * scanCellSize,
	}, nil
}

func validateGridPosition(pos world.Vec2) error {
	return pos.Validate()
}

func validateGridSize(size float64) error {
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		return fmt.Errorf("grid size %v: %w", size, ErrInvalidGridSize)
	}
	return nil
}
