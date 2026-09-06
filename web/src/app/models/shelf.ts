import { Item } from './item';

export interface Shelf {
    id: string;
    name: string;
    description: string;
    photoUrl: string;
    createdAt: string;
    updatedAt: string;
}

export interface ShelfRow {
    id: string;
    shelfId: string;
    rowIndex: number;
    yStartNorm: number;
    yEndNorm: number;
    columns?: ShelfColumn[];
}

export interface ShelfColumn {
    id: string;
    shelfRowId: string;
    colIndex: number;
    xStartNorm: number;
    xEndNorm: number;
}

export interface ShelfSlot {
    id: string;
    shelfId: string;
    shelfRowId: string;
    shelfColumnId: string;
    rowIndex: number;
    colIndex: number;
    xStartNorm: number;
    xEndNorm: number;
    yStartNorm: number;
    yEndNorm: number;
}

export interface PlacementRecord {
    id: string;
    itemId: string;
    shelfId: string;
    shelfSlotId: string | null;
    createdAt: string;
}

export interface PlacementWithItem {
    item: Item;
    placement: PlacementRecord;
}

export interface ShelfWithLayout {
    shelf: Shelf;
    rows: ShelfRow[];
    slots: ShelfSlot[];
    placements: PlacementWithItem[];
    unplaced: PlacementWithItem[];
}

export interface ShelfSummary {
    shelf: Shelf;
    itemCount: number;
    placedCount: number;
    slotCount: number;
}

export interface LayoutSlotInput {
    newSlot?: boolean;
    slotId?: string;
    rowIndex: number;
    colIndex: number;
    xStartNorm: number;
    xEndNorm: number;
    yStartNorm: number;
    yEndNorm: number;
}

export interface LayoutUpdateResponse {
    shelf: ShelfWithLayout;
    displaced: PlacementWithItem[] | null;
}

export type ScanStatus = 'created' | 'moved' | 'present';

export interface ScanAndAssignResult {
    item: Item;
    status: ScanStatus;
}
