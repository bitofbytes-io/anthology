import { TestBed } from '@angular/core/testing';
import { provideNoopAnimations } from '@angular/platform-browser/animations';
import {
    ActivatedRoute,
    ParamMap,
    provideRouter,
    withDisabledInitialNavigation,
} from '@angular/router';
import { MatSnackBar } from '@angular/material/snack-bar';
import { MatDialog } from '@angular/material/dialog';
import { BehaviorSubject, of } from 'rxjs';
import { vi } from 'vitest';

import { AddItemPageComponent } from './add-item-page.component';
import { Item, ItemForm } from '../../models';
import { ItemLookupService } from '../../services/item-lookup.service';
import { ItemService } from '../../services/item.service';
import { SeriesService } from '../../services/series.service';

// Shared TestBed setup for the add-item page specs.

export function createParamMap(params: Record<string, string[]>): ParamMap {
    return {
        keys: Object.keys(params),
        has: (name: string) => (params[name]?.length ?? 0) > 0,
        get: (name: string) => params[name]?.[0] ?? null,
        getAll: (name: string) => [...(params[name] ?? [])],
    } satisfies ParamMap;
}

export function sampleItem(overrides: Partial<Item> = {}): Item {
    return {
        id: 'id-123',
        title: 'Test',
        creator: 'Me',
        itemType: 'book',
        notes: '',
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        ...overrides,
    } satisfies Item;
}

export function sampleForm(overrides: Partial<ItemForm> = {}): ItemForm {
    return {
        title: 'Test',
        creator: 'Me',
        itemType: 'book',
        releaseYear: null,
        pageCount: null,
        isbn13: '',
        isbn10: '',
        description: '',
        notes: '',
        ...overrides,
    };
}

export interface AddItemPageHarness {
    itemServiceSpy: {
        create: ReturnType<typeof vi.fn>;
        importCsv: ReturnType<typeof vi.fn>;
        checkDuplicates: ReturnType<typeof vi.fn>;
    };
    snackBarSpy: { open: ReturnType<typeof vi.fn> };
    itemLookupServiceSpy: { lookup: ReturnType<typeof vi.fn> };
    dialogSpy: { open: ReturnType<typeof vi.fn> };
    queryParamMapSubject: BehaviorSubject<ParamMap>;
    createComponent: () => ReturnType<typeof TestBed.createComponent<AddItemPageComponent>>;
}

/** Configures TestBed for AddItemPageComponent with spied services; call from `beforeEach`. */
export async function setupAddItemPageTest(): Promise<AddItemPageHarness> {
    const itemServiceSpy = {
        create: vi.fn().mockName('ItemService.create'),
        importCsv: vi.fn().mockName('ItemService.importCsv'),
        checkDuplicates: vi.fn().mockName('ItemService.checkDuplicates'),
    };
    itemServiceSpy.checkDuplicates.mockReturnValue(of([])); // Default: no duplicates
    const snackBarSpy = { open: vi.fn().mockName('MatSnackBar.open') };
    const itemLookupServiceSpy = { lookup: vi.fn().mockName('ItemLookupService.lookup') };
    const dialogSpy = { open: vi.fn().mockName('MatDialog.open') };
    const queryParamMapSubject = new BehaviorSubject<ParamMap>(createParamMap({}));
    const mockSeriesService = {
        list: () => of({ series: [], standaloneItems: [] }),
    };

    await TestBed.configureTestingModule({
        imports: [AddItemPageComponent],
        providers: [
            provideNoopAnimations(),
            { provide: ItemService, useValue: itemServiceSpy },
            { provide: ItemLookupService, useValue: itemLookupServiceSpy },
            { provide: SeriesService, useValue: mockSeriesService },
            provideRouter([], withDisabledInitialNavigation()),
            { provide: MatSnackBar, useValue: snackBarSpy },
            { provide: MatDialog, useValue: dialogSpy },
            {
                provide: ActivatedRoute,
                useValue: { queryParamMap: queryParamMapSubject.asObservable() },
            },
        ],
    })
        .overrideProvider(MatSnackBar, { useValue: snackBarSpy })
        .overrideProvider(MatDialog, { useValue: dialogSpy })
        .compileComponents();

    return {
        itemServiceSpy,
        snackBarSpy,
        itemLookupServiceSpy,
        dialogSpy,
        queryParamMapSubject,
        createComponent: () => {
            const fixture = TestBed.createComponent(AddItemPageComponent);
            fixture.detectChanges();
            return fixture;
        },
    };
}
