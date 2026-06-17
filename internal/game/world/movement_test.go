package world

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestAdvanceMovementUsesServerSpeedAndTickDelta(t *testing.T) {
	got, done := AdvanceMovement(Vec2{X: 0, Y: 0}, Vec2{X: 100, Y: 0}, 12, 250*time.Millisecond)

	if done {
		t.Fatal("AdvanceMovement() done = true, want false")
	}
	assertVecNear(t, got, Vec2{X: 3, Y: 0})
}

func TestAdvanceMovementStopsAtTargetWithoutOvershoot(t *testing.T) {
	target := Vec2{X: 3, Y: 4}

	got, done := AdvanceMovement(Vec2{X: 0, Y: 0}, target, 100, time.Second)

	if !done {
		t.Fatal("AdvanceMovement() done = false, want true")
	}
	if got != target {
		t.Fatalf("AdvanceMovement() position = %+v, want exact target %+v", got, target)
	}
}

func TestAdvanceMovementTreatsNonPositiveSpeedAndDeltaAsNoMovement(t *testing.T) {
	current := Vec2{X: 1, Y: 1}
	target := Vec2{X: 11, Y: 1}

	tests := []struct {
		name  string
		speed float64
		delta time.Duration
	}{
		{name: "zero speed", speed: 0, delta: time.Second},
		{name: "negative speed", speed: -1, delta: time.Second},
		{name: "zero delta", speed: 10, delta: 0},
		{name: "negative delta", speed: 10, delta: -time.Second},
		{name: "nan speed", speed: math.NaN(), delta: time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, done := AdvanceMovement(current, target, test.speed, test.delta)
			if done {
				t.Fatal("AdvanceMovement() done = true, want false")
			}
			if got != current {
				t.Fatalf("AdvanceMovement() position = %+v, want no movement %+v", got, current)
			}
		})
	}
}

func TestMovementAPIDoesNotAcceptClientFinalPosition(t *testing.T) {
	intentType := reflect.TypeOf(MovementIntent{})
	if intentType.NumField() != 1 || intentType.Field(0).Name != "Target" {
		t.Fatalf("MovementIntent fields = %v, want only Target", exportedFieldNames(intentType))
	}

	for _, forbidden := range []string{
		"Position",
		"CurrentPosition",
		"ClientPosition",
		"FinalPosition",
		"NewPosition",
	} {
		if _, ok := intentType.FieldByName(forbidden); ok {
			t.Fatalf("MovementIntent exposes client-supplied %s", forbidden)
		}
	}

	advanceType := reflect.TypeOf(AdvanceMovement)
	expectedInputs := []reflect.Type{
		reflect.TypeOf(Vec2{}),
		reflect.TypeOf(Vec2{}),
		reflect.TypeOf(float64(0)),
		reflect.TypeOf(time.Duration(0)),
	}
	if advanceType.NumIn() != len(expectedInputs) {
		t.Fatalf("AdvanceMovement input count = %d, want %d", advanceType.NumIn(), len(expectedInputs))
	}
	for index, expected := range expectedInputs {
		if got := advanceType.In(index); got != expected {
			t.Fatalf("AdvanceMovement input %d = %s, want %s", index, got, expected)
		}
	}
	if advanceType.NumOut() != 2 || advanceType.Out(0) != reflect.TypeOf(Vec2{}) || advanceType.Out(1) != reflect.TypeOf(false) {
		t.Fatalf("AdvanceMovement outputs = (%s, %s), want (world.Vec2, bool)", advanceType.Out(0), advanceType.Out(1))
	}
}

func TestValidationRejectsInvalidIDsTypesAndPositions(t *testing.T) {
	validEntity := Entity{
		WorldID:  "world-1",
		ZoneID:   "zone-1",
		ID:       "entity-1",
		Type:     EntityTypePlayer,
		Position: Vec2{X: 1, Y: 2},
	}
	if err := validEntity.Validate(); err != nil {
		t.Fatalf("valid entity rejected: %v", err)
	}

	tests := []struct {
		name   string
		entity Entity
		want   error
	}{
		{
			name:   "empty world id",
			entity: withWorldID(validEntity, ""),
			want:   foundation.ErrEmptyID,
		},
		{
			name:   "invalid zone id",
			entity: withZoneID(validEntity, "zone:1"),
			want:   foundation.ErrInvalidID,
		},
		{
			name:   "empty entity id",
			entity: withEntityID(validEntity, ""),
			want:   foundation.ErrEmptyID,
		},
		{
			name:   "invalid entity type",
			entity: withEntityType(validEntity, EntityType("asteroid")),
			want:   ErrInvalidEntityType,
		},
		{
			name:   "invalid entity position",
			entity: withPosition(validEntity, Vec2{X: math.Inf(1), Y: 0}),
			want:   ErrInvalidPosition,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.entity.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("Entity.Validate() error = %v, want %v", err, test.want)
			}
		})
	}

	if err := (EntityType("")).Validate(); !errors.Is(err, ErrInvalidEntityType) {
		t.Fatalf("EntityType.Validate() error = %v, want %v", err, ErrInvalidEntityType)
	}
	if err := (Vec2{X: math.NaN(), Y: 0}).Validate(); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("Vec2.Validate() error = %v, want %v", err, ErrInvalidPosition)
	}
	if _, err := NewMovementIntent(Vec2{X: 0, Y: math.Inf(-1)}); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("NewMovementIntent() error = %v, want %v", err, ErrInvalidPosition)
	}
	if _, err := NewMovementState(Vec2{X: math.NaN(), Y: 0}); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("NewMovementState() error = %v, want %v", err, ErrInvalidPosition)
	}
}

func TestDistanceHelpersSupportAOIVisibilityReuse(t *testing.T) {
	a := Vec2{X: -1, Y: -2}
	b := Vec2{X: 2, Y: 2}

	if got := DistanceSquared(a, b); got != 25 {
		t.Fatalf("DistanceSquared() = %v, want 25", got)
	}
	if got := a.DistanceSquared(b); got != 25 {
		t.Fatalf("Vec2.DistanceSquared() = %v, want 25", got)
	}
	if got := Distance(a, b); got != 5 {
		t.Fatalf("Distance() = %v, want 5", got)
	}
	if got := a.Distance(b); got != 5 {
		t.Fatalf("Vec2.Distance() = %v, want 5", got)
	}
}

func assertVecNear(t *testing.T, got Vec2, want Vec2) {
	t.Helper()

	const tolerance = 1e-9
	if math.Abs(got.X-want.X) > tolerance || math.Abs(got.Y-want.Y) > tolerance {
		t.Fatalf("position = %+v, want %+v", got, want)
	}
}

func exportedFieldNames(structType reflect.Type) []string {
	fields := make([]string, 0, structType.NumField())
	for index := 0; index < structType.NumField(); index++ {
		field := structType.Field(index)
		if field.IsExported() {
			fields = append(fields, field.Name)
		}
	}
	return fields
}

func withWorldID(entity Entity, worldID WorldID) Entity {
	entity.WorldID = worldID
	return entity
}

func withZoneID(entity Entity, zoneID ZoneID) Entity {
	entity.ZoneID = zoneID
	return entity
}

func withEntityID(entity Entity, entityID EntityID) Entity {
	entity.ID = entityID
	return entity
}

func withEntityType(entity Entity, entityType EntityType) Entity {
	entity.Type = entityType
	return entity
}

func withPosition(entity Entity, position Vec2) Entity {
	entity.Position = position
	return entity
}
