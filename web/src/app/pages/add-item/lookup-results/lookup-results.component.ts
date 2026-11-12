import { Component, EventEmitter, Input, Output, ChangeDetectionStrategy } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';

import { ItemForm } from '../../../models';

@Component({
    selector: 'app-lookup-results',
    standalone: true,
    imports: [MatButtonModule],
    templateUrl: './lookup-results.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './lookup-results.component.scss',
})
export class LookupResultsComponent {
    @Input({ required: true }) results: ItemForm[] = [];
    @Input() busy = false;

    @Output() quickAdd = new EventEmitter<ItemForm>();
    @Output() useForManual = new EventEmitter<ItemForm>();

    handleQuickAdd(result: ItemForm): void {
        if (!this.busy) {
            this.quickAdd.emit(result);
        }
    }

    handleUseForManual(result: ItemForm): void {
        this.useForManual.emit(result);
    }
}
