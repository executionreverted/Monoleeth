package spatial

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestQueryRadiusReturnsNearbyEntities(t *testing.T) {
	idx := newTestIndex(t, 10)
	mustInsert(t, idx, "player", Position{X: 0, Y: 0})
	mustInsert(t, idx, "near-a", Position{X: 3, Y: 4})
	mustInsert(t, idx, "near-b", Position{X: -5, Y: 0})

	results, err := idx.QueryRadius(Position{X: 0, Y: 0}, 5)
	if err != nil {
		t.Fatalf("QueryRadius() = %v, want nil", err)
	}

	if got, want := resultIDs(results), []EntityID{"near-a", "near-b", "player"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryRadius ids = %v, want %v", got, want)
	}
}

func TestQueryRadiusExcludesFarEntitiesAfterExactDistanceCheck(t *testing.T) {
	idx := newTestIndex(t, 10)
	mustInsert(t, idx, "inside", Position{X: 6, Y: 8})
	mustInsert(t, idx, "same-cell-but-outside", Position{X: 9, Y: 9})
	mustInsert(t, idx, "outside-cell", Position{X: 20, Y: 0})

	results, err := idx.QueryRadius(Position{X: 0, Y: 0}, 10)
	if err != nil {
		t.Fatalf("QueryRadius() = %v, want nil", err)
	}

	if got, want := resultIDs(results), []EntityID{"inside"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryRadius ids = %v, want %v", got, want)
	}
}

func TestQueryWindowReturnsEntitiesInsideSquareWindow(t *testing.T) {
	idx := newTestIndex(t, 10)
	mustInsert(t, idx, "center", Position{X: 0, Y: 0})
	mustInsert(t, idx, "corner", Position{X: 10, Y: 10})
	mustInsert(t, idx, "edge", Position{X: -10, Y: 0})
	mustInsert(t, idx, "outside-x", Position{X: 10.1, Y: 0})
	mustInsert(t, idx, "outside-y", Position{X: 0, Y: -10.1})

	results, err := idx.QueryWindow(Position{X: 0, Y: 0}, 10)
	if err != nil {
		t.Fatalf("QueryWindow() = %v, want nil", err)
	}

	if got, want := resultIDs(results), []EntityID{"center", "corner", "edge"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryWindow ids = %v, want %v", got, want)
	}
}

func TestUpdateMovesEntityBetweenCells(t *testing.T) {
	idx := newTestIndex(t, 10)
	mustInsert(t, idx, "ship", Position{X: 0, Y: 0})

	if err := idx.Update("ship", Position{X: 25, Y: 0}); err != nil {
		t.Fatalf("Update() = %v, want nil", err)
	}

	oldResults, err := idx.QueryRadius(Position{X: 0, Y: 0}, 5)
	if err != nil {
		t.Fatalf("old QueryRadius() = %v, want nil", err)
	}
	if got := resultIDs(oldResults); len(got) != 0 {
		t.Fatalf("old QueryRadius ids = %v, want none", got)
	}

	newResults, err := idx.QueryRadius(Position{X: 25, Y: 0}, 1)
	if err != nil {
		t.Fatalf("new QueryRadius() = %v, want nil", err)
	}
	if got, want := resultIDs(newResults), []EntityID{"ship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("new QueryRadius ids = %v, want %v", got, want)
	}
}

func TestRemoveDeletesEntityFromFutureQueries(t *testing.T) {
	idx := newTestIndex(t, 10)
	mustInsert(t, idx, "ship", Position{X: 0, Y: 0})

	if removed := idx.Remove("ship"); !removed {
		t.Fatal("Remove() = false, want true")
	}
	if removed := idx.Remove("ship"); removed {
		t.Fatal("second Remove() = true, want false")
	}

	results, err := idx.QueryRadius(Position{X: 0, Y: 0}, 5)
	if err != nil {
		t.Fatalf("QueryRadius() = %v, want nil", err)
	}
	if got := resultIDs(results); len(got) != 0 {
		t.Fatalf("QueryRadius ids = %v, want none", got)
	}
}

func TestQueryRadiusReturnsDeterministicOrdering(t *testing.T) {
	idx := newTestIndex(t, 10)
	mustInsert(t, idx, "delta", Position{X: 3, Y: 0})
	mustInsert(t, idx, "alpha", Position{X: -3, Y: 0})
	mustInsert(t, idx, "charlie", Position{X: 0, Y: 3})
	mustInsert(t, idx, "bravo", Position{X: 0, Y: -3})

	want := []EntityID{"alpha", "bravo", "charlie", "delta"}
	for attempt := 0; attempt < 10; attempt++ {
		results, err := idx.QueryRadius(Position{X: 0, Y: 0}, 5)
		if err != nil {
			t.Fatalf("QueryRadius() attempt %d = %v, want nil", attempt, err)
		}
		if got := resultIDs(results); !reflect.DeepEqual(got, want) {
			t.Fatalf("QueryRadius ids attempt %d = %v, want %v", attempt, got, want)
		}
	}
}

func TestIndexRejectsInvalidInputs(t *testing.T) {
	if _, err := NewIndex(0); !errors.Is(err, ErrInvalidCellSize) {
		t.Fatalf("NewIndex(0) error = %v, want ErrInvalidCellSize", err)
	}

	idx := newTestIndex(t, 10)
	if err := idx.Insert("", Position{}); !errors.Is(err, ErrEmptyEntityID) {
		t.Fatalf("Insert(empty) error = %v, want ErrEmptyEntityID", err)
	}
	mustInsert(t, idx, "ship", Position{})
	if err := idx.Insert("ship", Position{}); !errors.Is(err, ErrEntityAlreadyIndexed) {
		t.Fatalf("Insert(duplicate) error = %v, want ErrEntityAlreadyIndexed", err)
	}
	if err := idx.Update("missing", Position{}); !errors.Is(err, ErrEntityNotIndexed) {
		t.Fatalf("Update(missing) error = %v, want ErrEntityNotIndexed", err)
	}
	if _, err := idx.QueryRadius(Position{}, -1); !errors.Is(err, ErrNegativeRadius) {
		t.Fatalf("QueryRadius(negative) error = %v, want ErrNegativeRadius", err)
	}
	if err := idx.Insert("nan", Position{X: math.NaN()}); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("Insert(nan) error = %v, want ErrInvalidPosition", err)
	}
	if err := idx.Update("ship", Position{X: math.Inf(1)}); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("Update(inf) error = %v, want ErrInvalidPosition", err)
	}
	if _, err := idx.QueryRadius(Position{Y: math.Inf(-1)}, 1); !errors.Is(err, ErrInvalidPosition) {
		t.Fatalf("QueryRadius(invalid center) error = %v, want ErrInvalidPosition", err)
	}
	if _, err := idx.QueryRadius(Position{}, math.Inf(1)); !errors.Is(err, ErrNegativeRadius) {
		t.Fatalf("QueryRadius(infinite radius) error = %v, want ErrNegativeRadius", err)
	}
	if _, err := idx.QueryWindow(Position{}, math.Inf(1)); !errors.Is(err, ErrNegativeHalfExtent) {
		t.Fatalf("QueryWindow(infinite half extent) error = %v, want ErrNegativeHalfExtent", err)
	}
}

func newTestIndex(t *testing.T, cellSize float64) *Index {
	t.Helper()

	idx, err := NewIndex(cellSize)
	if err != nil {
		t.Fatalf("NewIndex(%v) = %v, want nil", cellSize, err)
	}
	return idx
}

func mustInsert(t *testing.T, idx *Index, id EntityID, position Position) {
	t.Helper()

	if err := idx.Insert(id, position); err != nil {
		t.Fatalf("Insert(%q, %+v) = %v, want nil", id, position, err)
	}
}

func resultIDs(results []QueryResult) []EntityID {
	ids := make([]EntityID, 0, len(results))
	for _, result := range results {
		ids = append(ids, result.ID)
	}
	return ids
}
