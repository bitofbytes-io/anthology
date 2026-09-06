package shelves

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
)

type inMemoryRepository struct {
	mu         sync.RWMutex
	shelves    map[uuid.UUID]Shelf
	rows       map[uuid.UUID][]ShelfRow
	columns    map[uuid.UUID][]ShelfColumn
	slots      map[uuid.UUID][]ShelfSlot
	placements map[uuid.UUID]map[uuid.UUID]ItemPlacement // shelfID -> itemID -> placement
}

// NewInMemoryRepository seeds an empty shelf repository.
func NewInMemoryRepository() Repository {
	return &inMemoryRepository{
		shelves:    make(map[uuid.UUID]Shelf),
		rows:       make(map[uuid.UUID][]ShelfRow),
		columns:    make(map[uuid.UUID][]ShelfColumn),
		slots:      make(map[uuid.UUID][]ShelfSlot),
		placements: make(map[uuid.UUID]map[uuid.UUID]ItemPlacement),
	}
}

func (m *inMemoryRepository) CreateShelf(ctx context.Context, shelf Shelf, rows []ShelfRow, columns []ShelfColumn, slots []ShelfSlot) (ShelfWithLayout, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.shelves[shelf.ID] = shelf
	m.rows[shelf.ID] = slices.Clone(rows)
	m.columns[shelf.ID] = slices.Clone(columns)
	m.slots[shelf.ID] = slices.Clone(slots)
	m.placements[shelf.ID] = make(map[uuid.UUID]ItemPlacement)

	return m.buildLayout(ctx, shelf.ID, shelf.OwnerID)
}

func (m *inMemoryRepository) ListShelves(ctx context.Context, ownerID uuid.UUID) ([]ShelfSummary, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summaries := make([]ShelfSummary, 0, len(m.shelves))
	for id, shelf := range m.shelves {
		// Filter by owner_id
		if shelf.OwnerID != ownerID {
			continue
		}
		placementMap := m.placements[id]
		placed := 0
		for _, placement := range placementMap {
			if placement.ShelfSlotID != nil {
				placed++
			}
		}
		summaries = append(summaries, ShelfSummary{
			Shelf:       shelf,
			ItemCount:   len(placementMap),
			PlacedCount: placed,
			SlotCount:   len(m.slots[id]),
		})
	}

	slices.SortFunc(summaries, func(a, b ShelfSummary) int {
		if a.Shelf.CreatedAt.Equal(b.Shelf.CreatedAt) {
			return 0
		}
		if a.Shelf.CreatedAt.After(b.Shelf.CreatedAt) {
			return -1
		}
		return 1
	})

	return summaries, nil
}

func (m *inMemoryRepository) GetShelf(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID) (ShelfWithLayout, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	shelf, ok := m.shelves[shelfID]
	if !ok {
		return ShelfWithLayout{}, ErrNotFound
	}
	// Check owner matches (return 404 to prevent enumeration attacks)
	if shelf.OwnerID != ownerID {
		return ShelfWithLayout{}, ErrNotFound
	}

	return m.buildLayout(ctx, shelfID, ownerID)
}

func (m *inMemoryRepository) SaveLayout(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, rows []ShelfRow, columns []ShelfColumn, slots []ShelfSlot, removedSlotIDs []uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	shelf, ok := m.shelves[shelfID]
	if !ok {
		return ErrNotFound
	}
	// Check owner matches (return 404 to prevent enumeration attacks)
	if shelf.OwnerID != ownerID {
		return ErrNotFound
	}

	m.rows[shelfID] = slices.Clone(rows)
	m.columns[shelfID] = slices.Clone(columns)
	m.slots[shelfID] = slices.Clone(slots)

	if len(removedSlotIDs) > 0 {
		removedSet := make(map[uuid.UUID]struct{}, len(removedSlotIDs))
		for _, id := range removedSlotIDs {
			removedSet[id] = struct{}{}
		}
		for itemID, placement := range m.placements[shelfID] {
			if placement.ShelfSlotID != nil {
				if _, removed := removedSet[*placement.ShelfSlotID]; removed {
					delete(m.placements[shelfID], itemID)
				}
			}
		}
	}

	return nil
}

func (m *inMemoryRepository) AssignItemToSlot(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, slotID uuid.UUID, itemID uuid.UUID) (ItemPlacement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	shelf, ok := m.shelves[shelfID]
	if !ok {
		return ItemPlacement{}, ErrNotFound
	}
	// Check owner matches (return 404 to prevent enumeration attacks)
	if shelf.OwnerID != ownerID {
		return ItemPlacement{}, ErrNotFound
	}

	var slotExists bool
	for _, slot := range m.slots[shelfID] {
		if slot.ID == slotID {
			slotExists = true
			break
		}
	}
	if !slotExists {
		return ItemPlacement{}, ErrSlotNotFound
	}

	// Delete any existing placements for this item across ALL shelves (not just this shelf)
	// to ensure an item can only be on one shelf at a time
	for sid := range m.placements {
		delete(m.placements[sid], itemID)
	}

	placement := ItemPlacement{
		ID:          uuid.New(),
		ItemID:      itemID,
		ShelfID:     shelfID,
		ShelfSlotID: &slotID,
		CreatedAt:   time.Now().UTC(),
	}

	if m.placements[shelfID] == nil {
		m.placements[shelfID] = make(map[uuid.UUID]ItemPlacement)
	}
	m.placements[shelfID][itemID] = placement

	return placement, nil
}

func (m *inMemoryRepository) RemoveItemFromSlot(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, slotID uuid.UUID, itemID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	shelf, ok := m.shelves[shelfID]
	if !ok {
		return ErrNotFound
	}
	// Check owner matches (return 404 to prevent enumeration attacks)
	if shelf.OwnerID != ownerID {
		return ErrNotFound
	}

	placement, ok := m.placements[shelfID][itemID]
	if !ok {
		return ErrSlotNotFound
	}
	if placement.ShelfSlotID == nil || *placement.ShelfSlotID != slotID {
		return ErrSlotNotFound
	}

	placement.ShelfSlotID = nil
	placement.CreatedAt = time.Now().UTC()
	m.placements[shelfID][itemID] = placement
	return nil
}

func (m *inMemoryRepository) ListPlacements(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID) ([]ItemPlacement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	shelf, ok := m.shelves[shelfID]
	if !ok {
		return nil, ErrNotFound
	}
	// Check owner matches (return 404 to prevent enumeration attacks)
	if shelf.OwnerID != ownerID {
		return nil, ErrNotFound
	}

	var placements []ItemPlacement
	for _, placement := range m.placements[shelfID] {
		placements = append(placements, placement)
	}
	return placements, nil
}

func (m *inMemoryRepository) UpsertUnplaced(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID, itemID uuid.UUID) (ItemPlacement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	shelf, ok := m.shelves[shelfID]
	if !ok {
		return ItemPlacement{}, ErrNotFound
	}
	// Check owner matches (return 404 to prevent enumeration attacks)
	if shelf.OwnerID != ownerID {
		return ItemPlacement{}, ErrNotFound
	}

	placement := ItemPlacement{
		ID:          uuid.New(),
		ItemID:      itemID,
		ShelfID:     shelfID,
		ShelfSlotID: nil,
		CreatedAt:   time.Now().UTC(),
	}
	if existing, ok := m.placements[shelfID][itemID]; ok {
		placement.ID = existing.ID
	}
	if m.placements[shelfID] == nil {
		m.placements[shelfID] = make(map[uuid.UUID]ItemPlacement)
	}
	m.placements[shelfID][itemID] = placement
	return placement, nil
}

func (m *inMemoryRepository) buildLayout(ctx context.Context, shelfID uuid.UUID, ownerID uuid.UUID) (ShelfWithLayout, error) {
	shelf := m.shelves[shelfID]
	rows := slices.Clone(m.rows[shelfID])
	cols := slices.Clone(m.columns[shelfID])
	slots := slices.Clone(m.slots[shelfID])

	rowColumns := make(map[uuid.UUID][]ShelfColumn)
	for _, col := range cols {
		rowColumns[col.ShelfRowID] = append(rowColumns[col.ShelfRowID], col)
	}

	var rowWithColumns []RowWithColumns
	for _, row := range rows {
		rowWithColumns = append(rowWithColumns, RowWithColumns{ShelfRow: row, Columns: rowColumns[row.ID]})
	}

	var placements []PlacementWithItem
	for _, placement := range m.placements[shelfID] {
		placements = append(placements, PlacementWithItem{Placement: placement})
	}

	return ShelfWithLayout{
		Shelf:      shelf,
		Rows:       rowWithColumns,
		Slots:      slots,
		Placements: placements,
	}, nil
}
