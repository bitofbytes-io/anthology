import { Component, EventEmitter, Input, Output, ChangeDetectionStrategy } from '@angular/core';
import { NgClass } from '@angular/common';
import { MatIconModule } from '@angular/material/icon';
import { MatTooltipModule } from '@angular/material/tooltip';

import { PlacementWithItem } from '../../../models/shelf';

@Component({
    selector: 'app-assigned-items-list',
    standalone: true,
    imports: [NgClass, MatIconModule, MatTooltipModule],
    templateUrl: './assigned-items-list.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './assigned-items-list.component.scss',
})
export class AssignedItemsListComponent {
    @Input() items: PlacementWithItem[] = [];
    @Input() recentlyScannedIds: Set<string> = new Set();

    @Output() removeItem = new EventEmitter<string>();

    isRecentlyScanned(itemId: string): boolean {
        return this.recentlyScannedIds.has(itemId);
    }

    onRemove(itemId: string): void {
        this.removeItem.emit(itemId);
    }
}
