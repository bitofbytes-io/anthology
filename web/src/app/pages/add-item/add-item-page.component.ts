import { HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, OnInit, computed, inject, signal, viewChild } from '@angular/core';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatCardModule } from '@angular/material/card';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog, MatDialogModule } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import { ActivatedRoute, ParamMap, Router, RouterModule } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatTabsModule } from '@angular/material/tabs';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatRadioModule } from '@angular/material/radio';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { catchError, finalize, of, switchMap, firstValueFrom } from 'rxjs';

import { ItemFormComponent } from '../../components/item-form/item-form.component';
import {
    DuplicateDialogComponent,
    DuplicateDialogData,
    DuplicateDialogResult,
} from '../../components/duplicate-dialog/duplicate-dialog.component';
import { DuplicateMatch, ItemForm } from '../../models';
import { ItemService } from '../../services/item.service';
import { ItemLookupCategory, ItemLookupService } from '../../services/item-lookup.service';
import { CsvImportComponent } from './csv-import/csv-import.component';
import { BarcodeScannerPanelComponent } from '../../components/shelves/barcode-scanner-panel/barcode-scanner-panel.component';
import { LookupResultsComponent } from './lookup-results/lookup-results.component';
import { NotificationService } from '../../services/notification.service';

type SearchCategoryValue = ItemLookupCategory;

interface SearchCategoryConfig {
    value: SearchCategoryValue;
    label: string;
    description: string;
    inputLabel: string;
    placeholder: string;
    itemType: ItemForm['itemType'];
    disabled?: boolean;
}

@Component({
    selector: 'app-add-item-page',
    standalone: true,
    imports: [
        MatButtonModule,
        MatCardModule,
        MatDialogModule,
        MatFormFieldModule,
        MatIconModule,
        MatInputModule,
        MatProgressSpinnerModule,
        MatRadioModule,
        MatTabsModule,
        ReactiveFormsModule,
        RouterModule,
        ItemFormComponent,
        CsvImportComponent,
        BarcodeScannerPanelComponent,
        LookupResultsComponent,
    ],
    templateUrl: './add-item-page.component.html',
    styleUrl: './add-item-page.component.scss',
})
export class AddItemPageComponent implements OnInit {
    private static readonly SEARCH_CATEGORIES: SearchCategoryConfig[] = [
        {
            value: 'book',
            label: 'Book',
            description: 'Search by ISBN or keyword to auto-fill book details.',
            inputLabel: 'Search for books',
            placeholder: 'ISBN or title keyword',
            itemType: 'book',
        },
        {
            value: 'game',
            label: 'Game',
            description: 'Look up tabletop or video releases by UPC or title.',
            inputLabel: 'Search for games',
            placeholder: 'UPC or title keyword',
            itemType: 'game',
            disabled: true,
        },
        {
            value: 'movie',
            label: 'Movie',
            description: 'Use UPC or keywords to find film metadata.',
            inputLabel: 'Search for movies',
            placeholder: 'UPC or title keyword',
            itemType: 'movie',
            disabled: true,
        },
        {
            value: 'music',
            label: 'Music',
            description: 'Find album details with UPC or artist keywords.',
            inputLabel: 'Search for music',
            placeholder: 'UPC or title keyword',
            itemType: 'music',
            disabled: true,
        },
    ];

    private static readonly MANUAL_ENTRY_TAB_INDEX = 1;

    private readonly itemService = inject(ItemService);
    private readonly itemLookupService = inject(ItemLookupService);
    private readonly router = inject(Router);
    private readonly route = inject(ActivatedRoute);
    private readonly notification = inject(NotificationService);
    private readonly dialog = inject(MatDialog);
    private readonly destroyRef = inject(DestroyRef);
    private readonly fb = inject(FormBuilder);

    private readonly scannerPanel = viewChild(BarcodeScannerPanelComponent);

    readonly busy = signal(false);
    readonly lookupBusy = signal(false);
    readonly lookupError = signal<string | null>(null);
    readonly lookupResults = signal<ItemForm[]>([]);
    readonly manualDraft = signal<ItemForm | null>(null);
    readonly manualDraftSource = signal<{ query: string; label: string } | null>(null);
    readonly lastLookupSummary = signal<string | null>(null);
    readonly selectedTab = signal(0);
    /** True while the CSV tab is uploading; the other tabs are disabled meanwhile. */
    readonly csvImportBusy = signal(false);
    readonly scanning = signal(false);
    readonly seriesPrefill = signal<{ seriesName: string; volumeNumber: number | null } | null>(
        null,
    );

    readonly searchCategories = AddItemPageComponent.SEARCH_CATEGORIES;

    readonly searchForm = this.fb.group({
        category: [
            AddItemPageComponent.SEARCH_CATEGORIES[0].value as SearchCategoryValue,
            Validators.required,
        ],
        query: ['', [Validators.required, Validators.minLength(3)]],
    });

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

    readonly activeCategory = computed(() => {
        const value = this.searchForm.get('category')?.value as SearchCategoryValue | null;
        return (
            AddItemPageComponent.SEARCH_CATEGORIES.find((category) => category.value === value) ??
            AddItemPageComponent.SEARCH_CATEGORIES[0]
        );
    });

    handleDetectedBarcode(rawValue: string): void {
        const value = rawValue.trim();
        if (!value) {
            return;
        }

        this.searchForm.get('query')?.setValue(value);
        this.handleLookupSubmit('scanner');
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

    handleLookupSubmit(source: 'manual' | 'scanner' = 'manual'): void {
        if (this.lookupBusy()) {
            return;
        }

        if (source === 'manual') {
            this.scanning.set(false);
        }

        if (this.searchForm.invalid) {
            this.searchForm.markAllAsTouched();
            if (source === 'scanner') {
                this.rejectScannedBarcode();
            }
            return;
        }

        const rawCategory = this.searchForm.get('category')?.value as SearchCategoryValue | null;
        const rawQuery = this.searchForm.get('query')?.value ?? '';
        const query = rawQuery.trim();

        if (!rawCategory || !query) {
            this.searchForm.get('query')?.setErrors({ required: true });
            if (source === 'scanner') {
                this.rejectScannedBarcode();
            }
            return;
        }

        const category = this.getCategoryConfig(rawCategory);

        this.lookupBusy.set(true);
        this.lookupError.set(null);
        this.lookupResults.set([]);
        this.lastLookupSummary.set(null);

        this.itemLookupService
            .lookup(query, rawCategory)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => {
                    this.lookupBusy.set(false);
                    if (source === 'scanner') {
                        this.scanning.set(false);
                    }
                }),
            )
            .subscribe({
                next: (results) => {
                    const drafts = results.map((partial) => this.composeDraft(partial, category));
                    this.lookupResults.set(drafts);
                    if (drafts.length > 0) {
                        this.manualDraft.set({ ...drafts[0] });
                        this.manualDraftSource.set({
                            query,
                            label: category.label,
                        });
                        if (source !== 'scanner') {
                            const summary =
                                drafts.length > 1
                                    ? `Loaded ${drafts.length} matches for "${query}". Choose one below.`
                                    : `Metadata loaded for "${query}".`;
                            this.lastLookupSummary.set(summary);
                        }
                    } else {
                        this.manualDraft.set(null);
                        this.manualDraftSource.set(null);
                        this.lastLookupSummary.set(null);

                        this.lookupError.set(
                            'No results found. Try another barcode or type the ISBN.',
                        );
                    }
                },
                error: (error) => {
                    this.manualDraft.set(null);
                    this.manualDraftSource.set(null);
                    this.lookupResults.set([]);

                    let message = "We couldn't find a match. Try another ISBN.";
                    if (error instanceof HttpErrorResponse) {
                        const serverMessage =
                            typeof error.error?.error === 'string' ? error.error.error.trim() : '';
                        if (serverMessage) {
                            message = serverMessage;
                        } else if (error.status === 404) {
                            message = 'No results found. Try another barcode or type the ISBN.';
                        }
                    }

                    this.lookupError.set(message);
                },
            });
    }

    toggleScanning(): void {
        if (this.scanning()) {
            this.scanning.set(false);
            return;
        }
        this.lookupError.set(null);
        this.scanning.set(true);
    }

    handleScannerFailed(message: string): void {
        this.scanning.set(false);
        this.lookupError.set(message);
    }

    private rejectScannedBarcode(): void {
        // Keep the camera open so the user can try another barcode.
        this.scannerPanel()?.reportScanComplete();
        this.lookupError.set('That barcode was not valid. Try again or type the ISBN.');
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

    private getCategoryConfig(value: SearchCategoryValue): SearchCategoryConfig {
        return (
            AddItemPageComponent.SEARCH_CATEGORIES.find((category) => category.value === value) ??
            AddItemPageComponent.SEARCH_CATEGORIES[0]
        );
    }

    private composeDraft(partial: Partial<ItemForm>, category: SearchCategoryConfig): ItemForm {
        const releaseYear = partial.releaseYear;
        let normalizedReleaseYear: number | null = null;
        const pageCount = partial.pageCount;
        let normalizedPageCount: number | null = null;
        const retailPriceUsd = partial.retailPriceUsd;
        let normalizedRetailPriceUsd: number | null = null;

        if (typeof releaseYear === 'number') {
            normalizedReleaseYear = releaseYear;
        } else if (typeof releaseYear === 'string') {
            const parsed = Number.parseInt(releaseYear, 10);
            normalizedReleaseYear = Number.isNaN(parsed) ? null : parsed;
        }

        if (typeof pageCount === 'number') {
            normalizedPageCount = pageCount;
        } else if (typeof pageCount === 'string') {
            const parsed = Number.parseInt(pageCount, 10);
            normalizedPageCount = Number.isNaN(parsed) ? null : parsed;
        }

        if (typeof retailPriceUsd === 'number') {
            normalizedRetailPriceUsd = retailPriceUsd;
        } else if (typeof retailPriceUsd === 'string') {
            const parsed = Number.parseFloat(retailPriceUsd);
            normalizedRetailPriceUsd = Number.isNaN(parsed) ? null : parsed;
        }

        return {
            title: partial.title ?? '',
            creator: partial.creator ?? '',
            itemType: category.itemType,
            releaseYear: normalizedReleaseYear,
            pageCount: normalizedPageCount,
            isbn13: partial.isbn13 ?? '',
            isbn10: partial.isbn10 ?? '',
            description: partial.description ?? '',
            coverImage: partial.coverImage ?? '',
            genre: partial.genre,
            retailPriceUsd: normalizedRetailPriceUsd,
            googleVolumeId: partial.googleVolumeId ?? '',
            platform: partial.platform ?? '',
            ageGroup: partial.ageGroup ?? '',
            playerCount: partial.playerCount ?? '',
            notes: partial.notes ?? '',
            seriesName: partial.seriesName ?? '',
            volumeNumber: partial.volumeNumber ?? null,
            totalVolumes: partial.totalVolumes ?? null,
        };
    }
}
