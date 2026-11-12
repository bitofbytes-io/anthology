import {
    Component,
    DestroyRef,
    EventEmitter,
    inject,
    Input,
    OnChanges,
    Output,
    signal,
    SimpleChanges,
    ChangeDetectionStrategy,
} from '@angular/core';

import { FormBuilder, ReactiveFormsModule } from '@angular/forms';
import { MatAutocompleteModule } from '@angular/material/autocomplete';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatSelectModule } from '@angular/material/select';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import {
    catchError,
    combineLatest,
    debounceTime,
    distinctUntilChanged,
    finalize,
    map,
    of,
    startWith,
    switchMap,
} from 'rxjs';

import { Item, ItemType, ITEM_TYPE_LABELS } from '../../../models';
import { ItemService } from '../../../services/item.service';
import { NotificationService } from '../../../services/notification.service';

type ItemTypeFilter = ItemType | 'all';

@Component({
    selector: 'app-item-search',
    standalone: true,
    imports: [
        ReactiveFormsModule,
        MatAutocompleteModule,
        MatFormFieldModule,
        MatInputModule,
        MatSelectModule,
    ],
    templateUrl: './item-search.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './item-search.component.scss',
})
export class ItemSearchComponent implements OnChanges {
    private static readonly MIN_SEARCH_LENGTH = 2;
    private static readonly SEARCH_RESULT_LIMIT = 10;

    private readonly itemService = inject(ItemService);
    private readonly notification = inject(NotificationService);
    private readonly fb = inject(FormBuilder);
    private readonly destroyRef = inject(DestroyRef);

    @Input() disabled = false;
    @Output() itemSelected = new EventEmitter<string>();

    readonly searchControl = this.fb.control('', { nonNullable: true });
    readonly typeControl = this.fb.control<ItemTypeFilter>('all', { nonNullable: true });
    readonly searchResults = signal<Item[]>([]);
    readonly searching = signal(false);
    readonly minSearchLength = ItemSearchComponent.MIN_SEARCH_LENGTH;

    readonly typeOptions: Array<{ value: ItemTypeFilter; label: string }> = [
        { value: 'all', label: 'All items' },
        { value: 'book', label: ITEM_TYPE_LABELS.book },
        { value: 'game', label: ITEM_TYPE_LABELS.game },
        { value: 'movie', label: ITEM_TYPE_LABELS.movie },
        { value: 'music', label: ITEM_TYPE_LABELS.music },
    ];

    constructor() {
        this.initializeSearch();
    }

    ngOnChanges(changes: SimpleChanges): void {
        if (changes['disabled']) {
            this.updateControlsDisabledState();
        }
    }

    private updateControlsDisabledState(): void {
        if (this.disabled) {
            this.searchControl.disable({ emitEvent: false });
            this.typeControl.disable({ emitEvent: false });
        } else {
            this.searchControl.enable({ emitEvent: false });
            this.typeControl.enable({ emitEvent: false });
        }
    }

    get searchQuery(): string {
        return (this.searchControl.value ?? '').trim();
    }

    handleSelection(itemId: string): void {
        this.itemSelected.emit(itemId);
        this.searchControl.setValue('', { emitEvent: false });
        this.searchResults.set([]);
    }

    private initializeSearch(): void {
        const query$ = this.searchControl.valueChanges.pipe(
            startWith(this.searchControl.value),
            debounceTime(250),
            map((value) => (value ?? '').trim()),
            distinctUntilChanged(),
        );
        const type$ = this.typeControl.valueChanges.pipe(startWith(this.typeControl.value));

        combineLatest([query$, type$])
            .pipe(
                switchMap(([query, type]) => this.queryItems(query, type)),
                takeUntilDestroyed(this.destroyRef),
            )
            .subscribe((results) => {
                this.searchResults.set(results);
            });
    }

    private queryItems(query: string, typeFilter: ItemTypeFilter) {
        if (query.length < this.minSearchLength) {
            this.searchResults.set([]);
            this.searching.set(false);
            return of<Item[]>([]);
        }

        this.searching.set(true);
        const filters: { itemType?: ItemType; query: string; limit: number } = {
            query,
            limit: ItemSearchComponent.SEARCH_RESULT_LIMIT,
        };
        if (typeFilter !== 'all') {
            filters.itemType = typeFilter;
        }

        return this.itemService.list(filters).pipe(
            catchError(() => {
                this.notification.error('Unable to search your library');
                return of<Item[]>([]);
            }),
            finalize(() => this.searching.set(false)),
        );
    }
}
