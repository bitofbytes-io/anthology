import { ItemForm } from '../../../models';
import { SearchCategoryConfig } from './search-categories';

/** Normalizes a partial lookup result into a complete item draft for the given category. */
export function composeLookupDraft(
    partial: Partial<ItemForm>,
    category: SearchCategoryConfig,
): ItemForm {
    const releaseYear = partial.releaseYear;
    let normalizedReleaseYear: number | null = null;
    const pageCount = partial.pageCount;
    let normalizedPageCount: number | null = null;
    const retailPriceUsd = partial.retailPriceUsd;
    let normalizedRetailPriceUsd: number | null = null;

    if (typeof releaseYear === 'number') {
        normalizedReleaseYear = releaseYear;
    } else if (typeof releaseYear === 'string') {
        const parsed = Number.parseInt(releaseYear, 10);
        normalizedReleaseYear = Number.isNaN(parsed) ? null : parsed;
    }

    if (typeof pageCount === 'number') {
        normalizedPageCount = pageCount;
    } else if (typeof pageCount === 'string') {
        const parsed = Number.parseInt(pageCount, 10);
        normalizedPageCount = Number.isNaN(parsed) ? null : parsed;
    }

    if (typeof retailPriceUsd === 'number') {
        normalizedRetailPriceUsd = retailPriceUsd;
    } else if (typeof retailPriceUsd === 'string') {
        const parsed = Number.parseFloat(retailPriceUsd);
        normalizedRetailPriceUsd = Number.isNaN(parsed) ? null : parsed;
    }

    return {
        title: partial.title ?? '',
        creator: partial.creator ?? '',
        itemType: category.itemType,
        releaseYear: normalizedReleaseYear,
        pageCount: normalizedPageCount,
        isbn13: partial.isbn13 ?? '',
        isbn10: partial.isbn10 ?? '',
        description: partial.description ?? '',
        coverImage: partial.coverImage ?? '',
        genre: partial.genre,
        retailPriceUsd: normalizedRetailPriceUsd,
        googleVolumeId: partial.googleVolumeId ?? '',
        platform: partial.platform ?? '',
        ageGroup: partial.ageGroup ?? '',
        playerCount: partial.playerCount ?? '',
        notes: partial.notes ?? '',
        seriesName: partial.seriesName ?? '',
        volumeNumber: partial.volumeNumber ?? null,
        totalVolumes: partial.totalVolumes ?? null,
    };
}
