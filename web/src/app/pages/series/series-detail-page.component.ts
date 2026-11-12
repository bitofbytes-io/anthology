import { NgClass } from '@angular/common';
import {
    Component,
    DestroyRef,
    inject,
    OnInit,
    signal,
    ChangeDetectionStrategy,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatChipsModule } from '@angular/material/chips';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { catchError, EMPTY, filter, finalize, switchMap, tap } from 'rxjs';

import { Item, SeriesStatus, SeriesSummary, SERIES_STATUS_LABELS } from '../../models';
import { SeriesService } from '../../services/series.service';
import { NotificationService } from '../../services/notification.service';
import { ItemCardComponent } from '../items/item-card/item-card.component';
import {
    EditSeriesDialogComponent,
    EditSeriesDialogData,
    EditSeriesDialogResult,
} from '../../components/edit-series-dialog/edit-series-dialog.component';
import {
    ConfirmDeleteDialogComponent,
    ConfirmDeleteDialogData,
    ConfirmDeleteDialogResult,
} from '../../components/confirm-delete-dialog/confirm-delete-dialog.component';

const STATUS_CLASS_MAP: Record<SeriesStatus, string> = {
    complete: 'status-complete',
    incomplete: 'status-incomplete',
    unknown: 'status-unknown',
};

@Component({
    selector: 'app-series-detail-page',
    standalone: true,
    imports: [
        NgClass,
        MatButtonModule,
        MatCardModule,
        MatChipsModule,
        MatIconModule,
        MatProgressBarModule,
        ItemCardComponent,
    ],
    templateUrl: './series-detail-page.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './series-detail-page.component.scss',
})
export class SeriesDetailPageComponent implements OnInit {
    private readonly route = inject(ActivatedRoute);
    private readonly router = inject(Router);
    private readonly seriesService = inject(SeriesService);
    private readonly notification = inject(NotificationService);
    private readonly dialog = inject(MatDialog);
    private readonly destroyRef = inject(DestroyRef);

    readonly series = signal<SeriesSummary | null>(null);
    readonly loading = signal(true);
    readonly busy = signal(false);

    ngOnInit(): void {
        this.route.queryParams
            .pipe(
                switchMap((params) => {
                    const name = params['name'];
                    if (!name) {
                        this.router.navigate(['/items']);
                        return EMPTY;
                    }
                    this.loading.set(true);
                    return this.seriesService.get(name).pipe(
                        tap((series) => {
                            this.series.set(series);
                            this.loading.set(false);
                        }),
                        catchError(() => {
                            this.notification.error('Unable to load series details.');
                            this.loading.set(false);
                            return EMPTY;
                        }),
                    );
                }),
                takeUntilDestroyed(this.destroyRef),
            )
            .subscribe();
    }

    goBack(): void {
        this.router.navigate(['/items']);
    }

    editItem(item: Item): void {
        this.router.navigate(['/items', item.id, 'edit']);
    }

    addMissingVolume(volumeNumber: number): void {
        const seriesName = this.series()?.seriesName;
        if (!seriesName) return;

        this.router.navigate(['/items/add'], {
            queryParams: {
                prefill: 'series',
                seriesName,
                volumeNumber,
            },
        });
    }

    getStatusClass(status: SeriesStatus): string {
        return STATUS_CLASS_MAP[status];
    }

    getStatusLabel(status: SeriesStatus): string {
        return SERIES_STATUS_LABELS[status];
    }

    trackById(_: number, item: Item): string {
        return item.id;
    }

    trackByVolume(_: number, volume: number): number {
        return volume;
    }

    openEditDialog(): void {
        const currentSeries = this.series();
        if (!currentSeries) return;

        const dialogRef = this.dialog.open<
            EditSeriesDialogComponent,
            EditSeriesDialogData,
            EditSeriesDialogResult
        >(EditSeriesDialogComponent, {
            data: { seriesName: currentSeries.seriesName },
            width: '400px',
        });

        dialogRef
            .afterClosed()
            .pipe(
                filter(
                    (result): result is { action: 'save'; newName: string } =>
                        result?.action === 'save',
                ),
                tap(() => this.busy.set(true)),
                switchMap((result) =>
                    this.seriesService.update(currentSeries.seriesName, result.newName).pipe(
                        tap((updated) => {
                            this.series.set(updated);
                            this.notification.success('Series renamed successfully.');
                            // Update URL without reloading
                            this.router.navigate([], {
                                relativeTo: this.route,
                                queryParams: { name: updated.seriesName },
                                queryParamsHandling: 'merge',
                            });
                        }),
                        catchError(() => {
                            this.notification.error('Unable to rename series.');
                            return EMPTY;
                        }),
                    ),
                ),
                finalize(() => this.busy.set(false)),
                takeUntilDestroyed(this.destroyRef),
            )
            .subscribe();
    }

    openDeleteDialog(): void {
        const currentSeries = this.series();
        if (!currentSeries) return;

        const dialogRef = this.dialog.open<
            ConfirmDeleteDialogComponent,
            ConfirmDeleteDialogData,
            ConfirmDeleteDialogResult
        >(ConfirmDeleteDialogComponent, {
            data: {
                title: 'Delete Series',
                message: `This will remove the series association from all items in "${currentSeries.seriesName}". The items themselves will not be deleted.`,
                itemCount: currentSeries.ownedCount,
                confirmLabel: 'Delete Series',
            },
            width: '400px',
        });

        dialogRef
            .afterClosed()
            .pipe(
                filter((result): result is ConfirmDeleteDialogResult => result === 'confirm'),
                tap(() => this.busy.set(true)),
                switchMap(() =>
                    this.seriesService.delete(currentSeries.seriesName).pipe(
                        tap((response) => {
                            this.notification.success(
                                `Series deleted. ${response.itemsUpdated} item${response.itemsUpdated === 1 ? '' : 's'} updated.`,
                            );
                            this.router.navigate(['/items']);
                        }),
                        catchError(() => {
                            this.notification.error('Unable to delete series.');
                            return EMPTY;
                        }),
                        finalize(() => this.busy.set(false)),
                    ),
                ),
                takeUntilDestroyed(this.destroyRef),
            )
            .subscribe();
    }
}
