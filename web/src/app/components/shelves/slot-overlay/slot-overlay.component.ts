import { Component, EventEmitter, Input, Output, ChangeDetectionStrategy } from '@angular/core';
import { NgClass, NgStyle } from '@angular/common';

export type CornerPosition = 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right';

export interface SlotPosition {
    xStartNorm: number;
    xEndNorm: number;
    yStartNorm: number;
    yEndNorm: number;
}

export interface CornerDragEvent {
    corner: CornerPosition;
    event: PointerEvent;
}

@Component({
    selector: 'app-slot-overlay',
    standalone: true,
    imports: [NgClass, NgStyle],
    templateUrl: './slot-overlay.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './slot-overlay.component.scss',
})
export class SlotOverlayComponent {
    @Input() rowIndex = 0;
    @Input() colIndex = 0;
    @Input() position: SlotPosition = { xStartNorm: 0, xEndNorm: 1, yStartNorm: 0, yEndNorm: 1 };
    @Input() mode: 'view' | 'edit' = 'view';
    @Input() selected = false;
    @Input() active = false;
    @Input() slotId?: string;

    @Output() slotClick = new EventEmitter<void>();
    @Output() cornerDragStart = new EventEmitter<CornerDragEvent>();

    get slotStyle(): Record<string, string> {
        return {
            left: `${this.position.xStartNorm * 100}%`,
            top: `${this.position.yStartNorm * 100}%`,
            width: `${(this.position.xEndNorm - this.position.xStartNorm) * 100}%`,
            height: `${(this.position.yEndNorm - this.position.yStartNorm) * 100}%`,
        };
    }

    get label(): string {
        return `${this.rowIndex + 1} / ${this.colIndex + 1}`;
    }

    get ariaLabelPrefix(): string {
        return `slot ${this.rowIndex + 1}/${this.colIndex + 1}`;
    }

    onClick(): void {
        this.slotClick.emit();
    }

    onCornerPointerDown(corner: CornerPosition, event: PointerEvent): void {
        this.cornerDragStart.emit({ corner, event });
    }
}
