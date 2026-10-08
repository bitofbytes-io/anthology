import {
    commitRows,
    composeOutcome,
    countReview,
    duplicateReason,
    reviewRows,
} from './csv-import-review';
import { CsvCommitResult, CsvCommitRowOutcome } from '../../../models/import';
import { demoPreview, libraryMatch, rowWithItem } from './csv-import.fixtures';

const none = new Map<number, number>();
const noneDeselected = new Set<number>();

function statusOf(rows: ReturnType<typeof reviewRows>, row: number) {
    return rows.find((entry) => entry.source.row === row);
}

function added(rows: { row: number; item: { title: string } }[]): CsvCommitResult {
    return {
        added: rows.length,
        skipped: 0,
        failed: 0,
        interrupted: 0,
        unprocessed: 0,
        rows: rows.map((entry) => ({
            row: entry.row,
            status: 'added',
            title: entry.item.title,
            identifier: '',
            itemId: `new-${entry.row}`,
        })),
    };
}

describe('csv import review', () => {
    it('reproduces the server classification and selects only ready rows', () => {
        const preview = demoPreview();
        const rows = reviewRows(preview.rows, none, noneDeselected);

        expect(rows.map((row) => row.status)).toEqual(preview.rows.map((row) => row.status));
        expect(countReview(rows)).toEqual({ ready: 6, duplicate: 2, needsMatch: 2, selected: 6 });
        expect(rows.filter((row) => row.selected).map((row) => row.source.row)).toEqual([
            2, 3, 5, 7, 10, 11,
        ]);
    });

    it('makes a row ready and selected once an edition is chosen', () => {
        const rows = reviewRows(demoPreview().rows, new Map([[6, 0]]), noneDeselected);
        const chosen = statusOf(rows, 6);

        expect(chosen?.status).toBe('ready');
        expect(chosen?.selected).toBe(true);
        expect(chosen?.item?.googleVolumeId).toBe('vol-first');
        expect(countReview(rows)).toEqual({ ready: 7, duplicate: 2, needsMatch: 1, selected: 7 });
    });

    it('turns a later row into a duplicate when a chosen edition claims its title', () => {
        const rows = reviewRows(demoPreview().rows, new Map([[6, 1]]), noneDeselected);

        expect(statusOf(rows, 6)?.status).toBe('ready');
        const later = statusOf(rows, 10);
        expect(later?.status).toBe('duplicate');
        expect(later?.selected).toBe(false);
        expect(later?.fileMatch).toEqual({ field: 'title', row: 6 });
        expect(countReview(rows)).toEqual({ ready: 6, duplicate: 3, needsMatch: 1, selected: 6 });
        expect(commitRows(rows).map((row) => row.row)).not.toContain(10);
    });

    it('skips a chosen edition that is already in the library', () => {
        const rows = reviewRows(demoPreview().rows, new Map([[6, 2]]), noneDeselected);
        const chosen = statusOf(rows, 6);

        expect(chosen?.status).toBe('duplicate');
        expect(chosen?.selected).toBe(false);
        expect(chosen?.libraryMatches[0].itemId).toBe('existing-1');
    });

    it('keeps in-file duplicates skipped when the earlier row is deselected', () => {
        const rows = reviewRows(demoPreview().rows, none, new Set([3]));

        expect(statusOf(rows, 3)?.selected).toBe(false);
        expect(statusOf(rows, 8)?.status).toBe('duplicate');
        expect(statusOf(rows, 8)?.selected).toBe(false);
    });

    it('commits exactly the selected rows with their reviewed items', () => {
        const preview = demoPreview();
        const rows = reviewRows(preview.rows, new Map([[6, 0]]), new Set([11]));
        const request = commitRows(rows);

        expect(request.map((row) => row.row)).toEqual([2, 3, 5, 6, 7, 10]);
        expect(request[3].item).toEqual(preview.rows[4].candidates?.[0].item);
        expect(request[0].item).toEqual(preview.rows[0].item);
    });

    it('accounts for every row in the final outcome', () => {
        const rows = reviewRows(demoPreview().rows, new Map([[6, 0]]), noneDeselected);
        const outcome = composeOutcome(rows, added(commitRows(rows)));

        expect(outcome.counts).toEqual({
            added: 7,
            skipped: 2,
            failed: 0,
            interrupted: 0,
            unprocessed: 1,
            unknown: 0,
        });
        expect(outcome.rows).toHaveLength(10);
        const unresolved = outcome.rows.find((row) => row.row === 9);
        expect(unresolved?.status).toBe('unprocessed');
        expect(unresolved?.message).toBe('no metadata found for 9783333333333');
        expect(outcome.rows.find((row) => row.row === 4)?.message).toBe(
            'Same title as an item already in your library',
        );
        expect(outcome.rows.find((row) => row.row === 8)?.message).toBe(
            'Same title as row 3 of this file',
        );
    });

    it('reports deselected rows and unchosen editions as not imported', () => {
        const rows = reviewRows(demoPreview().rows, none, new Set([11]));
        const outcome = composeOutcome(rows, added(commitRows(rows)));

        expect(outcome.counts.added).toBe(5);
        expect(outcome.counts.unprocessed).toBe(3);
        expect(outcome.rows.find((row) => row.row === 11)?.message).toBe(
            'Not selected for import.',
        );
        expect(outcome.rows.find((row) => row.row === 6)?.message).toBe(
            'No catalog edition was chosen.',
        );
    });

    it('keeps partial results distinct and counts every row', () => {
        const rows = reviewRows(demoPreview().rows, none, noneDeselected);
        const sent = commitRows(rows);
        const statuses: CsvCommitRowOutcome['status'][] = [
            'added',
            'failed',
            'skipped',
            'added',
            'interrupted',
            'unprocessed',
        ];
        const result: CsvCommitResult = {
            added: 2,
            skipped: 1,
            failed: 1,
            interrupted: 1,
            unprocessed: 1,
            rows: sent.map((row, index) => ({
                row: row.row,
                status: statuses[index],
                title: row.item.title,
                identifier: '',
                message: statuses[index] === 'failed' ? 'This row could not be saved.' : undefined,
                libraryMatches: statuses[index] === 'skipped' ? [libraryMatch()] : undefined,
            })),
        };
        const outcome = composeOutcome(rows, result);

        expect(outcome.counts).toEqual({
            added: 2,
            skipped: 3,
            failed: 1,
            interrupted: 1,
            unprocessed: 3,
            unknown: 0,
        });
        expect(Object.values(outcome.counts).reduce((sum, count) => sum + count, 0)).toBe(10);
        expect(outcome.rows.filter((row) => row.sent && row.status === 'unprocessed')).toHaveLength(
            1,
        );
    });

    it('marks a sent row without a server result as unknown', () => {
        const rows = reviewRows(demoPreview().rows, none, noneDeselected);
        const result = added(commitRows(rows).slice(1));
        const outcome = composeOutcome(rows, result);

        expect(outcome.counts.unknown).toBe(1);
        expect(outcome.rows[0].status).toBe('unknown');
    });

    it('counts large imports exactly', () => {
        const preview = Array.from({ length: 1000 }, (_, index) =>
            rowWithItem(index + 2, 'ready', { title: `Volume ${index}` }),
        );
        const rows = reviewRows(preview, none, noneDeselected);
        const outcome = composeOutcome(rows, added(commitRows(rows)));

        expect(outcome.counts.added).toBe(1000);
        expect(outcome.rows).toHaveLength(1000);
    });

    it('explains duplicate reasons', () => {
        expect(duplicateReason([libraryMatch(), libraryMatch({ field: 'isbn13' })], null)).toBe(
            'Same title and ISBN-13 as an item already in your library',
        );
        expect(duplicateReason([], { field: 'isbn10', row: 4 })).toBe(
            'Same ISBN-10 as row 4 of this file',
        );
    });
});
