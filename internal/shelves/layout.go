package shelves

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

func removedSlots(previous, next []ShelfSlot) []uuid.UUID {
	nextSet := make(map[uuid.UUID]struct{}, len(next))
	for _, slot := range next {
		nextSet[slot.ID] = struct{}{}
	}

	var removed []uuid.UUID
	for _, slot := range previous {
		if _, exists := nextSet[slot.ID]; !exists {
			removed = append(removed, slot.ID)
		}
	}
	return removed
}

func normalizeSlots(
	slots []LayoutSlotInput,
	shelfID uuid.UUID,
	existingRowIDs map[int]uuid.UUID,
	existingColumnIDs map[string]uuid.UUID,
	existingSlotIDs map[string]uuid.UUID,
	existingSlotIDSet map[uuid.UUID]struct{},
) ([]ShelfRow, []ShelfColumn, []ShelfSlot, error) {
	if len(slots) == 0 {
		return []ShelfRow{}, []ShelfColumn{}, []ShelfSlot{}, nil
	}

	key := func(rowIdx, colIdx int) string {
		return fmt.Sprintf("%d-%d", rowIdx, colIdx)
	}

	rowGroups := make(map[int][]LayoutSlotInput)
	seenKeys := make(map[string]struct{})
	explicitIDs := make(map[uuid.UUID]struct{})
	for _, slot := range slots {
		if slot.SlotID != nil {
			if slot.NewSlot {
				return nil, nil, nil, fmt.Errorf("%w: newSlot cannot include slotId", ErrValidation)
			}
			if _, exists := explicitIDs[*slot.SlotID]; exists {
				return nil, nil, nil, fmt.Errorf("%w: duplicate slotId", ErrValidation)
			}
			explicitIDs[*slot.SlotID] = struct{}{}
		}
	}

	for _, slot := range slots {
		if slot.RowIndex < 0 || slot.ColIndex < 0 {
			return nil, nil, nil, fmt.Errorf("%w: row and column indexes must be non-negative", ErrValidation)
		}
		if slot.XStartNorm < 0 || slot.XEndNorm > 1 || slot.XEndNorm <= slot.XStartNorm {
			return nil, nil, nil, fmt.Errorf("%w: slot %d/%d has invalid x boundaries", ErrValidation, slot.RowIndex, slot.ColIndex)
		}
		if slot.YStartNorm < 0 || slot.YEndNorm > 1 || slot.YEndNorm <= slot.YStartNorm {
			return nil, nil, nil, fmt.Errorf("%w: slot %d/%d has invalid y boundaries", ErrValidation, slot.RowIndex, slot.ColIndex)
		}
		slotKey := key(slot.RowIndex, slot.ColIndex)
		if _, exists := seenKeys[slotKey]; exists {
			return nil, nil, nil, fmt.Errorf("%w: duplicate definition for row %d column %d", ErrValidation, slot.RowIndex, slot.ColIndex)
		}
		seenKeys[slotKey] = struct{}{}
		rowGroups[slot.RowIndex] = append(rowGroups[slot.RowIndex], slot)
	}

	rowIndexes := make([]int, 0, len(rowGroups))
	for idx := range rowGroups {
		rowIndexes = append(rowIndexes, idx)
	}
	sort.Ints(rowIndexes)

	rows := make([]ShelfRow, 0, len(rowIndexes))
	columns := make([]ShelfColumn, 0, len(slots))
	normalizedSlots := make([]ShelfSlot, 0, len(slots))

	for _, rowIdx := range rowIndexes {
		rowSlots := rowGroups[rowIdx]
		sort.Slice(rowSlots, func(i, j int) bool { return rowSlots[i].ColIndex < rowSlots[j].ColIndex })

		rowYStart := rowSlots[0].YStartNorm
		rowYEnd := rowSlots[0].YEndNorm
		for _, slot := range rowSlots[1:] {
			if slot.YStartNorm < rowYStart {
				rowYStart = slot.YStartNorm
			}
			if slot.YEndNorm > rowYEnd {
				rowYEnd = slot.YEndNorm
			}
		}

		rowID, ok := existingRowIDs[rowIdx]
		if !ok {
			rowID = uuid.New()
		}
		rows = append(rows, ShelfRow{
			ID:         rowID,
			ShelfID:    shelfID,
			RowIndex:   rowIdx,
			YStartNorm: rowYStart,
			YEndNorm:   rowYEnd,
		})

		for _, slot := range rowSlots {
			colKey := key(rowIdx, slot.ColIndex)
			colID, ok := existingColumnIDs[colKey]
			if !ok {
				colID = uuid.New()
			}

			columns = append(columns, ShelfColumn{
				ID:         colID,
				ShelfRowID: rowID,
				ColIndex:   slot.ColIndex,
				XStartNorm: slot.XStartNorm,
				XEndNorm:   slot.XEndNorm,
			})

			slotID := uuid.New()
			if slot.SlotID != nil {
				if _, ok := existingSlotIDSet[*slot.SlotID]; !ok {
					return nil, nil, nil, fmt.Errorf("%w: slotId %s does not belong to this shelf", ErrValidation, slot.SlotID.String())
				}
				slotID = *slot.SlotID
			} else if existingID, ok := existingSlotIDs[colKey]; ok && !slot.NewSlot {
				// Legacy clients omit IDs for unchanged slots. Do not steal an ID
				// explicitly retained elsewhere in a reordered layout.
				if _, reserved := explicitIDs[existingID]; !reserved {
					slotID = existingID
				}
			}

			normalizedSlots = append(normalizedSlots, ShelfSlot{
				ID:            slotID,
				ShelfID:       shelfID,
				ShelfRowID:    rowID,
				ShelfColumnID: colID,
				RowIndex:      rowIdx,
				ColIndex:      slot.ColIndex,
				XStartNorm:    slot.XStartNorm,
				XEndNorm:      slot.XEndNorm,
				YStartNorm:    slot.YStartNorm,
				YEndNorm:      slot.YEndNorm,
			})
		}
	}

	return rows, columns, normalizedSlots, nil
}
