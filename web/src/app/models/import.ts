import { BookStatus, Format, Genre } from './book';
import { ItemType } from './item-types';

export interface CsvImportSummary {
    totalRows: number;
    imported: number;
    skippedDuplicates: CsvImportDuplicate[];
    failed: CsvImportFailure[];
}

export interface CsvImportDuplicate {
    row: number;
    title?: string;
    identifier?: string;
    reason: string;
}

export interface CsvImportFailure {
    row: number;
    title?: string;
    identifier?: string;
    error: string;
}

/** Preview status of a CSV row; mirrors internal/importer.RowStatus. */
export type CsvRowStatus = 'ready' | 'duplicate' | 'needs_match';

/** The exact-match field that made a row a duplicate. */
export type CsvDuplicateField = 'title' | 'isbn13' | 'isbn10';

/**
 * The normalized item a reviewed row creates (internal/importer.ReviewedItem).
 * The browser sends it back unchanged when committing.
 */
export interface CsvReviewedItem {
    title: string;
    creator: string;
    itemType: ItemType;
    releaseYear: number | null;
    pageCount: number | null;
    currentPage: number | null;
    isbn13: string;
    isbn10: string;
    description: string;
    coverImage: string;
    format: Format | '';
    genre: Genre | '';
    rating: number | null;
    retailPriceUsd: number | null;
    googleVolumeId: string;
    platform: string;
    ageGroup: string;
    playerCount: string;
    readingStatus: BookStatus;
    readAt: string | null;
    notes: string;
    seriesName: string;
    volumeNumber: number | null;
    totalVolumes: number | null;
    createdAt?: string;
    updatedAt?: string;
}

/** An existing library item a row duplicates. */
export interface CsvLibraryMatch {
    field: CsvDuplicateField;
    itemId: string;
    title: string;
    creator: string;
    itemType: ItemType;
    releaseYear?: number;
    isbn13: string;
    isbn10: string;
}

/** An earlier row of the same file a row duplicates. */
export interface CsvFileMatch {
    field: CsvDuplicateField;
    row: number;
}

/** A catalog edition a titleless book row can be imported as. */
export interface CsvCandidate {
    item: CsvReviewedItem;
    keys: string[];
    libraryMatches: CsvLibraryMatch[];
    /** Fields where the CSV value was kept over a different catalog value. */
    csvOverrides: string[];
}

export interface CsvPreviewRow {
    /** Row number in the file; the header is row 1. */
    row: number;
    status: CsvRowStatus;
    itemType: string;
    title: string;
    identifier: string;
    item?: CsvReviewedItem;
    keys?: string[];
    libraryMatches?: CsvLibraryMatch[];
    fileMatch?: CsvFileMatch;
    problem?: string;
    candidates?: CsvCandidate[];
}

/** Read-only review of an uploaded CSV (internal/importer.Preview). */
export interface CsvImportPreview {
    totalRows: number;
    ready: number;
    duplicates: number;
    needsMatch: number;
    rows: CsvPreviewRow[];
}

export interface CsvCommitRow {
    row: number;
    item: CsvReviewedItem;
}

export type CsvCommitStatus = 'added' | 'skipped' | 'failed' | 'interrupted' | 'unprocessed';

export interface CsvCommitRowOutcome {
    row: number;
    status: CsvCommitStatus;
    title: string;
    identifier: string;
    itemId?: string;
    message?: string;
    libraryMatches?: CsvLibraryMatch[];
    fileMatch?: CsvFileMatch;
}

/** Result of committing reviewed rows (internal/importer.CommitResult). */
export interface CsvCommitResult {
    added: number;
    skipped: number;
    failed: number;
    interrupted: number;
    unprocessed: number;
    rows: CsvCommitRowOutcome[];
}
