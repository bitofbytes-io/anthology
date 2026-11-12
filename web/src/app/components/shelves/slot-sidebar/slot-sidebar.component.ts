import {
    Component,
    EventEmitter,
    Input,
    Output,
    signal,
    ViewChild,
    ChangeDetectionStrategy,
} from '@angular/core';

import { MatCardModule } from '@angular/material/card';
import { MatChipsModule } from '@angular/material/chips';
import { MatIconModule } from '@angular/material/icon';
import { MatTabsModule } from '@angular/material/tabs';

import { ShelfSlot, PlacementWithItem } from '../../../models/shelf';
import { AssignedItemsListComponent } from '../assigned-items-list/assigned-items-list.component';
import { ItemSearchComponent } from '../item-search/item-search.component';
import { BarcodeScannerPanelComponent } from '../barcode-scanner-panel/barcode-scanner-panel.component';

@Component({
    selector: 'app-slot-sidebar',
    standalone: true,
    imports: [
        MatCardModule,
        MatChipsModule,
        MatIconModule,
        MatTabsModule,
        AssignedItemsListComponent,
        ItemSearchComponent,
        BarcodeScannerPanelComponent,
    ],
    templateUrl: './slot-sidebar.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './slot-sidebar.component.scss',
})
export class SlotSidebarComponent {
    @ViewChild(BarcodeScannerPanelComponent) scannerPanel?: BarcodeScannerPanelComponent;

    @Input() slot: ShelfSlot | null = null;
    @Input() assignedItems: PlacementWithItem[] = [];
    @Input() unplacedItems: PlacementWithItem[] = [];
    @Input() displacedItems: PlacementWithItem[] = [];
    @Input() recentlyScannedIds: Set<string> = new Set();

    @Output() itemRemoved = new EventEmitter<string>();
    @Output() itemSelected = new EventEmitter<string>();
    @Output() barcodeScanned = new EventEmitter<string>();
    @Output() unplacedItemAssigned = new EventEmitter<string>();

    readonly selectedTab = signal(0);

    get slotLabel(): string {
        if (!this.slot) return '';
        return `Slot ${this.slot.rowIndex + 1} · ${this.slot.colIndex + 1}`;
    }

    get placedCount(): number {
        return this.assignedItems.length;
    }

    handleTabChange(index: number): void {
        this.selectedTab.set(index);
    }

    get isScannerTabActive(): boolean {
        return this.selectedTab() === 1;
    }

    onItemRemove(itemId: string): void {
        this.itemRemoved.emit(itemId);
    }

    onItemSelected(itemId: string): void {
        this.itemSelected.emit(itemId);
    }

    onBarcodeScanned(isbn: string): void {
        this.barcodeScanned.emit(isbn);
    }

    onUnplacedItemAssign(itemId: string): void {
        this.unplacedItemAssigned.emit(itemId);
    }

    reportScanComplete(): void {
        this.scannerPanel?.reportScanComplete();
    }
}
