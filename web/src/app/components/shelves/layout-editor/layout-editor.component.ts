import { Component, EventEmitter, Input, Output, ChangeDetectionStrategy } from '@angular/core';

import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatIconModule } from '@angular/material/icon';

export interface LayoutRow {
    rowIndex: number;
    columns: LayoutColumn[];
}

export interface LayoutColumn {
    colIndex: number;
}

export interface SlotSelection {
    rowIndex: number;
    colIndex: number;
}

@Component({
    selector: 'app-layout-editor',
    standalone: true,
    imports: [MatButtonModule, MatCardModule, MatIconModule],
    templateUrl: './layout-editor.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './layout-editor.component.scss',
})
export class LayoutEditorComponent {
    @Input() rows: LayoutRow[] = [];
    @Input() selectedSlot: SlotSelection | null = null;

    @Output() addRow = new EventEmitter<void>();
    @Output() removeRow = new EventEmitter<number>();
    @Output() addColumn = new EventEmitter<number>();
    @Output() removeColumn = new EventEmitter<{ rowIndex: number; colIndex: number }>();
    @Output() selectSlot = new EventEmitter<SlotSelection>();

    isSelected(rowIndex: number, colIndex: number): boolean {
        return (
            !!this.selectedSlot &&
            this.selectedSlot.rowIndex === rowIndex &&
            this.selectedSlot.colIndex === colIndex
        );
    }

    onAddRow(): void {
        this.addRow.emit();
    }

    onRemoveRow(rowIndex: number): void {
        this.removeRow.emit(rowIndex);
    }

    onAddColumn(rowIndex: number): void {
        this.addColumn.emit(rowIndex);
    }

    onRemoveColumn(rowIndex: number, colIndex: number): void {
        this.removeColumn.emit({ rowIndex, colIndex });
    }

    onSelectSlot(rowIndex: number, colIndex: number): void {
        this.selectSlot.emit({ rowIndex, colIndex });
    }
}
