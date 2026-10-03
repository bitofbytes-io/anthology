import { Item } from './item';

export type SeriesStatus = 'complete' | 'incomplete' | 'unknown';

export interface SeriesSummary {
    seriesName: string;
    ownedCount: number;
    totalVolumes?: number | null;
    missingCount?: number | null;
    status: SeriesStatus;
    items?: Item[];
    missingVolumes?: number[];
}

export interface SeriesListResponse {
    series: SeriesSummary[];
}

export interface MissingVolume {
    seriesName: string;
    volumeNumber: number;
}

export const SERIES_STATUS_LABELS: Record<SeriesStatus, string> = {
    complete: 'Complete',
    incomplete: 'Incomplete',
    unknown: 'Unknown',
};
