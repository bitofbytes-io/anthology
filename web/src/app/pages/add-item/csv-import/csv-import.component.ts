import { HttpErrorResponse } from '@angular/common/http';
import { Component, ElementRef, inject, output, signal, viewChild } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressSpinnerModule } from '@angular/material/progress-spinner';
import { finalize } from 'rxjs';

import { CsvImportSummary } from '../../../models/import';
import { ItemService } from '../../../services/item.service';

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
const DEFAULT_IMPORT_ERROR = 'Import failed. Confirm the CSV matches the template.';

/**
 * Owns the CSV import flow: file selection and validation, the upload, and the
 * resulting summary or error.
 */
@Component({
    selector: 'app-csv-import',
    standalone: true,
    imports: [MatButtonModule, MatIconModule, MatProgressSpinnerModule],
    templateUrl: './csv-import.component.html',
    styleUrl: './csv-import.component.scss',
})
export class CsvImportComponent {
    private readonly itemService = inject(ItemService);

    /** Emits true while an upload is in flight, so the page can keep this tab open. */
    readonly busyChange = output<boolean>();

    private readonly csvInput = viewChild<ElementRef<HTMLInputElement>>('csvInput');

    readonly csvFields = CSV_FIELDS;
    readonly csvTemplateUrl = '/csv-import-template.csv';

    readonly selectedFile = signal<File | null>(null);
    readonly busy = signal(false);
    readonly summary = signal<CsvImportSummary | null>(null);
    readonly error = signal<string | null>(null);

    handleFileChange(event: Event): void {
        const input = event.target as HTMLInputElement | null;
        const file = input?.files?.[0] ?? null;

        this.summary.set(null);
        const validationError = this.validateCsvFile(file);
        this.error.set(validationError);
        if (validationError) {
            this.selectedFile.set(null);
            this.resetInput();
            return;
        }

        this.selectedFile.set(file);
    }

    handleSubmit(event?: Event): void {
        event?.preventDefault();
        event?.stopPropagation();

        const file = this.selectedFile() ?? this.csvInput()?.nativeElement.files?.[0] ?? null;
        if (!file || this.busy()) {
            return;
        }

        const validationError = this.validateCsvFile(file);
        if (validationError) {
            this.error.set(validationError);
            return;
        }

        this.setBusy(true);
        this.error.set(null);
        this.summary.set(null);

        // Not tied to this component's lifetime: the page keeps the tab open
        // while busy, and an abandoned request would cut the import short.
        this.itemService
            .importCsv(file)
            .pipe(finalize(() => this.setBusy(false)))
            .subscribe({
                next: (summary) => {
                    this.summary.set({
                        ...summary,
                        skippedDuplicates: summary.skippedDuplicates ?? [],
                        failed: summary.failed ?? [],
                    });
                    this.clearSelectedFile();
                },
                error: (error: unknown) => {
                    this.error.set(importErrorMessage(error));
                },
            });
    }

    handleReset(): void {
        this.clearSelectedFile();
        this.summary.set(null);
        this.error.set(null);
    }

    private setBusy(busy: boolean): void {
        this.busy.set(busy);
        this.busyChange.emit(busy);
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

function importErrorMessage(error: unknown): string {
    if (error instanceof HttpErrorResponse) {
        const serverMessage =
            typeof error.error?.error === 'string' ? error.error.error.trim() : '';
        if (serverMessage) {
            return serverMessage;
        }
    }
    return DEFAULT_IMPORT_ERROR;
}
