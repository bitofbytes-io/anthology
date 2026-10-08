import { inject, Injectable } from '@angular/core';
import { HttpClient, HttpParams } from '@angular/common/http';
import { map, Observable } from 'rxjs';

import { environment } from '../config/environment';
import {
    BookStatus,
    DuplicateCheckInput,
    DuplicateMatch,
    Item,
    ItemForm,
    ItemType,
    LetterHistogram,
    ShelfStatusFilter,
    ShelfStatusFilters,
} from '../models';
import {
    CsvCommitResult,
    CsvCommitRow,
    CsvImportPreview,
    CsvImportSummary,
} from '../models/import';

@Injectable({ providedIn: 'root' })
export class ItemService {
    private readonly http = inject(HttpClient);
    private readonly baseUrl = `${environment.apiUrl}/items`;

    list(filters?: {
        itemType?: ItemType;
        letter?: string;
        status?: BookStatus;
        shelfStatus?: ShelfStatusFilter;
        query?: string;
        limit?: number;
    }): Observable<Item[]> {
        let params = new HttpParams();
        if (filters?.itemType) {
            params = params.set('type', filters.itemType);
        }
        if (filters?.letter) {
            params = params.set('letter', filters.letter);
        }
        if (filters?.status) {
            params = params.set('status', filters.status);
        }
        if (filters?.shelfStatus && filters.shelfStatus !== ShelfStatusFilters.All) {
            params = params.set('shelf_status', filters.shelfStatus);
        }
        if (filters?.query) {
            params = params.set('query', filters.query);
        }
        if (filters?.limit && filters.limit > 0) {
            params = params.set('limit', filters.limit.toString());
        }

        return this.http
            .get<{
                items: Item[];
            }>(this.baseUrl, { params: params.keys().length ? params : undefined })
            .pipe(map((response) => response.items));
    }

    get(id: string): Observable<Item> {
        return this.http.get<Item>(`${this.baseUrl}/${id}`);
    }

    create(form: ItemForm): Observable<Item> {
        const payload = this.normalizeForm(form);
        return this.http.post<Item>(this.baseUrl, payload);
    }

    update(id: string, form: Partial<ItemForm>): Observable<Item> {
        const payload = this.normalizeForm(form);
        return this.http.put<Item>(`${this.baseUrl}/${id}`, payload);
    }

    delete(id: string): Observable<void> {
        return this.http.delete<void>(`${this.baseUrl}/${id}`);
    }

    resync(id: string): Observable<Item> {
        return this.http.post<Item>(`${this.baseUrl}/${id}/resync`, {});
    }

    importCsv(file: File): Observable<CsvImportSummary> {
        const formData = new FormData();
        formData.append('file', file);
        return this.http.post<CsvImportSummary>(`${this.baseUrl}/import`, formData);
    }

    /** Reviews a CSV without saving anything. */
    previewCsvImport(file: File): Observable<CsvImportPreview> {
        const formData = new FormData();
        formData.append('file', file);
        return this.http.post<CsvImportPreview>(`${this.baseUrl}/import/preview`, formData);
    }

    /** Saves reviewed rows exactly as previewed; the server does no catalog lookups. */
    commitCsvImport(rows: CsvCommitRow[]): Observable<CsvCommitResult> {
        return this.http.post<CsvCommitResult>(`${this.baseUrl}/import/commit`, { rows });
    }

    exportCsv(filters?: {
        itemType?: ItemType;
        status?: BookStatus;
        shelfStatus?: ShelfStatusFilter;
    }): Observable<Blob> {
        let params = new HttpParams();
        if (filters?.itemType) {
            params = params.set('type', filters.itemType);
        }
        if (filters?.status) {
            params = params.set('status', filters.status);
        }
        if (filters?.shelfStatus && filters.shelfStatus !== ShelfStatusFilters.All) {
            params = params.set('shelf_status', filters.shelfStatus);
        }

        return this.http.get(`${this.baseUrl}/export`, {
            params: params.keys().length ? params : undefined,
            responseType: 'blob',
        });
    }

    getHistogram(filters?: {
        itemType?: ItemType;
        status?: BookStatus;
    }): Observable<LetterHistogram> {
        let params = new HttpParams();
        if (filters?.itemType) {
            params = params.set('type', filters.itemType);
        }
        if (filters?.status) {
            params = params.set('status', filters.status);
        }

        return this.http
            .get<{
                histogram: LetterHistogram;
                total: number;
            }>(`${this.baseUrl}/histogram`, { params: params.keys().length ? params : undefined })
            .pipe(map((response) => response.histogram));
    }

    checkDuplicates(input: DuplicateCheckInput): Observable<DuplicateMatch[]> {
        let params = new HttpParams();
        if (input.title) {
            params = params.set('title', input.title);
        }
        if (input.isbn13) {
            params = params.set('isbn13', input.isbn13);
        }
        if (input.isbn10) {
            params = params.set('isbn10', input.isbn10);
        }

        return this.http
            .get<{
                duplicates: DuplicateMatch[];
            }>(`${this.baseUrl}/duplicates`, { params: params.keys().length ? params : undefined })
            .pipe(map((response) => response.duplicates));
    }

    private normalizeForm(form: Partial<ItemForm>): Record<string, unknown> {
        const payload: Record<string, unknown> = { ...form };
        const itemType = payload['itemType'] as ItemType | undefined;

        if ('releaseYear' in payload) {
            const releaseYear = payload['releaseYear'];
            if (releaseYear === '' || releaseYear === null) {
                payload['releaseYear'] = null;
            } else if (typeof releaseYear === 'string') {
                payload['releaseYear'] = Number.parseInt(releaseYear, 10);
            }
        }

        if ('pageCount' in payload) {
            const pageCount = payload['pageCount'];
            if (pageCount === '' || pageCount === null) {
                payload['pageCount'] = null;
            } else if (typeof pageCount === 'string') {
                payload['pageCount'] = Number.parseInt(pageCount, 10);
            }
        }

        if ('currentPage' in payload) {
            const currentPage = payload['currentPage'];
            if (currentPage === '' || currentPage === null) {
                payload['currentPage'] = null;
            } else if (typeof currentPage === 'string') {
                payload['currentPage'] = Number.parseInt(currentPage, 10);
            }
        }

        if ('readAt' in payload) {
            const readAt = payload['readAt'];
            if (readAt === '' || readAt === null) {
                payload['readAt'] = null;
            } else if (readAt instanceof Date) {
                payload['readAt'] = Number.isNaN(readAt.getTime()) ? null : readAt.toISOString();
            } else if (typeof readAt === 'string') {
                const dateValue = new Date(readAt);
                payload['readAt'] = Number.isNaN(dateValue.getTime())
                    ? null
                    : dateValue.toISOString();
            }
        }

        if ('readingStatus' in payload && payload['readingStatus'] === undefined) {
            delete payload['readingStatus'];
        }

        if ('rating' in payload) {
            const rating = payload['rating'];
            if (rating === '' || rating === null) {
                payload['rating'] = null;
            } else if (typeof rating === 'string') {
                payload['rating'] = Number.parseInt(rating, 10);
            }
        }

        if ('retailPriceUsd' in payload) {
            const price = payload['retailPriceUsd'];
            if (price === '' || price === null) {
                payload['retailPriceUsd'] = null;
            } else if (typeof price === 'string') {
                payload['retailPriceUsd'] = Number.parseFloat(price);
            }
        }

        // Handle series fields
        if ('volumeNumber' in payload) {
            const volumeNumber = payload['volumeNumber'];
            if (volumeNumber === '' || volumeNumber === null) {
                payload['volumeNumber'] = null;
            } else if (typeof volumeNumber === 'string') {
                payload['volumeNumber'] = Number.parseInt(volumeNumber, 10);
            }
        }

        if ('totalVolumes' in payload) {
            const totalVolumes = payload['totalVolumes'];
            if (totalVolumes === '' || totalVolumes === null) {
                payload['totalVolumes'] = null;
            } else if (typeof totalVolumes === 'string') {
                payload['totalVolumes'] = Number.parseInt(totalVolumes, 10);
            }
        }

        // Strip book-specific fields for non-books
        if (itemType && itemType !== 'book') {
            delete payload['pageCount'];
            delete payload['currentPage'];
            delete payload['isbn13'];
            delete payload['isbn10'];
            delete payload['format'];
            delete payload['genre'];
            delete payload['rating'];
            delete payload['retailPriceUsd'];
            delete payload['googleVolumeId'];
            delete payload['readingStatus'];
            delete payload['readAt'];
            delete payload['seriesName'];
            delete payload['volumeNumber'];
            delete payload['totalVolumes'];
        }

        // Strip game-specific fields for non-games
        if (itemType && itemType !== 'game') {
            delete payload['platform'];
            delete payload['ageGroup'];
            delete payload['playerCount'];
        }

        return payload;
    }
}
