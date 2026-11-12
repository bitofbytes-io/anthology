import {
    Component,
    DestroyRef,
    computed,
    inject,
    signal,
    ChangeDetectionStrategy,
} from '@angular/core';

import { MatCardModule } from '@angular/material/card';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { RouterModule } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatButtonModule } from '@angular/material/button';

import { ShelfService } from '../../../services/shelf.service';
import { ShelfSummary } from '../../../models/shelf';
import { NotificationService } from '../../../services/notification.service';

@Component({
    selector: 'app-shelves-page',
    standalone: true,
    imports: [MatCardModule, MatIconModule, MatProgressBarModule, RouterModule, MatButtonModule],
    templateUrl: './shelves-page.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './shelves-page.component.scss',
})
export class ShelvesPageComponent {
    private readonly shelfService = inject(ShelfService);
    private readonly notification = inject(NotificationService);
    private readonly destroyRef = inject(DestroyRef);

    readonly loading = signal(false);
    readonly shelves = signal<ShelfSummary[]>([]);
    readonly hasShelves = computed(() => this.shelves().length > 0);

    constructor() {
        this.loadShelves();
    }

    loadShelves(): void {
        this.loading.set(true);
        this.shelfService
            .list()
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (shelves) => {
                    this.shelves.set(shelves);
                    this.loading.set(false);
                },
                error: () => {
                    this.loading.set(false);
                    this.notification.error('Unable to load shelves right now.');
                },
            });
    }

    trackByShelf(_: number, shelf: ShelfSummary): string {
        return shelf.shelf.id;
    }
}
