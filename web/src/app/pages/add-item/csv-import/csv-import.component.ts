import { HttpErrorResponse } from '@angular/common/http';
import {
    Component,
    DestroyRef,
    ElementRef,
    Injector,
    afterNextRender,
    computed,
    inject,
    output,
    signal,
    viewChild,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { RouterModule } from '@angular/router';
import { Subscription, finalize } from 'rxjs';

import { CsvImportPreview } from '../../../models/import';
import { ItemService } from '../../../services/item.service';
import { NotificationService } from '../../../services/notification.service';
import {
    ImportOutcome,
    ReviewRow,
    commitRows,
    composeOutcome,
    countReview,
    duplicateReason,
    reviewRows,
} from './csv-import-review';
import {
    CsvImportReviewComponent,
    RowChoice,
    RowToggle,
} from './import-review/import-review.component';
import { CsvImportResultComponent } from './import-result/import-result.component';

const CSV_MAX_FILE_SIZE_BYTES = 5 * 1024 * 1024; // 5 MB - matches server limit
const CSV_ALLOWED_MIME_TYPES = ['text/csv', 'application/vnd.ms-excel'];
const CSV_ALLOWED_EXTENSIONS = ['.csv'];
const CSV_FIELDS = [
    'title',
    'creator',
    'itemType',
    'releaseYear',
    'pageCount',
    'isbn13',
    'isbn10',
    'description',
    'coverImage',
    'notes',
    'platform',
    'ageGroup',
    'playerCount',
];
const DEFAULT_PREVIEW_ERROR = 'We could not check this file. Confirm the CSV matches the template.';
const SIGNED_OUT_WHILE_SAVING =
    'You were signed out while your CSV import was saving, so some rows may have been saved. After you sign in, preview the file again: saved rows show as possible duplicates.';

/**
 * - upload: choose a file and preview it (read-only).
 * - review: decide which rows to import; nothing has been saved.
 * - committing: the selected rows are being saved.
 * - result: the server reported what happened to every row.
 * - unknown: the commit request failed in transit, so some rows may be saved.
 */
export type CsvImportStage = 'upload' | 'review' | 'committing' | 'result' | 'unknown';

/**
 * Owns the CSV import flow: the file is previewed without saving anything, the
 * user reviews each row and picks catalog editions, and only then are the
 * selected rows saved exactly as reviewed.
 */
@Component({
    selector: 'app-csv-import',
    standalone: true,
    imports: [
        MatButtonModule,
        MatIconModule,
        MatProgressSpinnerModule,
        RouterModule,
        CsvImportReviewComponent,
        CsvImportResultComponent,
    ],
    templateUrl: './csv-import.component.html',
    styleUrl: './csv-import.component.scss',
    host: { '(window:beforeunload)': 'handleBeforeUnload($event)' },
})
export class CsvImportComponent {
    private readonly itemService = inject(ItemService);
    private readonly notification = inject(NotificationService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly injector = inject(Injector);
    private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

    /** Emits true while rows are being saved, so the page keeps this tab and route open. */
    readonly busyChange = output<boolean>();
    /** Emits true while a review is open or saving, so the page keeps this tab selected. */
    readonly reviewActiveChange = output<boolean>();

    private readonly csvInput = viewChild<ElementRef<HTMLInputElement>>('csvInput');

    readonly csvFields = CSV_FIELDS;
    readonly csvTemplateUrl = '/csv-import-template.csv';

    readonly stage = signal<CsvImportStage>('upload');
    readonly selectedFile = signal<File | null>(null);
    readonly previewBusy = signal(false);
    readonly preview = signal<CsvImportPreview | null>(null);
    readonly choices = signal<ReadonlyMap<number, number>>(new Map());
    readonly deselected = signal<ReadonlySet<number>>(new Set());
    readonly outcome = signal<ImportOutcome | null>(null);
    readonly error = signal<string | null>(null);
    /** Polite screen reader announcement of the latest review change. */
    readonly announcement = signal('');
    /** Number of rows sent in the last commit. */
    readonly sentRows = signal(0);

    readonly rows = computed<ReviewRow[]>(() => {
        const preview = this.preview();
        return preview ? reviewRows(preview.rows, this.choices(), this.deselected()) : [];
    });
    readonly counts = computed(() => countReview(this.rows()));
    readonly committing = computed(() => this.stage() === 'committing');

    /** The file the current preview came from, kept to preview it again. */
    private previewedFile: File | null = null;
    private previewSubscription: Subscription | null = null;

    constructor() {
        // The page only lets this component go while saving when a sign-out
        // redirects to the login page. The request may still save rows.
        this.destroyRef.onDestroy(() => {
            if (this.committing()) {
                this.notification.warn(SIGNED_OUT_WHILE_SAVING, { duration: 15000 });
            }
        });
    }

    handleFileChange(event: Event): void {
        const input = event.target as HTMLInputElement | null;
        const file = input?.files?.[0] ?? null;

        const validationError = this.validateCsvFile(file);
        this.error.set(validationError);
        if (validationError) {
            this.selectedFile.set(null);
            this.resetInput();
            return;
        }

        this.selectedFile.set(file);
    }

    handlePreview(event?: Event): void {
        event?.preventDefault();
        event?.stopPropagation();

        const file = this.selectedFile() ?? this.csvInput()?.nativeElement.files?.[0] ?? null;
        if (!file || this.previewBusy() || this.stage() !== 'upload') {
            return;
        }

        const validationError = this.validateCsvFile(file);
        if (validationError) {
            this.error.set(validationError);
            return;
        }
        this.runPreview(file);
    }

    /** Stops a preview in progress. Previews never save anything. */
    handleCancelPreview(): void {
        this.previewSubscription?.unsubscribe();
        this.previewSubscription = null;
        this.previewBusy.set(false);
        this.announcement.set('Preview cancelled. Nothing was imported.');
    }

    handleToggle(change: RowToggle): void {
        if (this.stage() !== 'review') {
            return;
        }
        this.deselected.update((current) => {
            const next = new Set(current);
            if (change.selected) {
                next.delete(change.row);
            } else {
                next.add(change.row);
            }
            return next;
        });
    }

    handleSelectAll(): void {
        if (this.stage() === 'review') {
            this.deselected.set(new Set());
        }
    }

    handleSelectNone(): void {
        if (this.stage() === 'review') {
            this.deselected.set(
                new Set(
                    this.rows()
                        .filter((row) => row.status === 'ready')
                        .map((row) => row.source.row),
                ),
            );
        }
    }

    handleChoose(choice: RowChoice): void {
        if (this.stage() !== 'review') {
            return;
        }
        const before = this.rows();
        this.choices.update((current) => {
            const next = new Map(current);
            if (choice.index === null) {
                next.delete(choice.row);
            } else {
                next.set(choice.row, choice.index);
            }
            return next;
        });
        this.announcement.set(describeChoice(before, this.rows(), choice.row));
    }

    handleCommit(): void {
        if (this.stage() !== 'review') {
            return;
        }
        const rows = this.rows();
        const request = commitRows(rows);
        if (request.length === 0) {
            return;
        }

        this.error.set(null);
        this.sentRows.set(request.length);
        this.setStage('committing');

        // Not tied to this component's lifetime: the page blocks navigation
        // while saving, and an abandoned request would not stop the import.
        this.itemService
            .commitCsvImport(request)
            .pipe(finalize(() => this.busyChange.emit(false)))
            .subscribe({
                next: (result) => {
                    const outcome = composeOutcome(rows, result);
                    this.outcome.set(outcome);
                    this.setStage('result');
                    this.announcement.set(
                        `Import finished: ${outcome.counts.added} added, ${outcome.counts.skipped} skipped.`,
                    );
                },
                error: (error: unknown) => {
                    if (rejectedBeforeSaving(error)) {
                        this.error.set(
                            `Nothing was imported. ${serverMessage(error) ?? ''}`.trim(),
                        );
                        this.setStage('review');
                        return;
                    }
                    this.setStage('unknown');
                },
            });
    }

    /** Previews the same file again, e.g. to see which rows a failed import saved. */
    handlePreviewAgain(): void {
        const file = this.previewedFile;
        if (!file || this.committing()) {
            return;
        }
        this.clearReview();
        this.setStage('upload');
        this.selectedFile.set(file);
        this.runPreview(file);
    }

    /** Discards the review or result and starts over. Nothing is saved. */
    handleReset(): void {
        if (this.committing()) {
            return;
        }
        this.previewSubscription?.unsubscribe();
        this.previewSubscription = null;
        this.previewBusy.set(false);
        this.clearReview();
        this.previewedFile = null;
        this.clearSelectedFile();
        this.error.set(null);
        this.setStage('upload');
    }

    handleBeforeUnload(event: BeforeUnloadEvent): void {
        if (this.committing()) {
            event.preventDefault();
            event.returnValue = '';
        }
    }

    private runPreview(file: File): void {
        this.previewBusy.set(true);
        this.error.set(null);
        this.announcement.set('');

        this.previewSubscription = this.itemService
            .previewCsvImport(file)
            .pipe(
                takeUntilDestroyed(this.destroyRef),
                finalize(() => this.previewBusy.set(false)),
            )
            .subscribe({
                next: (preview) => {
                    this.previewedFile = file;
                    this.preview.set({ ...preview, rows: preview.rows ?? [] });
                    this.choices.set(new Map());
                    this.deselected.set(new Set());
                    this.clearSelectedFile();
                    this.setStage('review');
                    const counts = this.counts();
                    this.announcement.set(
                        `Preview ready: ${counts.ready} ready, ${counts.duplicate} possible duplicates, ${counts.needsMatch} need a match.`,
                    );
                },
                error: (error: unknown) => {
                    this.error.set(serverMessage(error) ?? DEFAULT_PREVIEW_ERROR);
                },
            });
    }

    private setStage(stage: CsvImportStage): void {
        const previous = this.stage();
        this.stage.set(stage);
        this.reviewActiveChange.emit(stage === 'review' || stage === 'committing');
        if (stage === 'committing') {
            this.busyChange.emit(true);
        } else if (stage !== previous) {
            this.focusStageHeading();
        }
    }

    /** Moves focus to the new stage's heading, since the control that led there is gone. */
    private focusStageHeading(): void {
        afterNextRender(
            () =>
                this.host.nativeElement.querySelector<HTMLElement>('[data-stage-heading]')?.focus(),
            { injector: this.injector },
        );
    }

    private clearReview(): void {
        this.preview.set(null);
        this.choices.set(new Map());
        this.deselected.set(new Set());
        this.outcome.set(null);
        this.sentRows.set(0);
    }

    private validateCsvFile(file: File | null): string | null {
        if (!file) {
            return null;
        }

        const fileName = file.name.toLowerCase();
        const hasValidExtension = CSV_ALLOWED_EXTENSIONS.some((ext) => fileName.endsWith(ext));
        if (!hasValidExtension) {
            return 'Only CSV files are allowed.';
        }

        if (file.type && !CSV_ALLOWED_MIME_TYPES.includes(file.type)) {
            return 'Only CSV files are allowed.';
        }

        if (file.size > CSV_MAX_FILE_SIZE_BYTES) {
            const maxSizeMB = CSV_MAX_FILE_SIZE_BYTES / (1024 * 1024);
            return `File size exceeds ${maxSizeMB} MB limit.`;
        }

        return null;
    }

    private clearSelectedFile(): void {
        this.selectedFile.set(null);
        this.resetInput();
    }

    private resetInput(): void {
        const input = this.csvInput()?.nativeElement;
        if (input) {
            input.value = '';
        }
    }
}

function serverMessage(error: unknown): string | null {
    if (error instanceof HttpErrorResponse) {
        const message = typeof error.error?.error === 'string' ? error.error.error.trim() : '';
        return message || null;
    }
    return null;
}

/**
 * The API answers 4xx only before any row is saved: auth, origin, size, and
 * request validation. Anything else (no response, a gateway error, a timeout)
 * leaves the outcome unknown.
 */
function rejectedBeforeSaving(error: unknown): boolean {
    return (
        error instanceof HttpErrorResponse &&
        error.status >= 400 &&
        error.status < 500 &&
        error.status !== 408
    );
}

/** Describes how choosing (or clearing) an edition for `rowNumber` changed the review. */
function describeChoice(
    before: readonly ReviewRow[],
    after: readonly ReviewRow[],
    rowNumber: number,
): string {
    const messages: string[] = [];
    after.forEach((row, index) => {
        const previous = before[index];
        if (row.source.row === rowNumber) {
            if (row.status === 'ready') {
                messages.push(`Row ${rowNumber} is ready to import.`);
            } else if (row.status === 'duplicate') {
                messages.push(
                    `Row ${rowNumber} will be skipped: ${duplicateReason(row.libraryMatches, row.fileMatch)}.`,
                );
            } else {
                messages.push(`Row ${rowNumber} needs a match again.`);
            }
            return;
        }
        if (previous.status !== row.status) {
            if (row.status === 'duplicate') {
                messages.push(
                    `Row ${row.source.row} will now be skipped: ${duplicateReason(row.libraryMatches, row.fileMatch)}.`,
                );
            } else if (row.status === 'ready') {
                messages.push(`Row ${row.source.row} is ready to import again.`);
            }
        }
    });
    return messages.join(' ');
}
