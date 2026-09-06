import {
    Component,
    EventEmitter,
    Input,
    OnChanges,
    OnInit,
    Output,
    SimpleChanges,
    inject,
} from '@angular/core';
import { FormBuilder, FormGroup, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';

import {
    BookStatus,
    BOOK_STATUS_LABELS,
    Format,
    FORMAT_LABELS,
    Formats,
    Genre,
    GENRE_LABELS,
    Genres,
    Item,
    ItemForm,
    ItemType,
    ITEM_TYPE_LABELS,
} from '../../models';
import { BookDetailsComponent } from './book-details/book-details.component';
import { CoverSectionComponent } from './cover-section/cover-section.component';
import { GameDetailsComponent } from './game-details/game-details.component';

@Component({
    selector: 'app-item-form',
    standalone: true,
    imports: [
        BookDetailsComponent,
        CoverSectionComponent,
        GameDetailsComponent,
        MatButtonModule,
        MatFormFieldModule,
        MatIconModule,
        MatInputModule,
        MatSelectModule,
        ReactiveFormsModule,
    ],
    templateUrl: './item-form.component.html',
    styleUrl: './item-form.component.scss',
})
export class ItemFormComponent implements OnChanges, OnInit {
    private readonly fb = inject(FormBuilder);

    @Input() item: Item | null = null;
    @Input() draft: Partial<ItemForm> | null = null;
    @Input() mode: 'create' | 'edit' = 'create';
    @Input() busy = false;

    @Input() resyncing = false;

    @Output() readonly save = new EventEmitter<ItemForm>();
    @Output() readonly cancelled = new EventEmitter<void>();
    @Output() readonly deleteRequested = new EventEmitter<void>();
    @Output() readonly resyncRequested = new EventEmitter<void>();

    readonly itemTypeOptions = Object.entries(ITEM_TYPE_LABELS) as [ItemType, string][];
    readonly bookStatusOptions: Array<{ value: BookStatus; label: string }> = (
        Object.entries(BOOK_STATUS_LABELS) as [BookStatus, string][]
    ).map(([value, label]) => ({
        value,
        label,
    }));
    readonly formatOptions: Array<{ value: Format; label: string }> = (
        Object.entries(FORMAT_LABELS) as [Format, string][]
    ).map(([value, label]) => ({
        value,
        label,
    }));
    readonly genreOptions: Array<{ value: Genre | ''; label: string }> = [
        { value: '', label: 'None' },
        ...(Object.entries(GENRE_LABELS) as [Genre, string][]).map(([value, label]) => ({
            value,
            label,
        })),
    ];

    coverImageError: string | null = null;

    readonly form: FormGroup = this.fb.group({
        title: ['', [Validators.required, Validators.maxLength(120)]],
        creator: ['', [Validators.maxLength(120)]],
        itemType: ['book' as ItemType, Validators.required],
        releaseYear: [null, [Validators.min(0)]],
        pageCount: [null, [Validators.min(1)]],
        currentPage: [null, [Validators.min(0)]],
        isbn13: ['', [Validators.maxLength(20)]],
        isbn10: ['', [Validators.maxLength(20)]],
        description: ['', [Validators.maxLength(2000)]],
        coverImage: [''],
        format: [Formats.Unknown as Format],
        genre: ['' as Genre | ''],
        rating: [null, [Validators.min(1), Validators.max(10)]],
        retailPriceUsd: [null, [Validators.min(0)]],
        googleVolumeId: [''],
        platform: ['', [Validators.maxLength(200)]],
        ageGroup: ['', [Validators.maxLength(100)]],
        playerCount: ['', [Validators.maxLength(100)]],
        readingStatus: [BookStatus.None],
        readAt: [null],
        notes: ['', [Validators.maxLength(500)]],
        seriesName: ['', [Validators.maxLength(200)]],
        volumeNumber: [null, [Validators.min(1), Validators.max(200)]],
        totalVolumes: [null, [Validators.min(1), Validators.max(200)]],
    });

    get isBook(): boolean {
        return this.form.get('itemType')?.value === 'book';
    }

    get isGame(): boolean {
        return this.form.get('itemType')?.value === 'game';
    }

    get hasSeriesData(): boolean {
        const seriesName = this.form.get('seriesName')?.value;
        const volumeNumber = this.form.get('volumeNumber')?.value;
        const totalVolumes = this.form.get('totalVolumes')?.value;
        return !!(seriesName?.trim() || volumeNumber !== null || totalVolumes !== null);
    }

    ngOnInit(): void {
        this.form
            .get('itemType')
            ?.valueChanges.subscribe((type: ItemType) => this.handleItemTypeChange(type));
    }

    ngOnChanges(changes: SimpleChanges): void {
        if (changes['item'] || changes['draft']) {
            const next: ItemForm = {
                title: '',
                creator: '',
                itemType: 'book',
                releaseYear: null,
                pageCount: null,
                currentPage: null,
                isbn13: '',
                isbn10: '',
                description: '',
                coverImage: '',
                format: Formats.Unknown,
                genre: undefined,
                rating: null,
                retailPriceUsd: null,
                googleVolumeId: '',
                platform: '',
                ageGroup: '',
                playerCount: '',
                readingStatus: BookStatus.None,
                readAt: null,
                notes: '',
                seriesName: '',
                volumeNumber: null,
                totalVolumes: null,
            };

            if (this.draft) {
                next.title = this.draft.title ?? next.title;
                next.creator = this.draft.creator ?? next.creator;
                next.itemType = this.draft.itemType ?? next.itemType;

                const draftReleaseYear = this.draft.releaseYear;
                if (draftReleaseYear === undefined || draftReleaseYear === null) {
                    next.releaseYear = null;
                } else if (typeof draftReleaseYear === 'string') {
                    const parsed = Number.parseInt(draftReleaseYear, 10);
                    next.releaseYear = Number.isNaN(parsed) ? null : parsed;
                } else {
                    next.releaseYear = draftReleaseYear;
                }

                const draftPageCount = this.draft.pageCount;
                if (draftPageCount === undefined || draftPageCount === null) {
                    next.pageCount = null;
                } else if (typeof draftPageCount === 'string') {
                    const parsed = Number.parseInt(draftPageCount, 10);
                    next.pageCount = Number.isNaN(parsed) ? null : parsed;
                } else {
                    next.pageCount = draftPageCount;
                }

                const draftCurrentPage = this.draft.currentPage;
                if (draftCurrentPage === undefined || draftCurrentPage === null) {
                    next.currentPage = null;
                } else if (typeof draftCurrentPage === 'string') {
                    const parsed = Number.parseInt(draftCurrentPage, 10);
                    next.currentPage = Number.isNaN(parsed) ? null : parsed;
                } else {
                    next.currentPage = draftCurrentPage;
                }

                next.notes = this.draft.notes ?? next.notes;
                next.isbn13 = this.draft.isbn13 ?? next.isbn13;
                next.isbn10 = this.draft.isbn10 ?? next.isbn10;
                next.description = this.draft.description ?? next.description;
                next.coverImage = this.draft.coverImage ?? next.coverImage;
                next.format = this.draft.format ?? next.format;
                next.genre = this.draft.genre ?? next.genre;
                next.rating = this.parseInteger(this.draft.rating) ?? next.rating;
                next.retailPriceUsd =
                    this.parseNumber(this.draft.retailPriceUsd) ?? next.retailPriceUsd;
                next.googleVolumeId = this.draft.googleVolumeId ?? next.googleVolumeId;
                next.platform = this.draft.platform ?? next.platform;
                next.ageGroup = this.draft.ageGroup ?? next.ageGroup;
                next.playerCount = this.draft.playerCount ?? next.playerCount;
                next.readingStatus =
                    this.normalizeStatus(this.draft.readingStatus) ?? next.readingStatus;
                next.readAt = this.normalizeDateInput(this.draft.readAt) ?? next.readAt;
                next.seriesName = this.draft.seriesName ?? next.seriesName;
                next.volumeNumber = this.parseInteger(this.draft.volumeNumber) ?? next.volumeNumber;
                next.totalVolumes = this.parseInteger(this.draft.totalVolumes) ?? next.totalVolumes;
            }

            if (this.item) {
                next.title = this.item.title;
                next.creator = this.item.creator;
                next.itemType = this.item.itemType;
                next.releaseYear = this.item.releaseYear ?? null;
                next.pageCount = this.item.pageCount ?? null;
                next.currentPage = this.item.currentPage ?? null;
                next.isbn13 = this.item.isbn13 ?? '';
                next.isbn10 = this.item.isbn10 ?? '';
                next.description = this.item.description ?? '';
                next.coverImage = this.item.coverImage ?? '';
                next.format = this.item.format ?? Formats.Unknown;
                next.genre = this.item.genre;
                next.rating = this.item.rating ?? null;
                next.retailPriceUsd = this.item.retailPriceUsd ?? null;
                next.googleVolumeId = this.item.googleVolumeId ?? '';
                next.platform = this.item.platform ?? '';
                next.ageGroup = this.item.ageGroup ?? '';
                next.playerCount = this.item.playerCount ?? '';
                next.notes = this.item.notes;
                next.readingStatus =
                    this.normalizeStatus(this.item.readingStatus) ?? next.readingStatus;
                next.readAt = this.normalizeDateInput(this.item.readAt) ?? next.readAt;
                next.seriesName = this.item.seriesName ?? '';
                next.volumeNumber = this.item.volumeNumber ?? null;
                next.totalVolumes = this.item.totalVolumes ?? null;
            }

            if (next.readingStatus !== BookStatus.Read) {
                next.readAt = null;
            }
            if (next.readingStatus !== BookStatus.Reading) {
                next.currentPage = null;
            }

            this.form.reset(next);
        }
    }

    submit(): void {
        if (this.form.invalid) {
            this.form.markAllAsTouched();
            return;
        }

        const value = this.form.value as ItemForm;

        // Validate cover image URL
        const coverImageError = this.validateCoverImageUrl(value.coverImage);
        if (coverImageError) {
            this.coverImageError = coverImageError;
            return;
        }

        if (value.itemType === 'book' && value.readingStatus === BookStatus.Read && !value.readAt) {
            this.form.get('readAt')?.setErrors({ required: true });
            this.form.get('readAt')?.markAsTouched();
            return;
        }

        if (value.itemType === 'book' && value.readingStatus === BookStatus.Reading) {
            const totalPages = this.parseInteger(value.pageCount);
            const currentPageValue = this.parseInteger(value.currentPage);
            if (!this.ensureCurrentPageWithinTotal(totalPages, currentPageValue)) {
                return;
            }
        } else {
            this.ensureCurrentPageWithinTotal(null, null);
        }

        // Validate volumeNumber <= totalVolumes for books
        if (value.itemType === 'book') {
            const volumeNumber = this.parseInteger(value.volumeNumber);
            const totalVolumes = this.parseInteger(value.totalVolumes);
            if (volumeNumber !== null && totalVolumes !== null && volumeNumber > totalVolumes) {
                this.form.get('volumeNumber')?.setErrors({ exceedsTotal: true });
                this.form.get('volumeNumber')?.markAsTouched();
                return;
            }
        }

        const readingStatus =
            value.itemType === 'book' ? (value.readingStatus ?? BookStatus.None) : undefined;
        const readAt = value.itemType === 'book' ? this.normalizeDateOutput(value.readAt) : null;
        const currentPage =
            value.itemType === 'book'
                ? value.readingStatus === BookStatus.Reading
                    ? this.parseInteger(value.currentPage)
                    : null
                : undefined;

        this.save.emit({
            ...value,
            releaseYear:
                value.releaseYear === null || value.releaseYear === undefined
                    ? null
                    : value.releaseYear,
            pageCount:
                value.pageCount === null || value.pageCount === undefined ? null : value.pageCount,
            currentPage,
            description: value.description ?? '',
            isbn13: value.isbn13 ?? '',
            isbn10: value.isbn10 ?? '',
            coverImage: value.coverImage ?? '',
            format: value.format ?? Formats.Unknown,
            genre: value.genre || undefined,
            rating: this.parseInteger(value.rating),
            retailPriceUsd: this.parseNumber(value.retailPriceUsd),
            googleVolumeId: value.googleVolumeId ?? '',
            platform: value.platform ?? '',
            ageGroup: value.ageGroup ?? '',
            playerCount: value.playerCount ?? '',
            readingStatus,
            readAt,
            seriesName: value.itemType === 'book' ? (value.seriesName ?? '') : undefined,
            volumeNumber:
                value.itemType === 'book' ? this.parseInteger(value.volumeNumber) : undefined,
            totalVolumes:
                value.itemType === 'book' ? this.parseInteger(value.totalVolumes) : undefined,
        });
    }

    clearReleaseYear(): void {
        this.form.patchValue({ releaseYear: null });
    }

    requestResync(): void {
        this.resyncRequested.emit();
    }

    onCoverErrorSet(error: string): void {
        this.coverImageError = error;
    }

    onCoverErrorCleared(): void {
        this.coverImageError = null;
    }

    private normalizeDateInput(value: string | Date | null | undefined): Date | null {
        if (!value) {
            return null;
        }
        if (value instanceof Date) {
            return Number.isNaN(value.getTime()) ? null : value;
        }

        const date = new Date(value);
        if (Number.isNaN(date.getTime())) {
            return null;
        }

        return date;
    }

    private handleItemTypeChange(type: ItemType): void {
        if (type !== 'book') {
            this.form.patchValue({
                readingStatus: BookStatus.None,
                readAt: null,
                currentPage: null,
                format: Formats.Unknown,
                genre: '',
                rating: null,
                retailPriceUsd: null,
                googleVolumeId: '',
                seriesName: '',
                volumeNumber: null,
                totalVolumes: null,
            });
            this.form.get('readAt')?.setErrors(null);
            this.form.get('currentPage')?.setErrors(null);
            this.form.get('volumeNumber')?.setErrors(null);
            this.form.get('totalVolumes')?.setErrors(null);
        }
    }

    private normalizeStatus(value: unknown): BookStatus | null {
        return this.isValidStatus(value) ? (value as BookStatus) : null;
    }

    private isValidStatus(value: unknown): value is BookStatus {
        return Object.values(BookStatus).includes(value as BookStatus);
    }

    private parseInteger(value: unknown): number | null {
        if (value === null || value === undefined || value === '') {
            return null;
        }
        if (typeof value === 'number') {
            return Number.isFinite(value) ? value : null;
        }
        const parsed = Number.parseInt(String(value), 10);
        return Number.isNaN(parsed) ? null : parsed;
    }

    private parseNumber(value: unknown): number | null {
        if (value === null || value === undefined || value === '') {
            return null;
        }
        if (typeof value === 'number') {
            return Number.isFinite(value) ? value : null;
        }
        const parsed = Number.parseFloat(String(value));
        return Number.isNaN(parsed) ? null : parsed;
    }

    private ensureCurrentPageWithinTotal(
        totalPages: number | null,
        currentPage: number | null,
    ): boolean {
        const control = this.form.get('currentPage');
        if (!control) {
            return true;
        }

        const errors = control.errors ?? {};
        if (totalPages !== null && currentPage !== null && currentPage > totalPages) {
            control.setErrors({ ...errors, maxPages: true });
            control.markAsTouched();
            return false;
        }

        if ('maxPages' in errors) {
            const { maxPages, ...rest } = errors as Record<string, unknown>;
            if (Object.keys(rest).length === 0) {
                control.setErrors(null);
            } else {
                control.setErrors(rest);
            }
        }

        return true;
    }

    private normalizeDateOutput(value: unknown): string | null {
        if (!value) {
            return null;
        }

        if (value instanceof Date) {
            return Number.isNaN(value.getTime()) ? null : value.toISOString();
        }

        const date = new Date(value as string);
        return Number.isNaN(date.getTime()) ? null : date.toISOString();
    }

    private validateCoverImageUrl(coverImage: string | null | undefined): string | null {
        if (!coverImage || coverImage.trim() === '') {
            return null;
        }

        const trimmed = coverImage.trim();

        // Allow data URIs - MIME type validated by handleCoverFileChange for uploads,
        // server-side validation handles manually entered data URIs
        if (trimmed.startsWith('data:')) {
            return null;
        }

        // Block dangerous schemes
        const lowerTrimmed = trimmed.toLowerCase();
        if (lowerTrimmed.startsWith('javascript:') || lowerTrimmed.startsWith('vbscript:')) {
            return 'Invalid image URL scheme.';
        }

        // Validate URL format and require HTTPS
        try {
            const url = new URL(trimmed);
            if (url.protocol !== 'https:') {
                return 'Cover image URL must use HTTPS.';
            }
        } catch {
            return 'Cover image must be a valid URL or data URI.';
        }

        return null;
    }
}
