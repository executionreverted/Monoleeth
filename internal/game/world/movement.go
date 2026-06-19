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

// MovementPositionAt returns the server-timed position for movement at the
// supplied authoritative time.
func MovementPositionAt(movement MovementState, at time.Time) (Vec2, bool) {
	if !movement.Moving || movement.Validate() != nil {
		return movement.Origin, false
	}
	if movement.ArriveAtMS <= movement.StartedAtMS {
		return movement.Target, true
	}

	nowMS := at.UTC().UnixMilli()
	if nowMS <= movement.StartedAtMS {
		return movement.Origin, false
	}
	if nowMS >= movement.ArriveAtMS {
		return movement.Target, true
	}

	progress := float64(nowMS-movement.StartedAtMS) / float64(movement.ArriveAtMS-movement.StartedAtMS)
	if progress <= 0 {
		return movement.Origin, false
	}
	if progress >= 1 {
		return movement.Target, true
	}

	return Vec2{
		X: movement.Origin.X + (movement.Target.X-movement.Origin.X)*progress,
		Y: movement.Origin.Y + (movement.Target.Y-movement.Origin.Y)*progress,
	}, false
}
