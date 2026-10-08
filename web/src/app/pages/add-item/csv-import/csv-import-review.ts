import {
    CsvCommitResult,
    CsvCommitRow,
    CsvCommitStatus,
    CsvDuplicateField,
    CsvFileMatch,
    CsvLibraryMatch,
    CsvPreviewRow,
    CsvReviewedItem,
    CsvRowStatus,
} from '../../../models/import';

/** A previewed row with the user's choices applied. */
export interface ReviewRow {
    source: CsvPreviewRow;
    status: CsvRowStatus;
    /** The item this row would create: the row's own, or the chosen catalog edition. */
    item: CsvReviewedItem | null;
    libraryMatches: CsvLibraryMatch[];
    fileMatch: CsvFileMatch | null;
    /** Index of the chosen catalog edition, if any. */
    choice: number | null;
    selected: boolean;
}

/**
 * Applies catalog choices and deselections to previewed rows. It repeats the
 * server's duplicate rule in file order: a row is a duplicate when its item
 * matches a library item, or shares an exact key with an earlier row that is
 * ready. Choosing an edition can therefore turn a later row into a duplicate,
 * or make the chosen row itself one. Ready rows are selected unless the user
 * deselected them; other rows can never be selected.
 */
export function reviewRows(
    rows: readonly CsvPreviewRow[],
    choices: ReadonlyMap<number, number>,
    deselected: ReadonlySet<number>,
): ReviewRow[] {
    const claimed = new Map<string, number>();
    return rows.map((source) => {
        const choice = source.item ? null : (choices.get(source.row) ?? null);
        const candidate = choice === null ? undefined : source.candidates?.[choice];
        const item = source.item ?? candidate?.item ?? null;
        const keys = source.item ? (source.keys ?? []) : (candidate?.keys ?? []);
        const libraryMatches = source.item
            ? (source.libraryMatches ?? [])
            : (candidate?.libraryMatches ?? []);

        const review: ReviewRow = {
            source,
            status: 'needs_match',
            item,
            libraryMatches,
            fileMatch: null,
            choice: candidate ? choice : null,
            selected: false,
        };
        if (!item) {
            return review;
        }
        if (libraryMatches.length > 0) {
            review.status = 'duplicate';
            return review;
        }
        const claimedKey = keys.find((key) => claimed.has(key));
        if (claimedKey) {
            review.status = 'duplicate';
            review.fileMatch = {
                field: keyField(claimedKey),
                row: claimed.get(claimedKey) as number,
            };
            return review;
        }
        for (const key of keys) {
            claimed.set(key, source.row);
        }
        review.status = 'ready';
        review.selected = !deselected.has(source.row);
        return review;
    });
}

function keyField(key: string): CsvDuplicateField {
    return key.slice(0, key.indexOf(':')) as CsvDuplicateField;
}

export interface ReviewCounts {
    ready: number;
    duplicate: number;
    needsMatch: number;
    selected: number;
}

export function countReview(rows: readonly ReviewRow[]): ReviewCounts {
    const counts: ReviewCounts = { ready: 0, duplicate: 0, needsMatch: 0, selected: 0 };
    for (const row of rows) {
        if (row.status === 'ready') {
            counts.ready++;
        } else if (row.status === 'duplicate') {
            counts.duplicate++;
        } else {
            counts.needsMatch++;
        }
        if (row.selected) {
            counts.selected++;
        }
    }
    return counts;
}

/** The commit request rows: every selected row with its exact reviewed item. */
export function commitRows(rows: readonly ReviewRow[]): CsvCommitRow[] {
    return rows
        .filter((row) => row.selected && row.item)
        .map((row) => ({ row: row.source.row, item: row.item as CsvReviewedItem }));
}

const FIELD_LABELS: Record<CsvDuplicateField, string> = {
    title: 'title',
    isbn13: 'ISBN-13',
    isbn10: 'ISBN-10',
};

/** Explains a duplicate the way the importer decided it. */
export function duplicateReason(
    libraryMatches: readonly CsvLibraryMatch[],
    fileMatch: CsvFileMatch | null | undefined,
): string {
    if (libraryMatches.length > 0) {
        const fields = [...new Set(libraryMatches.map((match) => FIELD_LABELS[match.field]))];
        return `Same ${fields.join(' and ')} as an item already in your library`;
    }
    if (fileMatch) {
        return `Same ${FIELD_LABELS[fileMatch.field]} as row ${fileMatch.row} of this file`;
    }
    return 'Duplicate';
}

/** Final status of a row; `unknown` only appears if the server omitted a submitted row. */
export type OutcomeStatus = CsvCommitStatus | 'unknown';

export interface OutcomeRow {
    row: number;
    status: OutcomeStatus;
    title: string;
    identifier: string;
    message: string;
    itemId?: string;
    libraryMatches: CsvLibraryMatch[];
    /** Whether the row was sent to the server for import. */
    sent: boolean;
}

export interface ImportOutcome {
    rows: OutcomeRow[];
    counts: Record<OutcomeStatus, number>;
}

/**
 * Combines the commit result with the rows that were never sent, so every
 * previewed row has exactly one outcome: duplicates are skipped, and rows
 * without a chosen match or left unselected are unprocessed.
 */
export function composeOutcome(rows: readonly ReviewRow[], result: CsvCommitResult): ImportOutcome {
    const committed = new Map(result.rows.map((outcome) => [outcome.row, outcome]));
    const counts: Record<OutcomeStatus, number> = {
        added: 0,
        skipped: 0,
        failed: 0,
        interrupted: 0,
        unprocessed: 0,
        unknown: 0,
    };
    const outcomes = rows.map((row): OutcomeRow => {
        const title = row.item?.title || row.source.title;
        const identifier = row.item?.isbn13 || row.item?.isbn10 || row.source.identifier;
        let outcome: OutcomeRow;
        if (row.selected) {
            const reported = committed.get(row.source.row);
            outcome = reported
                ? {
                      row: reported.row,
                      status: reported.status,
                      title: reported.title || title,
                      identifier: reported.identifier || identifier,
                      message:
                          reported.status === 'skipped'
                              ? `${duplicateReason(reported.libraryMatches ?? [], reported.fileMatch)} when the import ran.`
                              : (reported.message ?? ''),
                      itemId: reported.itemId,
                      libraryMatches: reported.libraryMatches ?? [],
                      sent: true,
                  }
                : {
                      row: row.source.row,
                      status: 'unknown',
                      title,
                      identifier,
                      message: 'The server did not report a result for this row.',
                      libraryMatches: [],
                      sent: true,
                  };
        } else {
            outcome = {
                row: row.source.row,
                status: row.status === 'duplicate' ? 'skipped' : 'unprocessed',
                title,
                identifier,
                message: unsentMessage(row),
                libraryMatches: row.libraryMatches,
                sent: false,
            };
        }
        counts[outcome.status]++;
        return outcome;
    });
    return { rows: outcomes, counts };
}

function unsentMessage(row: ReviewRow): string {
    switch (row.status) {
        case 'duplicate':
            return duplicateReason(row.libraryMatches, row.fileMatch);
        case 'ready':
            return 'Not selected for import.';
        default:
            return row.source.candidates?.length
                ? 'No catalog edition was chosen.'
                : (row.source.problem ?? 'This row needs a match.');
    }
}
