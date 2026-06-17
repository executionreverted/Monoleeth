package world

import (
	"math"
	"time"
)

// AdvanceMovement moves current toward target by server-owned speed and tick delta.
//
// It never accepts a client-supplied final position. Zero or negative speed and
// zero or negative delta are safe no-op inputs unless the entity is already at
// the target.
func AdvanceMovement(current Vec2, target Vec2, speed float64, delta time.Duration) (Vec2, bool) {
	if current.Validate() != nil || target.Validate() != nil {
		return current, false
	}

	remainingSquared := current.DistanceSquared(target)
	if remainingSquared == 0 {
		return target, true
	}
	if !isFinite(remainingSquared) || speed <= 0 || !isFinite(speed) || delta <= 0 {
		return current, false
	}

	maxStep := speed * delta.Seconds()
	if maxStep <= 0 || !isFinite(maxStep) {
		return current, false
	}

	remaining := math.Sqrt(remainingSquared)
	if maxStep >= remaining {
		return target, true
	}

	scale := maxStep / remaining
	return Vec2{
		X: current.X + (target.X-current.X)*scale,
		Y: current.Y + (target.Y-current.Y)*scale,
	}, false
}
