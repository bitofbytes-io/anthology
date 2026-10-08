import {
    CsvCandidate,
    CsvImportPreview,
    CsvLibraryMatch,
    CsvPreviewRow,
    CsvReviewedItem,
} from '../../../models/import';

// Invented sample data for CSV import specs.

export function reviewedItem(overrides: Partial<CsvReviewedItem> = {}): CsvReviewedItem {
    return {
        title: 'Untitled',
        creator: '',
        itemType: 'book',
        releaseYear: null,
        pageCount: null,
        currentPage: null,
        isbn13: '',
        isbn10: '',
        description: '',
        coverImage: '',
        format: 'UNKNOWN',
        genre: '',
        rating: null,
        retailPriceUsd: null,
        googleVolumeId: '',
        platform: '',
        ageGroup: '',
        playerCount: '',
        readingStatus: 'none',
        readAt: null,
        notes: '',
        seriesName: '',
        volumeNumber: null,
        totalVolumes: null,
        ...overrides,
    };
}

function keysFor(item: CsvReviewedItem): string[] {
    const keys = [`title:${item.title.trim().toLowerCase()}`];
    if (item.isbn13) {
        keys.push(`isbn13:${item.isbn13.replace(/\D/g, '')}`);
    }
    return keys;
}

export function libraryMatch(overrides: Partial<CsvLibraryMatch> = {}): CsvLibraryMatch {
    return {
        field: 'title',
        itemId: 'existing-1',
        title: 'Salt and Signal',
        creator: 'R. Okafor',
        itemType: 'book',
        releaseYear: 2015,
        isbn13: '9784444444444',
        isbn10: '',
        ...overrides,
    };
}

export function rowWithItem(
    row: number,
    status: 'ready' | 'duplicate',
    item: Partial<CsvReviewedItem>,
    extras: Partial<CsvPreviewRow> = {},
): CsvPreviewRow {
    const full = reviewedItem(item);
    return {
        row,
        status,
        itemType: full.itemType,
        title: full.title,
        identifier: full.isbn13,
        item: full,
        keys: keysFor(full),
        ...extras,
    };
}

export function candidate(
    item: Partial<CsvReviewedItem>,
    extras: Partial<CsvCandidate> = {},
): CsvCandidate {
    const full = reviewedItem(item);
    return { item: full, keys: keysFor(full), libraryMatches: [], csvOverrides: [], ...extras };
}

/**
 * Ten rows like the approved mockup: 6 ready, 2 possible duplicates, and 2
 * that need a match. Row 6 offers three editions: choosing the first makes 7
 * ready rows; the second has the same title as row 10, so row 10 becomes a
 * duplicate; the third is already in the library.
 */
export function demoPreview(): CsvImportPreview {
    const rows: CsvPreviewRow[] = [
        rowWithItem(2, 'ready', {
            title: 'The Lantern Archive',
            creator: 'M. Ellery',
            isbn13: '9781111111111',
        }),
        rowWithItem(3, 'ready', { title: 'Orbit of Glass', creator: 'T. Varga' }),
        rowWithItem(
            4,
            'duplicate',
            { title: 'Salt and Signal', creator: 'R. Okafor' },
            { libraryMatches: [libraryMatch()] },
        ),
        rowWithItem(5, 'ready', { title: 'Quiet Harbor', itemType: 'game', platform: 'Switch' }),
        {
            row: 6,
            status: 'needs_match',
            itemType: 'book',
            title: '',
            identifier: '9782222222222',
            problem: 'Choose the catalog edition to import for this ISBN.',
            candidates: [
                candidate(
                    {
                        title: 'Paper Moons',
                        creator: 'CSV Author',
                        isbn13: '9782222222222',
                        googleVolumeId: 'vol-first',
                        notes: 'Signed copy',
                    },
                    { csvOverrides: ['creator'] },
                ),
                candidate({
                    title: 'Copper Garden',
                    isbn13: '9782222222222',
                    googleVolumeId: 'vol-second',
                }),
                candidate(
                    { title: 'Salt and Signal', isbn13: '9782222222222' },
                    { libraryMatches: [libraryMatch()] },
                ),
            ],
        },
        rowWithItem(7, 'ready', { title: 'Field Notes on Fog' }),
        rowWithItem(
            8,
            'duplicate',
            { title: 'orbit of glass', itemType: 'movie' },
            { fileMatch: { field: 'title', row: 3 } },
        ),
        {
            row: 9,
            status: 'needs_match',
            itemType: 'book',
            title: '',
            identifier: '9783333333333',
            problem: 'no metadata found for 9783333333333',
        },
        rowWithItem(10, 'ready', { title: 'Copper Garden', creator: 'L. Brandt' }),
        rowWithItem(11, 'ready', { title: 'Night Ferry', itemType: 'movie' }),
    ];
    return { totalRows: rows.length, ready: 6, duplicates: 2, needsMatch: 2, rows };
}
