import { Component, DestroyRef, computed, inject, signal, ViewChild } from '@angular/core';
import { NgClass } from '@angular/common';
import { ActivatedRoute, RouterModule } from '@angular/router';
import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize } from 'rxjs';
import { MatDialog } from '@angular/material/dialog';
import {
    ConfirmDeleteDialogComponent,
    ConfirmDeleteDialogData,
    ConfirmDeleteDialogResult,
} from '../../../components/confirm-delete-dialog/confirm-delete-dialog.component';

import {
    LayoutSlotInput,
    PlacementWithItem,
    ScanStatus,
    ShelfSlot,
    ShelfWithLayout,
} from '../../../models/shelf';
import { ShelfService } from '../../../services/shelf.service';
import {
    ShelfCanvasComponent,
    LayoutSlotData,
    SlotSelectionEvent,
    SlotPositionUpdate,
} from '../../../components/shelves/shelf-canvas/shelf-canvas.component';
import { SlotSidebarComponent } from '../../../components/shelves/slot-sidebar/slot-sidebar.component';
import {
    LayoutEditorComponent,
    LayoutRow,
} from '../../../components/shelves/layout-editor/layout-editor.component';
import { NotificationService } from '../../../services/notification.service';

/** A slot in the layout being edited. Its row and column are its position in the draft. */
type DraftSlot = Omit<LayoutSlotData, 'rowIndex' | 'colIndex'>;

@Component({
    selector: 'app-shelf-detail-page',
    standalone: true,
    imports: [
        NgClass,
        RouterModule,
        MatButtonModule,
        MatCardModule,
        MatIconModule,
        MatProgressBarModule,
        ShelfCanvasComponent,
        SlotSidebarComponent,
        LayoutEditorComponent,
    ],
    templateUrl: './shelf-detail-page.component.html',
    styleUrl: './shelf-detail-page.component.scss',
})
export class ShelfDetailPageComponent {
    private static readonly DEFAULT_SLOT_MARGIN = 0.02;
    private static readonly SCAN_DEBOUNCE_MS = 3000;

    private readonly route = inject(ActivatedRoute);
    private readonly dialog = inject(MatDialog);
    private confirmingLayout = false;
    private readonly shelfService = inject(ShelfService);
    private readonly notification = inject(NotificationService);
    private readonly destroyRef = inject(DestroyRef);

    private pendingSlotId: string | null = null;
    private lastScannedISBN: string | null = null;
    private lastScanTime: number = 0;
    private recentlyScannedItems = new Set<string>();

    @ViewChild(SlotSidebarComponent) sidebarComponent?: SlotSidebarComponent;

    readonly loading = signal(false);
    readonly savingLayout = signal(false);
    readonly shelf = signal<ShelfWithLayout | null>(null);
    readonly selectedSlot = signal<ShelfSlot | null>(null);
    readonly displaced = signal<PlacementWithItem[]>([]);
    readonly mode = signal<'view' | 'edit'>('view');
    readonly activeLayoutSelection = signal<SlotSelectionEvent | null>(null);

    readonly unplacedItems = computed(() => this.shelf()?.unplaced ?? []);
    readonly recentlyScannedIds = computed(() => this.recentlyScannedItems);

    /**
     * The layout being edited, as rows of slots. A row stays, empty, after its
     * last column is removed, so the editor keeps showing it.
     */
    readonly layoutDraft = signal<DraftSlot[][]>([]);
    readonly layoutSlots = computed<LayoutSlotData[]>(() =>
        this.layoutDraft().flatMap((row, rowIndex) =>
            row.map((slot, colIndex) => ({ ...slot, rowIndex, colIndex })),
        ),
    );
    readonly layoutRows = computed<LayoutRow[]>(() =>
        this.layoutDraft().map((row, rowIndex) => ({
            rowIndex,
            columns: row.map((_, colIndex) => ({ colIndex })),
        })),
    );

    constructor() {
        this.route.paramMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
            const id = params.get('id');
            if (!id) {
                this.shelf.set(null);
                return;
            }
            this.mode.set('view');
            this.displaced.set([]);
            this.selectedSlot.set(null);
            this.loadShelf(id);
        });

        this.route.queryParamMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
            this.pendingSlotId = params.get('slot');
            this.highlightSlotFromQuery();
        });
    }

    loadShelf(id: string): void {
        this.loading.set(true);
        this.shelfService
            .get(id)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (shelf) => {
                    this.shelf.set(shelf);
                    this.loading.set(false);
                    const highlighted = this.highlightSlotFromQuery();
                    if (!highlighted && !this.selectedSlot()) {
                        this.selectedSlot.set(shelf.slots[0] ?? null);
                    }
                    this.resetLayout();
                },
                error: () => {
                    this.loading.set(false);
                    this.notification.error('Could not load shelf.');
                },
            });
    }

    resetLayout(): void {
        const shelf = this.shelf();
        const slots = new Map(
            (shelf?.slots ?? []).map((slot) => [`${slot.rowIndex}-${slot.colIndex}`, slot]),
        );
        this.layoutDraft.set(
            (shelf?.rows ?? []).map((row) =>
                (row.columns ?? []).map((col) => {
                    const slot = slots.get(`${row.rowIndex}-${col.colIndex}`);
                    return {
                        slotId: slot?.id,
                        position: {
                            xStartNorm: col.xStartNorm,
                            xEndNorm: col.xEndNorm,
                            yStartNorm: slot?.yStartNorm ?? row.yStartNorm,
                            yEndNorm: slot?.yEndNorm ?? row.yEndNorm,
                        },
                    };
                }),
            ),
        );
    }

    private highlightSlotFromQuery(): boolean {
        const slotId = this.pendingSlotId;
        const shelf = this.shelf();
        if (!slotId || !shelf) {
            return false;
        }
        const slot = shelf.slots.find((s) => s.id === slotId);
        if (!slot) {
            return false;
        }
        this.selectedSlot.set(slot);
        this.pendingSlotId = null;
        return true;
    }

    // Mode management
    startEdit(): void {
        this.mode.set('edit');
        this.activeLayoutSelection.set(null);
        this.resetLayout();
    }

    cancelEdit(): void {
        this.mode.set('view');
        this.displaced.set([]);
        this.activeLayoutSelection.set(null);
        this.resetLayout();
    }

    // Layout editing
    addRow(): void {
        const margin = ShelfDetailPageComponent.DEFAULT_SLOT_MARGIN;
        this.editLayout((rows) => [
            ...rows,
            [
                {
                    position: {
                        xStartNorm: margin,
                        xEndNorm: 1 - margin,
                        yStartNorm: margin,
                        yEndNorm: 1 - margin,
                    },
                },
            ],
        ]);
    }

    addColumn(rowIndex: number): void {
        const margin = ShelfDetailPageComponent.DEFAULT_SLOT_MARGIN;
        this.editLayout((rows) =>
            rows.map((row, index) => {
                if (index !== rowIndex) {
                    return row;
                }
                const last = row.at(-1)?.position;
                return [
                    ...row,
                    {
                        position: {
                            xStartNorm: last?.xStartNorm ?? margin,
                            xEndNorm: last?.xEndNorm ?? 1 - margin,
                            yStartNorm: last?.yStartNorm ?? margin,
                            yEndNorm: last?.yEndNorm ?? 1 - margin,
                        },
                    },
                ];
            }),
        );
    }

    removeColumn(event: { rowIndex: number; colIndex: number }): void {
        this.editLayout((rows) =>
            rows.map((row, index) =>
                index === event.rowIndex ? row.filter((_, col) => col !== event.colIndex) : row,
            ),
        );
    }

    removeRow(rowIndex: number): void {
        this.editLayout((rows) => rows.filter((_, index) => index !== rowIndex));
    }

    private editLayout(edit: (rows: DraftSlot[][]) => DraftSlot[][]): void {
        this.layoutDraft.update(edit);
        this.ensureActiveLayoutSelection();
    }

    saveLayout(): void {
        const shelf = this.shelf();
        if (!shelf || this.savingLayout() || this.confirmingLayout) {
            return;
        }
        const slots: LayoutSlotInput[] = this.layoutSlots().map((slot) => ({
            newSlot: slot.slotId ? undefined : true,
            slotId: slot.slotId,
            rowIndex: slot.rowIndex,
            colIndex: slot.colIndex,
            xStartNorm: slot.position.xStartNorm,
            xEndNorm: slot.position.xEndNorm,
            yStartNorm: slot.position.yStartNorm,
            yEndNorm: slot.position.yEndNorm,
        }));

        const retained = new Set(slots.map((slot) => slot.slotId));
        const affected = new Set(
            shelf.placements
                .filter(
                    ({ placement }) =>
                        placement.shelfSlotId && !retained.has(placement.shelfSlotId),
                )
                .map(({ item }) => item.id),
        );
        if (affected.size > 0) {
            this.confirmingLayout = true;
            this.dialog
                .open<
                    ConfirmDeleteDialogComponent,
                    ConfirmDeleteDialogData,
                    ConfirmDeleteDialogResult
                >(ConfirmDeleteDialogComponent, {
                    width: '400px',
                    data: {
                        title: 'Remove books from shelf?',
                        message: `This will remove ${affected.size} ${affected.size === 1 ? 'book' : 'books'} from this shelf. They will remain in your library.`,
                        itemCount: affected.size,
                        confirmLabel: 'Save layout',
                    },
                })
                .afterClosed()
                .pipe(takeUntilDestroyed(this.destroyRef))
                .subscribe((result) => {
                    this.confirmingLayout = false;
                    if (result === 'confirm' && this.shelf()?.shelf.id === shelf.shelf.id) {
                        this.persistLayout(shelf.shelf.id, slots);
                    }
                });
            return;
        }
        this.persistLayout(shelf.shelf.id, slots);
    }

    private persistLayout(shelfID: string, slots: LayoutSlotInput[]): void {
        this.savingLayout.set(true);
        this.shelfService
            .updateLayout(shelfID, slots)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (response) => {
                    this.shelf.set(response.shelf);
                    const selectedId = this.selectedSlot()?.id;
                    this.selectedSlot.set(
                        response.shelf.slots.find((slot) => slot.id === selectedId) ??
                            response.shelf.slots[0] ??
                            null,
                    );
                    this.displaced.set(response.displaced ?? []);
                    this.savingLayout.set(false);
                    this.mode.set('view');
                    this.resetLayout();
                    this.notification.success('Layout updated');
                },
                error: (err) => {
                    this.savingLayout.set(false);
                    const message = err?.error?.error ?? 'Could not save layout';
                    this.notification.error(message);
                },
            });
    }

    // Canvas event handlers
    onSlotSelected(slot: ShelfSlot): void {
        this.selectedSlot.set(slot);
    }

    onLayoutSlotSelected(event: SlotSelectionEvent): void {
        this.activeLayoutSelection.set(event);
    }

    onSlotPositionChanged(update: SlotPositionUpdate): void {
        this.layoutDraft.update((rows) =>
            rows.map((row, rowIndex) =>
                rowIndex !== update.rowIndex
                    ? row
                    : row.map((slot, colIndex) =>
                          colIndex === update.colIndex
                              ? { ...slot, position: { ...update.position } }
                              : slot,
                      ),
            ),
        );
    }

    // Sidebar event handlers
    assignedItems(slotId: string): PlacementWithItem[] {
        return (this.shelf()?.placements ?? [])
            .filter((p) => p.placement.shelfSlotId === slotId)
            .sort((a, b) =>
                a.item.title.localeCompare(b.item.title, undefined, { sensitivity: 'base' }),
            );
    }

    onItemRemoved(itemId: string): void {
        const slot = this.selectedSlot();
        const shelf = this.shelf();
        if (!slot || !shelf) {
            return;
        }
        this.shelfService
            .removeItem(shelf.shelf.id, slot.id, itemId)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (updated) => this.shelf.set(updated),
                error: () => this.notification.error('Unable to remove item'),
            });
    }

    onItemSelected(itemId: string): void {
        const slot = this.selectedSlot();
        const shelf = this.shelf();
        if (!slot || !shelf) {
            return;
        }
        this.shelfService
            .assignItem(shelf.shelf.id, slot.id, itemId)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (updated) => this.shelf.set(updated),
                error: () => this.notification.error('Unable to assign item'),
            });
    }

    onBarcodeScanned(isbn: string): void {
        const now = Date.now();

        if (
            isbn === this.lastScannedISBN &&
            now - this.lastScanTime < ShelfDetailPageComponent.SCAN_DEBOUNCE_MS
        ) {
            this.notification.info('Item already scanned. Wait a moment before scanning again.', {
                duration: 2000,
            });
            this.sidebarComponent?.reportScanComplete();
            return;
        }

        this.lastScannedISBN = isbn;
        this.lastScanTime = now;

        const shelf = this.shelf();
        const slot = this.selectedSlot();

        if (!shelf || !slot) {
            this.notification.warn('No slot selected', { duration: 3000 });
            this.sidebarComponent?.reportScanComplete();
            return;
        }

        this.shelfService
            .scanAndAssign(shelf.shelf.id, slot.id, isbn)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.sidebarComponent?.reportScanComplete()),
            )
            .subscribe({
                next: (result) => {
                    this.handleScanSuccess(result.item.id, result.item.title, result.status);
                    this.loadShelf(shelf.shelf.id);
                },
                error: (err) => {
                    console.error('Scan failed', err);
                    const message = err.error?.error || 'Could not scan item. Please try again.';
                    this.notification.error(message);
                },
            });
    }

    private handleScanSuccess(itemId: string, title: string, status: ScanStatus): void {
        this.recentlyScannedItems.add(itemId);

        setTimeout(() => {
            this.recentlyScannedItems.delete(itemId);
        }, 5000);

        const slot = this.selectedSlot();
        if (!slot) {
            return;
        }

        if (status === 'created') {
            this.notification.success(`Added: ${title}`);
        } else if (status === 'moved') {
            this.notification.success(
                `Moved: ${title} → Slot ${slot.rowIndex + 1}·${slot.colIndex + 1}`,
            );
        } else if (status === 'present') {
            this.notification.info(`Already here: ${title}`);
        }
    }

    onUnplacedItemAssigned(itemId: string): void {
        this.onItemSelected(itemId);
    }

    private ensureActiveLayoutSelection(): void {
        const selection = this.activeLayoutSelection();
        if (!selection) {
            return;
        }
        const row = this.layoutDraft()[selection.rowIndex];
        if (!row || selection.colIndex >= row.length) {
            this.activeLayoutSelection.set(null);
        }
    }
}
