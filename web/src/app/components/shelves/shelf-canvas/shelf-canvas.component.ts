import {
    Component,
    ElementRef,
    EventEmitter,
    Input,
    Output,
    ViewChild,
    NgZone,
    ChangeDetectorRef,
    DestroyRef,
    inject,
    ChangeDetectionStrategy,
} from '@angular/core';

import {
    SlotOverlayComponent,
    SlotPosition,
    CornerPosition,
    CornerDragEvent,
} from '../slot-overlay/slot-overlay.component';
import { ShelfSlot } from '../../../models/shelf';

export interface LayoutSlotData {
    rowIndex: number;
    colIndex: number;
    position: SlotPosition;
    slotId?: string;
}

export interface SlotSelectionEvent {
    rowIndex: number;
    colIndex: number;
}

export interface CornerDragStartEvent {
    rowIndex: number;
    colIndex: number;
    corner: CornerPosition;
    event: PointerEvent;
}

export interface SlotPositionUpdate {
    rowIndex: number;
    colIndex: number;
    position: SlotPosition;
}

@Component({
    selector: 'app-shelf-canvas',
    standalone: true,
    imports: [SlotOverlayComponent],
    templateUrl: './shelf-canvas.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './shelf-canvas.component.scss',
})
export class ShelfCanvasComponent {
    private static readonly MIN_SEGMENT = 0.02;
    private static readonly AXIS_LOCK_THRESHOLD_PX = 16;

    private readonly ngZone = inject(NgZone);
    private readonly cdr = inject(ChangeDetectorRef);
    private readonly destroyRef = inject(DestroyRef);

    @ViewChild('canvasOverlay') canvasOverlay?: ElementRef<HTMLDivElement>;

    @Input() photoUrl = '';
    @Input() photoAlt = '';
    @Input() mode: 'view' | 'edit' = 'view';

    // View mode inputs
    @Input() slots: ShelfSlot[] = [];
    @Input() selectedSlotId: string | null = null;

    // Edit mode inputs
    @Input() layoutSlots: LayoutSlotData[] = [];
    @Input() selectedLayoutSlot: SlotSelectionEvent | null = null;

    @Output() slotSelected = new EventEmitter<ShelfSlot>();
    @Output() layoutSlotSelected = new EventEmitter<SlotSelectionEvent>();
    @Output() slotPositionChanged = new EventEmitter<SlotPositionUpdate>();

    private activeDrag: {
        rowIndex: number;
        colIndex: number;
        corner: CornerPosition;
    } | null = null;
    private overlayRect: DOMRect | null = null;
    private dragStartPoint: { x: number; y: number } | null = null;
    private dragLockedAxis: 'x' | 'y' | null = null;

    private readonly pointerMoveListener = (event: PointerEvent) => this.handlePointerMove(event);
    private readonly pointerUpListener = () => this.endDrag();

    constructor() {
        this.destroyRef.onDestroy(() => {
            this.endDrag();
        });
    }

    isLayoutSlotSelected(rowIndex: number, colIndex: number): boolean {
        const selected = this.selectedLayoutSlot;
        return !!selected && selected.rowIndex === rowIndex && selected.colIndex === colIndex;
    }

    isSlotActive(rowIndex: number, colIndex: number): boolean {
        const active = this.activeDrag;
        if (!!active && active.rowIndex === rowIndex && active.colIndex === colIndex) {
            return true;
        }
        return this.isLayoutSlotSelected(rowIndex, colIndex);
    }

    onViewSlotClick(slot: ShelfSlot): void {
        this.slotSelected.emit(slot);
    }

    onLayoutSlotClick(rowIndex: number, colIndex: number): void {
        this.layoutSlotSelected.emit({ rowIndex, colIndex });
    }

    onCornerDragStart(rowIndex: number, colIndex: number, event: CornerDragEvent): void {
        if (this.mode !== 'edit' || !this.isLayoutSlotSelected(rowIndex, colIndex)) {
            return;
        }
        this.startDrag(rowIndex, colIndex, event.corner, event.event);
    }

    getSlotPosition(slot: ShelfSlot): SlotPosition {
        return {
            xStartNorm: slot.xStartNorm,
            xEndNorm: slot.xEndNorm,
            yStartNorm: slot.yStartNorm,
            yEndNorm: slot.yEndNorm,
        };
    }

    private startDrag(
        rowIndex: number,
        colIndex: number,
        corner: CornerPosition,
        event: PointerEvent,
    ): void {
        const win = this.windowRef;
        if (!win) {
            return;
        }
        event.preventDefault();
        event.stopPropagation();
        this.endDrag();

        this.layoutSlotSelected.emit({ rowIndex, colIndex });
        this.activeDrag = { rowIndex, colIndex, corner };
        this.overlayRect = this.canvasOverlay?.nativeElement.getBoundingClientRect() ?? null;
        this.dragLockedAxis = null;
        this.dragStartPoint = { x: event.clientX, y: event.clientY };

        win.addEventListener('pointermove', this.pointerMoveListener);
        win.addEventListener('pointerup', this.pointerUpListener);
        win.addEventListener('pointercancel', this.pointerUpListener);
    }

    private endDrag(): void {
        const win = this.windowRef;
        if (!this.activeDrag) {
            return;
        }
        this.activeDrag = null;
        if (win) {
            win.removeEventListener('pointermove', this.pointerMoveListener);
            win.removeEventListener('pointerup', this.pointerUpListener);
            win.removeEventListener('pointercancel', this.pointerUpListener);
        }
        this.overlayRect = null;
        this.dragStartPoint = null;
        this.dragLockedAxis = null;
    }

    private handlePointerMove(event: PointerEvent): void {
        if (!this.activeDrag) {
            return;
        }
        if (!this.overlayRect) {
            this.overlayRect = this.canvasOverlay?.nativeElement.getBoundingClientRect() ?? null;
        }
        const rect = this.overlayRect;
        if (!rect) {
            return;
        }
        this.adjustCorner(event.clientX, event.clientY, rect);
    }

    private adjustCorner(clientX: number, clientY: number, rect: DOMRect): void {
        if (!this.activeDrag || !this.dragStartPoint) {
            return;
        }

        const slot = this.layoutSlots.find(
            (s) =>
                s.rowIndex === this.activeDrag!.rowIndex &&
                s.colIndex === this.activeDrag!.colIndex,
        );
        if (!slot) {
            return;
        }

        const dx = Math.abs(clientX - this.dragStartPoint.x);
        const dy = Math.abs(clientY - this.dragStartPoint.y);

        if (
            !this.dragLockedAxis &&
            Math.max(dx, dy) >= ShelfCanvasComponent.AXIS_LOCK_THRESHOLD_PX
        ) {
            const AXIS_LOCK_DOMINANCE_RATIO = 2;
            if (dy > dx * AXIS_LOCK_DOMINANCE_RATIO) {
                this.dragLockedAxis = 'y';
            } else if (dx > dy * AXIS_LOCK_DOMINANCE_RATIO) {
                this.dragLockedAxis = 'x';
            }
        }

        const allowX = this.dragLockedAxis === null || this.dragLockedAxis === 'x';
        const allowY = this.dragLockedAxis === null || this.dragLockedAxis === 'y';

        const normalizedX = this.clamp((clientX - rect.left) / rect.width, 0, 1);
        const normalizedY = this.clamp((clientY - rect.top) / rect.height, 0, 1);

        const corner = this.activeDrag.corner;
        const isTop = corner === 'top-left' || corner === 'top-right';
        const isLeft = corner === 'top-left' || corner === 'bottom-left';

        const newPosition = { ...slot.position };

        this.ngZone.run(() => {
            if (allowY && isTop) {
                const max = newPosition.yEndNorm - ShelfCanvasComponent.MIN_SEGMENT;
                newPosition.yStartNorm = this.clamp(normalizedY, 0, max);
            } else if (allowY && !isTop) {
                const min = newPosition.yStartNorm + ShelfCanvasComponent.MIN_SEGMENT;
                newPosition.yEndNorm = this.clamp(normalizedY, min, 1);
            }

            if (allowX && isLeft) {
                const max = newPosition.xEndNorm - ShelfCanvasComponent.MIN_SEGMENT;
                newPosition.xStartNorm = this.clamp(normalizedX, 0, max);
            } else if (allowX && !isLeft) {
                const min = newPosition.xStartNorm + ShelfCanvasComponent.MIN_SEGMENT;
                newPosition.xEndNorm = this.clamp(normalizedX, min, 1);
            }

            this.slotPositionChanged.emit({
                rowIndex: slot.rowIndex,
                colIndex: slot.colIndex,
                position: newPosition,
            });
            this.cdr.detectChanges();
        });

        if (
            clientX < rect.left ||
            clientX > rect.right ||
            clientY < rect.top ||
            clientY > rect.bottom
        ) {
            this.overlayRect = this.canvasOverlay?.nativeElement.getBoundingClientRect() ?? null;
        }
    }

    private clamp(value: number, min: number, max: number): number {
        if (Number.isNaN(value)) {
            return min;
        }
        if (max < min) {
            return min;
        }
        return Math.min(Math.max(value, min), max);
    }

    private get windowRef(): Window | null {
        return typeof window === 'undefined' ? null : window;
    }
}
