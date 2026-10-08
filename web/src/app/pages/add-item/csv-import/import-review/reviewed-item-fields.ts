import {
    BOOK_STATUS_LABELS,
    FORMAT_LABELS,
    Format,
    GENRE_LABELS,
    Genre,
} from '../../../../models/book';
import { CsvReviewedItem } from '../../../../models/import';
import { ITEM_TYPE_LABELS } from '../../../../models/item-types';

/** One labelled value of a reviewed item; a null value means it is not set. */
export interface ReviewedField {
    label: string;
    value: string | null;
    /** Long values span the full width of the details grid. */
    wide?: boolean;
}

const dateFormat = new Intl.DateTimeFormat('en-US', { dateStyle: 'medium', timeZone: 'UTC' });
const priceFormat = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' });

/**
 * Describes exactly what importing a reviewed item saves, using readable labels.
 * Only the fields that apply to the item's type are listed: the server clears
 * the others before saving.
 */
export function reviewedItemFields(item: CsvReviewedItem): ReviewedField[] {
    const fields: ReviewedField[] = [
        { label: 'Title', value: text(item.title) },
        { label: 'Creator', value: text(item.creator) },
        { label: 'Type', value: ITEM_TYPE_LABELS[item.itemType] ?? item.itemType },
        { label: 'Release year', value: number(item.releaseYear) },
    ];

    if (item.itemType === 'book') {
        fields.push(
            { label: 'ISBN-13', value: text(item.isbn13) },
            { label: 'ISBN-10', value: text(item.isbn10) },
            { label: 'Pages', value: number(item.pageCount) },
            {
                label: 'Format',
                value: item.format ? (FORMAT_LABELS[item.format as Format] ?? item.format) : null,
            },
            {
                label: 'Genre',
                value: item.genre ? (GENRE_LABELS[item.genre as Genre] ?? item.genre) : null,
            },
            { label: 'Rating', value: item.rating === null ? null : `${item.rating} / 10` },
            {
                label: 'Retail price',
                value:
                    item.retailPriceUsd === null ? null : priceFormat.format(item.retailPriceUsd),
            },
            { label: 'Google Books ID', value: text(item.googleVolumeId) },
            {
                label: 'Reading status',
                value: BOOK_STATUS_LABELS[item.readingStatus] ?? item.readingStatus,
            },
        );
        if (item.readingStatus === 'read') {
            fields.push({ label: 'Read on', value: date(item.readAt) });
        }
        if (item.readingStatus === 'reading') {
            fields.push({ label: 'Current page', value: number(item.currentPage) });
        }
        fields.push({ label: 'Series', value: series(item) });
    }

    if (item.itemType === 'game') {
        fields.push(
            { label: 'Platform', value: text(item.platform) },
            { label: 'Age group', value: text(item.ageGroup) },
            { label: 'Player count', value: text(item.playerCount) },
        );
    }

    fields.push(
        { label: 'Added on', value: date(item.createdAt) ?? 'When imported' },
        { label: 'Last updated', value: date(item.updatedAt) ?? 'Same as added on' },
        { label: 'Description', value: text(item.description), wide: true },
        { label: 'Cover image', value: cover(item.coverImage), wide: true },
        { label: 'Notes', value: text(item.notes), wide: true },
    );
    return fields;
}

function text(value: string | null | undefined): string | null {
    return value?.trim() ? value : null;
}

function number(value: number | null | undefined): string | null {
    return value === null || value === undefined ? null : String(value);
}

function date(value: string | null | undefined): string | null {
    if (!value) {
        return null;
    }
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? value : dateFormat.format(parsed);
}

function cover(value: string): string | null {
    if (!value.trim()) {
        return null;
    }
    return value.startsWith('data:') ? 'Embedded image' : value;
}

function series(item: CsvReviewedItem): string | null {
    const parts: string[] = [];
    if (item.seriesName.trim()) {
        parts.push(item.seriesName);
    }
    if (item.volumeNumber !== null) {
        parts.push(
            item.totalVolumes === null
                ? `Volume ${item.volumeNumber}`
                : `Volume ${item.volumeNumber} of ${item.totalVolumes}`,
        );
    } else if (item.totalVolumes !== null) {
        parts.push(`${item.totalVolumes} volumes`);
    }
    return parts.length ? parts.join(' · ') : null;
}
