import {
    Component,
    DestroyRef,
    OnInit,
    computed,
    inject,
    signal,
    ChangeDetectionStrategy,
} from '@angular/core';
import { MatCardModule } from '@angular/material/card';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog, MatDialogModule } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { ActivatedRoute, ParamMap, Router, RouterModule } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatTabsModule } from '@angular/material/tabs';
import { catchError, of, firstValueFrom } from 'rxjs';

import { ItemFormComponent } from '../../components/item-form/item-form.component';
import {
    DuplicateDialogComponent,
    DuplicateDialogData,
    DuplicateDialogResult,
} from '../../components/duplicate-dialog/duplicate-dialog.component';
import { DuplicateMatch, ItemForm } from '../../models';
import { ItemService } from '../../services/item.service';
import { CsvImportComponent } from './csv-import/csv-import.component';
import { LookupResultsComponent } from './lookup-results/lookup-results.component';
import { LookupCompletedEvent, SearchPanelComponent } from './search-panel/search-panel.component';
import { NotificationService } from '../../services/notification.service';

@Component({
    selector: 'app-add-item-page',
    standalone: true,
    imports: [
        MatButtonModule,
        MatCardModule,
        MatDialogModule,
        MatIconModule,
        MatTabsModule,
        RouterModule,
        ItemFormComponent,
        CsvImportComponent,
        LookupResultsComponent,
        SearchPanelComponent,
    ],
    templateUrl: './add-item-page.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './add-item-page.component.scss',
})
export class AddItemPageComponent implements OnInit {
    private static readonly MANUAL_ENTRY_TAB_INDEX = 1;

    private readonly itemService = inject(ItemService);
    private readonly router = inject(Router);
    private readonly route = inject(ActivatedRoute);
    private readonly notification = inject(NotificationService);
    private readonly dialog = inject(MatDialog);
    private readonly destroyRef = inject(DestroyRef);

    readonly busy = signal(false);
    readonly lookupResults = signal<ItemForm[]>([]);
    readonly manualDraft = signal<ItemForm | null>(null);
    readonly manualDraftSource = signal<{ query: string; label: string } | null>(null);
    readonly lastLookupSummary = signal<string | null>(null);
    readonly selectedTab = signal(0);
    /** True while the CSV tab is saving rows; the route and other tabs are blocked meanwhile. */
    readonly csvImportBusy = signal(false);
    /** True while a CSV review is open, so switching tabs cannot discard the review. */
    readonly csvReviewActive = signal(false);
    readonly otherTabsDisabled = computed(() => this.csvImportBusy() || this.csvReviewActive());
    readonly seriesPrefill = signal<{ seriesName: string; volumeNumber: number | null } | null>(
        null,
    );

    ngOnInit(): void {
        this.route.queryParamMap.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((params) => {
            const prefill = this.getLastQueryParam(params, 'prefill');
            if (prefill === 'series') {
                this.handleSeriesPrefill(params);
            }
        });
    }

    private getLastQueryParam(params: ParamMap, key: string): string | null {
        const values = params.getAll(key);
        return values.length > 0 ? values[values.length - 1] : null;
    }

    private handleSeriesPrefill(params: ParamMap): void {
        const seriesName = this.getLastQueryParam(params, 'seriesName') ?? '';
        const volumeNumber = Number.parseInt(
            this.getLastQueryParam(params, 'volumeNumber') ?? '',
            10,
        );
        const normalizedVolumeNumber = Number.isNaN(volumeNumber) ? null : volumeNumber;

        // Store series info for later merging when a search result is selected
        this.seriesPrefill.set({
            seriesName,
            volumeNumber: normalizedVolumeNumber,
        });

        // Navigate to Search tab (index 0) instead of Manual Entry
        this.selectedTab.set(0);

        // Show a hint to the user about what they're adding
        const volumeHint =
            normalizedVolumeNumber !== null ? ` (Volume ${normalizedVolumeNumber})` : '';
        this.lastLookupSummary.set(
            `Adding to series: "${seriesName}"${volumeHint}. Search for the book below.`,
        );
    }

    handleLookupStarted(): void {
        this.lookupResults.set([]);
        this.lastLookupSummary.set(null);
    }

    handleLookupCompleted({ drafts, query, label, summary }: LookupCompletedEvent): void {
        this.lookupResults.set(drafts);
        this.lastLookupSummary.set(summary);
        if (drafts.length > 0) {
            this.manualDraft.set({ ...drafts[0] });
            this.manualDraftSource.set({ query, label });
        } else {
            this.manualDraft.set(null);
            this.manualDraftSource.set(null);
        }
    }

    async handleSave(formValue: ItemForm): Promise<void> {
        if (this.busy()) {
            return;
        }

        this.busy.set(true);

        try {
            const duplicates = await firstValueFrom(
                this.itemService
                    .checkDuplicates({
                        title: formValue.title,
                        isbn13: formValue.isbn13,
                        isbn10: formValue.isbn10,
                    })
                    .pipe(
                        takeUntilDestroyed(this.destroyRef),
                        catchError((error) => {
                            console.warn('Duplicate check failed', error);
                            this.notification.warn(
                                'Duplicate check failed; proceeding may create duplicates.',
                            );
                            return of([] as DuplicateMatch[]);
                        }),
                    ),
            );

            if (duplicates.length > 0) {
                const dialogRef = this.dialog.open<
                    DuplicateDialogComponent,
                    DuplicateDialogData,
                    DuplicateDialogResult
                >(DuplicateDialogComponent, {
                    data: {
                        duplicates,
                        totalCount: duplicates.length,
                    },
                    width: '480px',
                    maxHeight: '90vh',
                });

                const decision = await firstValueFrom(dialogRef.afterClosed());
                if (decision !== 'add') {
                    return;
                }
            }

            const item = await firstValueFrom(
                this.itemService.create(formValue).pipe(takeUntilDestroyed(this.destroyRef)),
            );
            if (item) {
                this.seriesPrefill.set(null);
                this.notification.success(`Saved "${item.title}"`);
                await this.router.navigate(['/']);
            }
        } catch (error) {
            console.error('Failed to save item', error);
            this.notification.error('We could not save the item. Double-check required fields.');
        } finally {
            this.busy.set(false);
        }
    }

    handleCancel(): void {
        if (!this.busy()) {
            this.router.navigate(['/']);
        }
    }

    /** Route guard hook: leaving while CSV rows are saving would lose their result. */
    canDeactivate(): boolean {
        if (this.csvImportBusy()) {
            this.notification.warn(
                'Your CSV import is still saving. Stay on this page until it finishes.',
            );
            return false;
        }
        return true;
    }

    handleTabChange(index: number): void {
        this.selectedTab.set(index);
    }

    clearManualDraft(): void {
        this.manualDraft.set(null);
        this.manualDraftSource.set(null);
        this.lookupResults.set([]);
        this.seriesPrefill.set(null);
        this.lastLookupSummary.set(null);
    }

    clearSeriesPrefill(): void {
        this.seriesPrefill.set(null);
        this.lastLookupSummary.set(null);
    }

    private mergeSeriesPrefill(draft: ItemForm): ItemForm {
        const prefill = this.seriesPrefill();
        if (!prefill) {
            return draft;
        }

        return {
            ...draft,
            seriesName: draft.seriesName || prefill.seriesName,
            volumeNumber: draft.volumeNumber ?? prefill.volumeNumber,
        };
    }

    handleQuickAdd(preview: ItemForm): void {
        if (!preview || this.busy()) {
            return;
        }

        // Merge series prefill if present
        const mergedPreview = this.mergeSeriesPrefill({ ...preview });

        // handleSave already handles duplicate checking
        this.handleSave(mergedPreview);
    }

    handleUseForManual(preview: ItemForm): void {
        if (!preview) {
            return;
        }

        // Merge series prefill if present
        const mergedPreview = this.mergeSeriesPrefill({ ...preview });

        const prefill = this.seriesPrefill();
        this.manualDraft.set(mergedPreview);

        // Update source to reflect series prefill if present
        if (prefill) {
            const volumeHint =
                prefill.volumeNumber !== null ? ` (Vol. ${prefill.volumeNumber})` : '';
            this.manualDraftSource.set({
                query: prefill.seriesName,
                label: `Series${volumeHint}`,
            });
        }

        this.selectedTab.set(AddItemPageComponent.MANUAL_ENTRY_TAB_INDEX);
    }
}
