package spatial

import (
	"errors"
	"fmt"
	"sort"
)

var (
	// ErrEmptyEntityID reports an entity operation without an ID.
	ErrEmptyEntityID = errors.New("empty spatial entity id")
	// ErrEntityAlreadyIndexed reports an insert for an entity that is already indexed.
	ErrEntityAlreadyIndexed = errors.New("spatial entity already indexed")
	// ErrEntityNotIndexed reports an update for an entity that is not indexed.
	ErrEntityNotIndexed = errors.New("spatial entity not indexed")
)

// EntityID is the stable identifier used by the spatial index.
type EntityID string

// QueryResult is the position-only result returned by radius queries.
type QueryResult struct {
	ID       EntityID
	Position Position
}

// Index stores entity positions in fixed-size spatial hash cells.
//
// Index is intended to be owned by a single world worker goroutine. It does not
// perform locking and does not apply gameplay visibility rules.
type Index struct {
	cellSize float64
	entities map[EntityID]indexedEntity
	cells    map[Cell]map[EntityID]struct{}
}

type indexedEntity struct {
	position Position
	cell     Cell
}

// NewIndex returns an empty fixed-cell spatial index.
func NewIndex(cellSize float64) (*Index, error) {
	if err := validateCellSize(cellSize); err != nil {
		return nil, err
	}
	return &Index{
		cellSize: cellSize,
		entities: make(map[EntityID]indexedEntity),
		cells:    make(map[Cell]map[EntityID]struct{}),
	}, nil
}

// CellSize returns the index cell size in world units.
func (idx *Index) CellSize() float64 {
	if idx == nil {
		return 0
	}
	return idx.cellSize
}

// Insert adds an entity at position.
func (idx *Index) Insert(id EntityID, position Position) error {
	if id == "" {
		return ErrEmptyEntityID
	}
	if err := position.Validate(); err != nil {
		return err
	}
	if _, exists := idx.entities[id]; exists {
		return fmt.Errorf("entity %q: %w", id, ErrEntityAlreadyIndexed)
	}

	cell := CellCoord(position, idx.cellSize)
	idx.entities[id] = indexedEntity{position: position, cell: cell}
	idx.addToCell(id, cell)
	return nil
}

// Update moves an existing entity to position and updates cell membership when
// the position crosses a cell boundary.
func (idx *Index) Update(id EntityID, position Position) error {
	if id == "" {
		return ErrEmptyEntityID
	}
	if err := position.Validate(); err != nil {
		return err
	}

	entity, exists := idx.entities[id]
	if !exists {
		return fmt.Errorf("entity %q: %w", id, ErrEntityNotIndexed)
	}

	nextCell := CellCoord(position, idx.cellSize)
	if nextCell != entity.cell {
		idx.removeFromCell(id, entity.cell)
		idx.addToCell(id, nextCell)
	}
	idx.entities[id] = indexedEntity{position: position, cell: nextCell}
	return nil
}

// Remove deletes an entity from the index. It returns false when the entity was
// not present.
func (idx *Index) Remove(id EntityID) bool {
	entity, exists := idx.entities[id]
	if !exists {
		return false
	}

	delete(idx.entities, id)
	idx.removeFromCell(id, entity.cell)
	return true
}

// QueryRadius returns entities within radius of center after an exact squared
// distance check. Results are sorted by entity ID for deterministic AOI diffs.
func (idx *Index) QueryRadius(center Position, radius float64) ([]QueryResult, error) {
	cells, err := cellsForRadius(center, radius, idx.cellSize)
	if err != nil {
		return nil, err
	}

	radiusSquared := radius * radius
	results := make([]QueryResult, 0)
	for _, cell := range cells {
		for id := range idx.cells[cell] {
			entity := idx.entities[id]
			if distanceSquared(center, entity.position) <= radiusSquared {
				results = append(results, QueryResult{ID: id, Position: entity.position})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results, nil
}

// QueryWindow returns entities inside a square window centered on center.
// halfExtent is measured from center to each side. Results are sorted by entity
// ID for deterministic AOI diffs.
func (idx *Index) QueryWindow(center Position, halfExtent float64) ([]QueryResult, error) {
	cells, err := cellsForWindow(center, halfExtent, idx.cellSize)
	if err != nil {
		return nil, err
	}

	minX := center.X - halfExtent
	maxX := center.X + halfExtent
	minY := center.Y - halfExtent
	maxY := center.Y + halfExtent
	results := make([]QueryResult, 0)
	for _, cell := range cells {
		for id := range idx.cells[cell] {
			entity := idx.entities[id]
			if entity.position.X >= minX && entity.position.X <= maxX && entity.position.Y >= minY && entity.position.Y <= maxY {
				results = append(results, QueryResult{ID: id, Position: entity.position})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results, nil
}

func (idx *Index) addToCell(id EntityID, cell Cell) {
	if idx.cells[cell] == nil {
		idx.cells[cell] = make(map[EntityID]struct{})
	}
	idx.cells[cell][id] = struct{}{}
}

func (idx *Index) removeFromCell(id EntityID, cell Cell) {
	cellMembers := idx.cells[cell]
	if cellMembers == nil {
		return
	}
	delete(cellMembers, id)
	if len(cellMembers) == 0 {
		delete(idx.cells, cell)
	}
}
