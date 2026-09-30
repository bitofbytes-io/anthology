import {
    AfterViewInit,
    Component,
    DestroyRef,
    ElementRef,
    Injector,
    OnDestroy,
    ViewChild,
    computed,
    effect,
    inject,
    signal,
} from '@angular/core';
import { MatCardModule } from '@angular/material/card';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { Router } from '@angular/router';
import { takeUntilDestroyed, toObservable } from '@angular/core/rxjs-interop';
import { EMPTY, catchError, switchMap, tap } from 'rxjs';

import {
    BOOK_STATUS_LABELS,
    BookStatus,
    BookStatusFilter,
    BookStatusFilters,
    Item,
    ItemType,
    ItemTypes,
    ITEM_TYPE_LABELS,
    LetterHistogram,
    SeriesSummary,
    SHELF_STATUS_LABELS,
    ShelfStatusFilter,
    ShelfStatusFilters,
} from '../../models';
import { ItemService } from '../../services/item.service';
import { SeriesService } from '../../services/series.service';
import { AlphaRailComponent } from '../../components/alpha-rail/alpha-rail.component';
import {
    ItemsFilterPanelComponent,
    ItemTypeFilter,
    ViewMode,
} from './items-filter-panel/items-filter-panel.component';
import { ItemsGridViewComponent } from './items-grid-view/items-grid-view.component';
import { ItemsTableViewComponent } from './items-table-view/items-table-view.component';
import { ItemsSeriesViewComponent } from './items-series-view/items-series-view.component';
import { ItemsEmptyStateComponent } from './items-empty-state/items-empty-state.component';
import { NotificationService } from '../../services/notification.service';
import { LibraryActionsService } from '../../services/library-actions.service';

export interface LetterGroup {
    letter: string;
    items: Item[];
}

@Component({
    selector: 'app-items-page',
    standalone: true,
    imports: [
        MatCardModule,
        MatProgressBarModule,
        AlphaRailComponent,
        ItemsFilterPanelComponent,
        ItemsGridViewComponent,
        ItemsTableViewComponent,
        ItemsSeriesViewComponent,
        ItemsEmptyStateComponent,
    ],
    templateUrl: './items-page.component.html',
    styleUrl: './items-page.component.scss',
})
export class ItemsPageComponent implements AfterViewInit, OnDestroy {
    private readonly itemService = inject(ItemService);
    private readonly seriesService = inject(SeriesService);
    private readonly notification = inject(NotificationService);
    private readonly libraryActions = inject(LibraryActionsService);
    private readonly destroyRef = inject(DestroyRef);
    private readonly router = inject(Router);
    private readonly injector = inject(Injector);

    @ViewChild('scrollContainer') scrollContainer!: ElementRef<HTMLElement>;

    readonly alphaRailTop = signal<number>(0);
    readonly alphaRailMaxHeight = computed(() => {
        const top = this.alphaRailTop();
        if (!top) return null;
        return `calc(100dvh - ${top + 16}px)`;
    });

    readonly displayedColumns = [
        'title',
        'creator',
        'itemType',
        'releaseYear',
        'updatedAt',
    ] as const;
    readonly items = signal<Item[]>([]);
    readonly loading = signal(false);
    readonly typeFilter = signal<ItemTypeFilter>('all');
    readonly statusFilter = signal<BookStatusFilter>(BookStatusFilters.All);
    readonly shelfStatusFilter = signal<ShelfStatusFilter>(ShelfStatusFilters.All);
    readonly viewMode = signal<ViewMode>('table');
    readonly histogram = signal<LetterHistogram>({});
    readonly activeLetter = signal<string | null>(null);
    readonly seriesData = signal<SeriesSummary[]>([]);
    readonly expandedSeries = signal<Set<string>>(new Set());
    readonly seriesLoading = signal(false);
    readonly exportBusy = signal(false);

    readonly typeOptions: Array<{ value: ItemTypeFilter; label: string }> = [
        { value: 'all', label: 'All items' },
        { value: ItemTypes.Book, label: ITEM_TYPE_LABELS[ItemTypes.Book] },
        { value: ItemTypes.Game, label: ITEM_TYPE_LABELS[ItemTypes.Game] },
        { value: ItemTypes.Movie, label: ITEM_TYPE_LABELS[ItemTypes.Movie] },
        { value: ItemTypes.Music, label: ITEM_TYPE_LABELS[ItemTypes.Music] },
    ];
    readonly statusOptions: Array<{ value: BookStatusFilter; label: string }> = [
        { value: BookStatusFilters.All, label: 'All' },
        { value: BookStatusFilters.None, label: BOOK_STATUS_LABELS[BookStatus.None] },
        { value: BookStatusFilters.WantToRead, label: BOOK_STATUS_LABELS[BookStatus.WantToRead] },
        { value: BookStatusFilters.Reading, label: BOOK_STATUS_LABELS[BookStatus.Reading] },
        { value: BookStatusFilters.Read, label: BOOK_STATUS_LABELS[BookStatus.Read] },
    ];
    readonly shelfStatusOptions: Array<{ value: ShelfStatusFilter; label: string }> = [
        { value: ShelfStatusFilters.All, label: SHELF_STATUS_LABELS[ShelfStatusFilters.All] },
        { value: ShelfStatusFilters.On, label: SHELF_STATUS_LABELS[ShelfStatusFilters.On] },
        { value: ShelfStatusFilters.Off, label: SHELF_STATUS_LABELS[ShelfStatusFilters.Off] },
    ];

    readonly hasFilteredItems = computed(() => this.items().length > 0);
    readonly hasSeriesData = computed(() => this.seriesData().length > 0);
    readonly isUnfiltered = computed(
        () =>
            this.typeFilter() === 'all' &&
            this.statusFilter() === BookStatusFilters.All &&
            this.shelfStatusFilter() === ShelfStatusFilters.All,
    );
    readonly isGridView = computed(() => this.viewMode() === 'grid');
    readonly isSeriesView = computed(() => this.viewMode() === 'series');
    readonly showStatusFilter = computed(() => {
        const type = this.typeFilter();
        return type === 'all' || type === ItemTypes.Book;
    });

    readonly groupedItems = computed<LetterGroup[]>(() => {
        const items = this.items();
        const groups = new Map<string, Item[]>();

        for (const item of items) {
            const letter = this.getFirstLetter(item.title);
            if (!groups.has(letter)) {
                groups.set(letter, []);
            }
            groups.get(letter)!.push(item);
        }

        return Array.from(groups.entries())
            .sort(([a], [b]) => {
                if (a === '#') return 1;
                if (b === '#') return -1;
                return a.localeCompare(b);
            })
            .map(([letter, items]) => ({ letter, items }));
    });

    private observer: IntersectionObserver | null = null;
    private alphaRailUpdateHandler: (() => void) | null = null;

    constructor() {
        const filters$ = toObservable(computed(() => this.currentFilters()));

        this.libraryActions.exportRequested$
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe(() => this.exportToCsv());

        // Load items when filters change
        filters$
            .pipe(
                switchMap((filters) => {
                    this.loading.set(true);
                    return this.itemService.list(filters).pipe(
                        tap((items) => {
                            const sortedItems = [...items].sort((a, b) =>
                                a.title.localeCompare(b.title, undefined, { sensitivity: 'base' }),
                            );
                            this.items.set(sortedItems);
                            this.loading.set(false);
                        }),
                        catchError(() => {
                            this.loading.set(false);
                            this.notification.error('Unable to load your anthology right now.');
                            return EMPTY;
                        }),
                    );
                }),
                takeUntilDestroyed(this.destroyRef),
            )
            .subscribe();

        // Load histogram when filters change
        filters$
            .pipe(
                switchMap((filters) => {
                    return this.itemService.getHistogram(filters).pipe(
                        tap((histogram) => this.histogram.set(histogram)),
                        catchError(() => EMPTY),
                    );
                }),
                takeUntilDestroyed(this.destroyRef),
            )
            .subscribe();

        // Load series data when switching to series view
        const viewMode$ = toObservable(this.viewMode);
        viewMode$
            .pipe(
                switchMap((mode) => {
                    if (mode !== 'series') {
                        return EMPTY;
                    }
                    this.seriesLoading.set(true);
                    return this.seriesService.list({ includeItems: true }).pipe(
                        tap((response) => {
                            this.seriesData.set(response.series);
                            this.seriesLoading.set(false);
                        }),
                        catchError(() => {
                            this.seriesLoading.set(false);
                            this.notification.error('Unable to load series data.');
                            return EMPTY;
                        }),
                    );
                }),
                takeUntilDestroyed(this.destroyRef),
            )
            .subscribe();
    }

    ngAfterViewInit(): void {
        this.setupScrollObserver();
        this.setupAlphaRailMetrics();

        effect(
            () => {
                // Re-attach the observer after the list renders or changes
                this.groupedItems();
                setTimeout(() => this.observeLetterSections(), 0);
            },
            { injector: this.injector },
        );
    }

    ngOnDestroy(): void {
        this.observer?.disconnect();
        this.teardownAlphaRailMetrics();
    }

    private setupAlphaRailMetrics(): void {
        const update = () => this.updateAlphaRailMetrics();
        this.alphaRailUpdateHandler = update;

        // Ensure first measurement happens after initial layout.
        requestAnimationFrame(update);

        window.addEventListener('resize', update);
        window.addEventListener('scroll', update, { passive: true });
    }

    private teardownAlphaRailMetrics(): void {
        const update = this.alphaRailUpdateHandler;
        if (!update) return;

        window.removeEventListener('resize', update);
        window.removeEventListener('scroll', update);
        this.alphaRailUpdateHandler = null;
    }

    private updateAlphaRailMetrics(): void {
        const container = this.scrollContainer?.nativeElement;
        if (!container) return;
        const rect = container.getBoundingClientRect();
        this.alphaRailTop.set(Math.max(0, Math.round(rect.top)));
    }

    private setupScrollObserver(): void {
        const options: IntersectionObserverInit = {
            root: this.scrollContainer?.nativeElement,
            rootMargin: '-10% 0px -85% 0px',
            threshold: [0, 0.1, 0.5, 1],
        };

        this.observer = new IntersectionObserver((entries) => {
            const visible = entries
                .filter((e) => e.isIntersecting)
                .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);

            if (visible.length > 0) {
                const letter = visible[0].target.getAttribute('data-letter');
                if (letter) {
                    this.activeLetter.set(letter);
                }
            }
        }, options);

        // Initial observation after DOM renders
        setTimeout(() => this.observeLetterSections(), 0);
    }

    private observeLetterSections(): void {
        if (!this.observer) return;

        this.observer.disconnect();
        const sections = document.querySelectorAll('[data-letter]');
        sections.forEach((el) => this.observer!.observe(el));
    }

    scrollToLetter(letter: string): void {
        if (letter === 'ALL') {
            this.scrollContainer?.nativeElement.scrollTo({ top: 0, behavior: 'smooth' });
            this.activeLetter.set(null);
            return;
        }

        const target = document.querySelector(`[data-letter="${letter}"]`);
        if (target) {
            target.scrollIntoView({ behavior: 'smooth', block: 'start' });
        }
    }

    onLetterSelected(letter: string): void {
        this.scrollToLetter(letter);
    }

    startCreate(): void {
        this.router.navigate(['/items/add']);
    }

    startEdit(item: Item): void {
        this.router.navigate(['/items', item.id, 'edit']);
    }

    viewShelfPlacement(item: Item, event: MouseEvent): void {
        event.stopPropagation();
        const placement = item.shelfPlacement;
        if (!placement) {
            return;
        }
        this.router.navigate(['/shelves', placement.shelfId], {
            queryParams: { slot: placement.slotId },
        });
    }

    filterByType(itemType: ItemType): void {
        this.setTypeFilter(itemType);
    }

    setTypeFilter(type: ItemTypeFilter): void {
        this.typeFilter.set(type);
        // Clear status filter when switching to non-book type (UI-3)
        if (type !== 'all' && type !== ItemTypes.Book) {
            this.statusFilter.set(BookStatusFilters.All);
        }
    }

    setStatusFilter(status: BookStatusFilter): void {
        this.statusFilter.set(status);
    }

    setShelfStatusFilter(shelfStatus: ShelfStatusFilter): void {
        this.shelfStatusFilter.set(shelfStatus);
    }

    setViewMode(mode: ViewMode): void {
        this.viewMode.set(mode);
    }

    exportToCsv(): void {
        if (this.exportBusy()) {
            return;
        }

        this.exportBusy.set(true);
        const filters = this.currentFilters();

        this.itemService
            .exportCsv(filters)
            .pipe(
                tap((blob) => {
                    this.downloadBlob(blob, this.generateExportFilename());
                    this.notification.info('Library exported successfully');
                }),
                catchError(() => {
                    this.notification.error('Failed to export library');
                    return EMPTY;
                }),
                takeUntilDestroyed(this.destroyRef),
            )
            .subscribe({
                complete: () => this.exportBusy.set(false),
                error: () => this.exportBusy.set(false),
            });
    }

    private downloadBlob(blob: Blob, filename: string): void {
        const url = window.URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = filename;
        link.style.display = 'none';
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        window.setTimeout(() => window.URL.revokeObjectURL(url), 1000);
    }

    private generateExportFilename(): string {
        const date = new Date().toISOString().split('T')[0];
        return `anthology-export-${date}.csv`;
    }

    viewShelfPlacementFromChild(data: { item: Item; event: MouseEvent }): void {
        this.viewShelfPlacement(data.item, data.event);
    }

    viewSeriesFromChild(data: { item: Item; event: MouseEvent }): void {
        this.viewSeriesDetail(data.item.seriesName!);
    }

    toggleSeriesExpanded(seriesName: string): void {
        const current = this.expandedSeries();
        const newSet = new Set(current);
        if (newSet.has(seriesName)) {
            newSet.delete(seriesName);
        } else {
            newSet.add(seriesName);
        }
        this.expandedSeries.set(newSet);
    }

    addMissingVolume(data: { seriesName: string; volumeNumber: number }): void {
        this.router.navigate(['/items/add'], {
            queryParams: {
                prefill: 'series',
                seriesName: data.seriesName,
                volumeNumber: data.volumeNumber,
            },
        });
    }

    viewSeriesDetail(seriesName: string): void {
        this.router.navigate(['/series'], {
            queryParams: { name: seriesName },
        });
    }

    private getFirstLetter(title: string): string {
        const trimmed = title.trim();
        if (trimmed.length === 0) {
            return '#';
        }

        const first = trimmed[0].toUpperCase();
        if (first >= 'A' && first <= 'Z') {
            return first;
        }
        return '#';
    }

    private currentFilters():
        { itemType?: ItemType; status?: BookStatus; shelfStatus?: ShelfStatusFilter } | undefined {
        const filters: {
            itemType?: ItemType;
            status?: BookStatus;
            shelfStatus?: ShelfStatusFilter;
        } = {};

        const typeFilter = this.typeFilter();
        if (typeFilter !== 'all') {
            filters.itemType = typeFilter;
        }

        // Only include status filter when type is 'all' or 'book' (FE-1, FE-2)
        const statusFilter = this.statusFilter();
        if (
            statusFilter !== BookStatusFilters.All &&
            (typeFilter === 'all' || typeFilter === ItemTypes.Book)
        ) {
            filters.status = statusFilter;
        }

        const shelfStatusFilter = this.shelfStatusFilter();
        if (shelfStatusFilter !== ShelfStatusFilters.All) {
            filters.shelfStatus = shelfStatusFilter;
        }

        return Object.keys(filters).length > 0 ? filters : undefined;
    }
}
