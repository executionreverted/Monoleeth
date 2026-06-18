package discovery

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

var (
	// ErrInvalidHashPurpose reports a missing deterministic hash domain.
	ErrInvalidHashPurpose = errors.New("invalid hash purpose")
)

// CellHash returns a deterministic server-side hash for one scan cell and
// purpose. Different purpose strings isolate generation domains.
func CellHash(seed WorldSeed, cell ScanCellCoord, purpose string) (uint64, error) {
	if !seed.Valid() {
		return 0, ErrInvalidWorldSeed
	}
	if purpose == "" {
		return 0, ErrInvalidHashPurpose
	}
	return cellHash64(seed.staticKey[:], cell, purpose, 0), nil
}

func indexedCellHash(seed WorldSeed, cell ScanCellCoord, purpose string, index int) (uint64, error) {
	if !seed.Valid() {
		return 0, ErrInvalidWorldSeed
	}
	if purpose == "" {
		return 0, ErrInvalidHashPurpose
	}
	if index < 0 {
		return 0, fmt.Errorf("hash index %d: %w", index, ErrInvalidHashPurpose)
	}
	return cellHash64(seed.staticKey[:], cell, purpose, uint64(index)), nil
}

func cellHash64(key []byte, cell ScanCellCoord, purpose string, index uint64) uint64 {
	mac := hmac.New(sha256.New, key)
	var buf [24]byte
	binary.LittleEndian.PutUint64(buf[0:8], uint64(cell.X))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(cell.Y))
	binary.LittleEndian.PutUint64(buf[16:24], index)
	_, _ = mac.Write(buf[:])
	_, _ = mac.Write([]byte(purpose))
	sum := mac.Sum(nil)
	return binary.LittleEndian.Uint64(sum[:8])
}

func unitFloatFromHash(hash uint64) float64 {
	return float64(hash>>11) / (1 << 53)
}
