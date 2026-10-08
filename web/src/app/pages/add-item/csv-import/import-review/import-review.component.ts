import { Component, computed, input, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatCheckboxModule } from '@angular/material/checkbox';
import { MatIconModule } from '@angular/material/icon';
import { MatRadioModule } from '@angular/material/radio';
import { RouterModule } from '@angular/router';

import { ITEM_TYPE_LABELS, ItemType } from '../../../../models/item-types';
import { CsvReviewedItem, CsvRowStatus } from '../../../../models/import';
import { ReviewRow, duplicateReason } from '../csv-import-review';
import { ReviewedField, reviewedItemFields } from './reviewed-item-fields';

export interface RowToggle {
    row: number;
    selected: boolean;
}

export interface RowChoice {
    row: number;
    /** Candidate index, or null to clear the choice. */
    index: number | null;
}

export type ReviewFilter = 'all' | CsvRowStatus;

const STATUS_LABELS: Record<CsvRowStatus, string> = {
    ready: 'Ready',
    duplicate: 'Possible duplicate',
    needs_match: 'Needs match',
};

/** One side of the duplicate comparison. */
interface ComparedItem {
    label: string;
    title: string;
    creator: string;
    itemType: string;
    releaseYear: number | null;
    isbn13: string;
    isbn10: string;
    itemId?: string;
}

/**
 * Lists previewed rows with their status, selection, and details. Expanding a
 * row only changes what is shown; choices and selections live in the parent,
 * so collapsing a row keeps them.
 */
@Component({
    selector: 'app-csv-import-review',
    standalone: true,
    imports: [MatButtonModule, MatCheckboxModule, MatIconModule, MatRadioModule, RouterModule],
    templateUrl: './import-review.component.html',
    styleUrl: './import-review.component.scss',
})
export class CsvImportReviewComponent {
    readonly rows = input.required<ReviewRow[]>();
    /** Disables every control while the import is saving. */
    readonly busy = input(false);

    readonly rowToggle = output<RowToggle>();
    readonly choose = output<RowChoice>();
    readonly selectAll = output<void>();
    readonly selectNone = output<void>();

    readonly filter = signal<ReviewFilter>('all');
    readonly expanded = signal<ReadonlySet<number>>(new Set());

    readonly filters: { value: ReviewFilter; label: string }[] = [
        { value: 'all', label: 'All' },
        { value: 'ready', label: 'Ready' },
        { value: 'duplicate', label: 'Possible duplicates' },
        { value: 'needs_match', label: 'Needs match' },
    ];

    readonly visibleRows = computed(() => {
        const filter = this.filter();
        const rows = this.rows();
        return filter === 'all' ? rows : rows.filter((row) => row.status === filter);
    });

    private readonly byNumber = computed(
        () => new Map(this.rows().map((row) => [row.source.row, row])),
    );

    readonly statusLabels = STATUS_LABELS;

    count(filter: ReviewFilter): number {
        const rows = this.rows();
        return filter === 'all' ? rows.length : rows.filter((row) => row.status === filter).length;
    }

    isExpanded(row: ReviewRow): boolean {
        return this.expanded().has(row.source.row);
    }

    toggleDetails(row: ReviewRow): void {
        this.expanded.update((current) => {
            const next = new Set(current);
            if (!next.delete(row.source.row)) {
                next.add(row.source.row);
            }
            return next;
        });
    }

    /**
     * Ready rows show what will be saved, duplicates can be compared, and
     * editions chosen; other problems are explained inline.
     */
    hasDetails(row: ReviewRow): boolean {
        return (
            row.status === 'duplicate' ||
            (row.status === 'ready' && !!row.item) ||
            !!row.source.candidates?.length
        );
    }

    /** The exact reviewed item fields that importing the row saves. */
    savedFields(item: CsvReviewedItem): ReviewedField[] {
        return reviewedItemFields(item);
    }

    detailsLabel(row: ReviewRow): string {
        if (this.isExpanded(row)) {
            return 'Hide details';
        }
        if (row.source.candidates?.length) {
            return row.choice === null ? 'Choose edition' : 'Change edition';
        }
        return row.status === 'duplicate' ? 'Compare' : 'Details';
    }

    title(row: ReviewRow): string {
        return row.item?.title || row.source.title || 'Untitled row';
    }

    identifier(row: ReviewRow): string {
        return row.item?.isbn13 || row.item?.isbn10 || row.source.identifier;
    }

    typeLabel(itemType: string): string {
        return ITEM_TYPE_LABELS[itemType as ItemType] ?? (itemType || 'Unknown type');
    }

    /** The one-line explanation shown under the row title. */
    summary(row: ReviewRow): string {
        switch (row.status) {
            case 'duplicate':
                return `${duplicateReason(row.libraryMatches, row.fileMatch)}. It will be skipped.`;
            case 'needs_match': {
                const count = row.source.candidates?.length ?? 0;
                if (count > 0) {
                    return `Choose from ${count} catalog edition${count === 1 ? '' : 's'} to import this row.`;
                }
                return `${sentence(row.source.problem ?? 'This row cannot be imported')} Fix it in your CSV and preview again.`;
            }
            default:
                return row.choice === null ? '' : `Using catalog edition “${row.item?.title}”.`;
        }
    }

    /** Incoming row and the items it duplicates, for side-by-side comparison. */
    comparison(row: ReviewRow): ComparedItem[] {
        if (!row.item) {
            return [];
        }
        const compared: ComparedItem[] = [describe('This row', row.item)];
        for (const match of row.libraryMatches) {
            if (compared.some((entry) => entry.itemId === match.itemId)) {
                continue;
            }
            compared.push({
                label: 'In your library',
                title: match.title,
                creator: match.creator,
                itemType: match.itemType,
                releaseYear: match.releaseYear ?? null,
                isbn13: match.isbn13,
                isbn10: match.isbn10,
                itemId: match.itemId,
            });
        }
        const earlier = row.fileMatch ? this.byNumber().get(row.fileMatch.row)?.item : null;
        if (row.fileMatch && earlier) {
            compared.push(describe(`Row ${row.fileMatch.row} of this file`, earlier));
        }
        return compared.map((entry) => ({ ...entry, itemType: this.typeLabel(entry.itemType) }));
    }

    candidateNote(row: ReviewRow, index: number): string {
        const candidate = row.source.candidates?.[index];
        if (!candidate) {
            return '';
        }
        const notes: string[] = [];
        if (candidate.libraryMatches.length > 0) {
            notes.push(
                `${duplicateReason(candidate.libraryMatches, null)}, so it would be skipped`,
            );
        }
        if (candidate.csvOverrides.length > 0) {
            notes.push(`keeps your CSV values for ${candidate.csvOverrides.join(', ')}`);
        }
        return notes.join('; ');
    }

    handleRadioChange(row: ReviewRow, value: number): void {
        this.choose.emit({ row: row.source.row, index: value });
    }
}

function sentence(text: string): string {
    const trimmed = text.trim();
    const capitalized = trimmed.charAt(0).toUpperCase() + trimmed.slice(1);
    return /[.!?]$/.test(capitalized) ? capitalized : `${capitalized}.`;
}

function describe(label: string, item: CsvReviewedItem): ComparedItem {
    return {
        label,
        title: item.title,
        creator: item.creator,
        itemType: item.itemType,
        releaseYear: item.releaseYear,
        isbn13: item.isbn13,
        isbn10: item.isbn10,
    };
}
