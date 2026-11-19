import { HttpErrorResponse } from '@angular/common/http';
import {
    ChangeDetectionStrategy,
    Component,
    DestroyRef,
    computed,
    inject,
    input,
    output,
    signal,
    viewChild,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { MatRadioModule } from '@angular/material/radio';
import { finalize } from 'rxjs';

import { BarcodeScannerPanelComponent } from '../../../components/shelves/barcode-scanner-panel/barcode-scanner-panel.component';
import { ItemForm } from '../../../models';
import { ItemLookupCategory, ItemLookupService } from '../../../services/item-lookup.service';
import { composeLookupDraft } from './compose-lookup-draft';
import { SEARCH_CATEGORIES, getSearchCategory } from './search-categories';

/** Outcome of a lookup; `drafts` is empty when nothing matched or the request failed. */
export interface LookupCompletedEvent {
    drafts: ItemForm[];
    query: string;
    label: string;
    summary: string | null;
}

@Component({
    selector: 'app-search-panel',
    standalone: true,
    imports: [
        MatButtonModule,
        MatFormFieldModule,
        MatIconModule,
        MatInputModule,
        MatProgressSpinnerModule,
        MatRadioModule,
        ReactiveFormsModule,
        BarcodeScannerPanelComponent,
    ],
    templateUrl: './search-panel.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './search-panel.component.scss',
})
export class SearchPanelComponent {
    private readonly itemLookupService = inject(ItemLookupService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly fb = inject(FormBuilder);

    private readonly scannerPanel = viewChild(BarcodeScannerPanelComponent);

    /** Message from the page (lookup summary or series hint), shown when there is no error. */
    readonly summary = input<string | null>(null);

    /** Emitted when a lookup begins so the page can discard the previous results. */
    readonly lookupStarted = output<void>();
    readonly lookupCompleted = output<LookupCompletedEvent>();

    readonly lookupBusy = signal(false);
    readonly lookupError = signal<string | null>(null);
    readonly scanning = signal(false);

    readonly searchCategories = SEARCH_CATEGORIES;

    readonly searchForm = this.fb.group({
        category: [SEARCH_CATEGORIES[0].value as ItemLookupCategory, Validators.required],
        query: ['', [Validators.required, Validators.minLength(3)]],
    });

    readonly activeCategory = computed(() =>
        getSearchCategory(this.searchForm.get('category')?.value as ItemLookupCategory | null),
    );

    handleDetectedBarcode(rawValue: string): void {
        const value = rawValue.trim();
        if (!value) {
            return;
        }

        this.searchForm.get('query')?.setValue(value);
        this.handleLookupSubmit('scanner');
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

        const rawCategory = this.searchForm.get('category')?.value as ItemLookupCategory | null;
        const rawQuery = this.searchForm.get('query')?.value ?? '';
        const query = rawQuery.trim();

        if (!rawCategory || !query) {
            this.searchForm.get('query')?.setErrors({ required: true });
            if (source === 'scanner') {
                this.rejectScannedBarcode();
            }
            return;
        }

        const category = getSearchCategory(rawCategory);

        this.lookupBusy.set(true);
        this.lookupError.set(null);
        this.lookupStarted.emit();

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
                    const drafts = results.map((partial) => composeLookupDraft(partial, category));
                    let summary: string | null = null;
                    if (drafts.length === 0) {
                        this.lookupError.set(
                            'No results found. Try another barcode or type the ISBN.',
                        );
                    } else if (source !== 'scanner') {
                        summary =
                            drafts.length > 1
                                ? `Loaded ${drafts.length} matches for "${query}". Choose one below.`
                                : `Metadata loaded for "${query}".`;
                    }
                    this.lookupCompleted.emit({ drafts, query, label: category.label, summary });
                },
                error: (error) => {
                    this.lookupError.set(this.describeLookupError(error));
                    this.lookupCompleted.emit({
                        drafts: [],
                        query,
                        label: category.label,
                        summary: null,
                    });
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

    private describeLookupError(error: unknown): string {
        if (error instanceof HttpErrorResponse) {
            const serverMessage =
                typeof error.error?.error === 'string' ? error.error.error.trim() : '';
            if (serverMessage) {
                return serverMessage;
            }
            if (error.status === 404) {
                return 'No results found. Try another barcode or type the ISBN.';
            }
        }
        return "We couldn't find a match. Try another ISBN.";
    }
}
