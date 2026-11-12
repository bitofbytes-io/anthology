import { Component, DestroyRef, inject, signal, ChangeDetectionStrategy } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { ActivatedRoute, Router, RouterModule } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';

import { Item, ItemForm } from '../../models';
import { ItemService } from '../../services/item.service';
import { ItemFormComponent } from '../../components/item-form/item-form.component';
import { NotificationService } from '../../services/notification.service';

@Component({
    selector: 'app-edit-item-page',
    standalone: true,
    imports: [
        ItemFormComponent,
        MatButtonModule,
        MatCardModule,
        MatIconModule,
        MatProgressSpinnerModule,
        RouterModule,
    ],
    templateUrl: './edit-item-page.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './edit-item-page.component.scss',
})
export class EditItemPageComponent {
    private readonly route = inject(ActivatedRoute);
    private readonly router = inject(Router);
    private readonly itemService = inject(ItemService);
    private readonly notification = inject(NotificationService);
    private readonly destroyRef = inject(DestroyRef);

    readonly loading = signal(true);
    readonly busy = signal(false);
    readonly resyncing = signal(false);
    readonly item = signal<Item | null>(null);
    readonly loadError = signal<string | null>(null);
    readonly itemId = signal<string | null>(null);

    constructor() {
        const id = this.route.snapshot.paramMap.get('id');
        if (!id) {
            this.handleMissingItem();
            return;
        }

        this.itemId.set(id);
        this.fetchItem(id);
    }

    handleSave(formValue: ItemForm): void {
        const current = this.item();
        if (!current || this.busy()) {
            return;
        }

        this.busy.set(true);
        this.itemService
            .update(current.id, formValue)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (item) => {
                    this.busy.set(false);
                    this.notification.success(`Updated "${item.title}"`);
                    this.navigateBack();
                },
                error: () => {
                    this.busy.set(false);
                    this.notification.error(
                        'We could not save the item. Double-check required fields.',
                    );
                },
            });
    }

    handleDelete(): void {
        const current = this.item();
        if (!current || this.busy()) {
            return;
        }

        this.busy.set(true);
        this.itemService
            .delete(current.id)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: () => {
                    this.busy.set(false);
                    this.notification.success('Item deleted.');
                    this.navigateBack();
                },
                error: () => {
                    this.busy.set(false);
                    this.notification.error('Unable to delete this entry right now.');
                },
            });
    }

    handleCancel(): void {
        if (!this.busy() && !this.resyncing()) {
            this.navigateBack();
        }
    }

    handleResync(): void {
        const current = this.item();
        if (!current || this.busy() || this.resyncing()) {
            return;
        }

        this.resyncing.set(true);
        this.itemService
            .resync(current.id)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (updatedItem) => {
                    this.resyncing.set(false);
                    this.item.set(updatedItem);
                    this.notification.success('Metadata refreshed from Google Books.');
                },
                error: () => {
                    this.resyncing.set(false);
                    this.notification.error(
                        'Unable to refresh metadata. The book may not be found in Google Books.',
                    );
                },
            });
    }

    retry(): void {
        const id = this.itemId();
        if (!id) {
            return;
        }

        this.fetchItem(id);
    }

    navigateBack(): void {
        this.router.navigate(['/']);
    }

    private fetchItem(id: string): void {
        this.loading.set(true);
        this.loadError.set(null);

        this.itemService
            .get(id)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (item) => {
                    this.loading.set(false);
                    this.item.set(item);
                },
                error: () => {
                    this.loading.set(false);
                    this.loadError.set('Unable to load this item right now.');
                },
            });
    }

    private handleMissingItem(): void {
        this.loading.set(false);
        this.loadError.set('Item not found.');
    }
}
