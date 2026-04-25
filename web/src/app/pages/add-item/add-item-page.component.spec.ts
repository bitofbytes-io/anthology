import { HttpErrorResponse } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { provideNoopAnimations } from '@angular/platform-browser/animations';
import {
    ActivatedRoute,
    ParamMap,
    Router,
    provideRouter,
    withDisabledInitialNavigation,
} from '@angular/router';
import { MatSnackBar } from '@angular/material/snack-bar';
import { BehaviorSubject, of, throwError } from 'rxjs';
import { MatDialog } from '@angular/material/dialog';

import { AddItemPageComponent } from './add-item-page.component';
import { ItemService } from '../../services/item.service';
import { Item, ItemForm } from '../../models';
import { CsvImportSummary } from '../../models/import';
import { ItemLookupService } from '../../services/item-lookup.service';
import { SeriesService } from '../../services/series.service';

describe(AddItemPageComponent.name, () => {
    let itemServiceSpy: {
        create: ReturnType<typeof vi.fn>;
        importCsv: ReturnType<typeof vi.fn>;
        checkDuplicates: ReturnType<typeof vi.fn>;
    };
    let snackBarSpy: { open: ReturnType<typeof vi.fn> };
    let itemLookupServiceSpy: { lookup: ReturnType<typeof vi.fn> };
    let dialogSpy: { open: ReturnType<typeof vi.fn> };
    let queryParamMapSubject: BehaviorSubject<ParamMap>;

    const mockSeriesService = {
        list: () => of({ series: [], standaloneItems: [] }),
    };

    function createParamMap(params: Record<string, string[]>): ParamMap {
        return {
            keys: Object.keys(params),
            has: (name: string) => (params[name]?.length ?? 0) > 0,
            get: (name: string) => params[name]?.[0] ?? null,
            getAll: (name: string) => [...(params[name] ?? [])],
        } satisfies ParamMap;
    }

    beforeEach(async () => {
        itemServiceSpy = {
            create: vi.fn().mockName('ItemService.create'),
            importCsv: vi.fn().mockName('ItemService.importCsv'),
            checkDuplicates: vi.fn().mockName('ItemService.checkDuplicates'),
        };
        itemServiceSpy.checkDuplicates.mockReturnValue(of([])); // Default: no duplicates
        snackBarSpy = {
            open: vi.fn().mockName('MatSnackBar.open'),
        };
        itemLookupServiceSpy = {
            lookup: vi.fn().mockName('ItemLookupService.lookup'),
        };
        dialogSpy = {
            open: vi.fn().mockName('MatDialog.open'),
        };
        queryParamMapSubject = new BehaviorSubject<ParamMap>(createParamMap({}));

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
    });

    function createComponent() {
        const fixture = TestBed.createComponent(AddItemPageComponent);
        fixture.detectChanges();
        return fixture;
    }

    it('creates an item and routes back to the library on save', async () => {
        const mockItem = {
            id: 'id-123',
            title: 'Test',
            creator: 'Me',
            itemType: 'book',
            notes: '',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
        } satisfies Item;

        itemServiceSpy.create.mockReturnValue(of(mockItem));
        snackBarSpy.open.mockClear();
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const fixture = createComponent();
        await fixture.componentInstance.handleSave({
            title: 'Test',
            creator: 'Me',
            itemType: 'book',
            releaseYear: null,
            pageCount: null,
            isbn13: '',
            isbn10: '',
            description: '',
            notes: '',
        });
        await fixture.whenStable();
        expect(itemServiceSpy.create).toHaveBeenCalled();
        expect(snackBarSpy.open).toHaveBeenCalled();
        expect(navigateSpy).toHaveBeenCalledWith(['/']);
    });

    it('shows a snack bar message on failure', async () => {
        snackBarSpy.open.mockClear();
        const consoleErrorSpy = vi.spyOn(console, 'error');
        itemServiceSpy.create.mockReturnValue(throwError(() => new Error('fail')));
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const fixture = createComponent();
        await fixture.componentInstance.handleSave({
            title: 'Test',
            creator: 'Me',
            itemType: 'book',
            releaseYear: null,
            pageCount: null,
            isbn13: '',
            isbn10: '',
            description: '',
            notes: '',
        });
        await fixture.whenStable();

        expect(snackBarSpy.open).toHaveBeenCalled();
        expect(navigateSpy).not.toHaveBeenCalled();
        expect(consoleErrorSpy).toHaveBeenCalledWith('Failed to save item', expect.any(Error));
    });

    it('prompts for duplicates and only creates when confirmed', async () => {
        const mockDuplicate = {
            id: 'dup-1',
            title: 'Test',
            primaryIdentifier: '',
            identifierType: '',
            updatedAt: new Date().toISOString(),
        };
        itemServiceSpy.checkDuplicates.mockReturnValue(of([mockDuplicate]));

        const dialogRefSpy = {
            afterClosed: () => of<'add' | 'cancel'>('add'),
        };
        dialogSpy.open.mockReturnValue(dialogRefSpy as any);

        const mockItem = {
            id: 'id-123',
            title: 'Test',
            creator: 'Me',
            itemType: 'book',
            notes: '',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
        } satisfies Item;
        itemServiceSpy.create.mockReturnValue(of(mockItem));

        const fixture = createComponent();
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);

        await fixture.componentInstance.handleSave({
            title: 'Test',
            creator: 'Me',
            itemType: 'book',
            releaseYear: null,
            pageCount: null,
            isbn13: '',
            isbn10: '',
            description: '',
            notes: '',
        });
        await fixture.whenStable();

        expect(dialogSpy.open).toHaveBeenCalled();
        expect(itemServiceSpy.create).toHaveBeenCalled();
        expect(navigateSpy).toHaveBeenCalledWith(['/']);
    });

    it('does not create when duplicate dialog is cancelled', async () => {
        const mockDuplicate = {
            id: 'dup-1',
            title: 'Test',
            primaryIdentifier: '',
            identifierType: '',
            updatedAt: new Date().toISOString(),
        };
        itemServiceSpy.checkDuplicates.mockReturnValue(of([mockDuplicate]));

        const dialogRefSpy = {
            afterClosed: () => of<'add' | 'cancel'>('cancel'),
        };
        dialogSpy.open.mockReturnValue(dialogRefSpy as any);

        const fixture = createComponent();
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);

        await fixture.componentInstance.handleSave({
            title: 'Test',
            creator: 'Me',
            itemType: 'book',
            releaseYear: null,
            pageCount: null,
            isbn13: '',
            isbn10: '',
            description: '',
            notes: '',
        });
        await fixture.whenStable();

        expect(dialogSpy.open).toHaveBeenCalled();
        expect(itemServiceSpy.create).not.toHaveBeenCalled();
        expect(navigateSpy).not.toHaveBeenCalled();
    });

    it('triggers a lookup when a barcode is detected', async () => {
        itemLookupServiceSpy.lookup.mockReturnValue(of([]));
        const fixture = createComponent();
        const submitSpy = vi.spyOn(fixture.componentInstance, 'handleLookupSubmit');

        fixture.componentInstance.handleDetectedBarcode('9781234567890');
        await fixture.whenStable();

        expect(fixture.componentInstance.searchForm.get('query')?.value).toBe('9781234567890');
        expect(submitSpy).toHaveBeenCalledWith('scanner');
        expect(itemLookupServiceSpy.lookup).toHaveBeenCalledWith('9781234567890', 'book');
    });

    it('prefills series info and navigates to Search tab using the last repeated query param value', async () => {
        queryParamMapSubject.next(
            createParamMap({
                prefill: ['series'],
                seriesName: ['First Series', 'Second Series'],
                volumeNumber: ['1', '2'],
            }),
        );

        const fixture = createComponent();
        await fixture.whenStable();

        expect(fixture.componentInstance.seriesPrefill()?.seriesName).toBe('Second Series');
        expect(fixture.componentInstance.seriesPrefill()?.volumeNumber).toBe(2);
        expect(fixture.componentInstance.selectedTab()).toBe(0);
    });

    it('merges series prefill into quick add result', async () => {
        queryParamMapSubject.next(
            createParamMap({
                prefill: ['series'],
                seriesName: ['Harry Potter'],
                volumeNumber: ['3'],
            }),
        );

        const mockItem = {
            id: 'item-1',
            title: 'Prisoner of Azkaban',
            creator: 'J.K. Rowling',
            itemType: 'book',
            notes: '',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
        } satisfies Item;

        itemServiceSpy.create.mockReturnValue(of(mockItem));
        const fixture = createComponent();
        await fixture.whenStable();

        const router = TestBed.inject(Router);
        vi.spyOn(router, 'navigate').mockResolvedValue(true);

        const draft: ItemForm = {
            title: 'Prisoner of Azkaban',
            creator: 'J.K. Rowling',
            itemType: 'book',
            releaseYear: 1999,
            pageCount: 317,
            isbn13: '9780439136365',
            isbn10: '',
            description: '',
            notes: '',
            seriesName: '',
            volumeNumber: null,
            totalVolumes: null,
        };

        fixture.componentInstance.handleQuickAdd(draft);
        await fixture.whenStable();

        expect(itemServiceSpy.create).toHaveBeenCalled();
        const [createCall] = itemServiceSpy.create.mock.calls[0]!;
        expect(createCall.seriesName).toBe('Harry Potter');
        expect(createCall.volumeNumber).toBe(3);
    });

    it('merges series prefill when using lookup result for manual entry', async () => {
        queryParamMapSubject.next(
            createParamMap({
                prefill: ['series'],
                seriesName: ['Harry Potter'],
                volumeNumber: ['3'],
            }),
        );

        const fixture = createComponent();
        await fixture.whenStable();

        const preview: ItemForm = {
            title: 'Prisoner of Azkaban',
            creator: 'J.K. Rowling',
            itemType: 'book',
            releaseYear: 1999,
            pageCount: 317,
            isbn13: '9780439136365',
            isbn10: '',
            description: '',
            notes: '',
            seriesName: '',
            volumeNumber: null,
            totalVolumes: null,
        };

        fixture.componentInstance.handleUseForManual(preview);

        expect(fixture.componentInstance.manualDraft()?.seriesName).toBe('Harry Potter');
        expect(fixture.componentInstance.manualDraft()?.volumeNumber).toBe(3);
        expect(fixture.componentInstance.selectedTab()).toBe(1);
    });

    it('navigates back when cancel is invoked while idle', () => {
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const fixture = createComponent();
        fixture.componentInstance.handleCancel();
        expect(navigateSpy).toHaveBeenCalledWith(['/']);
    });

    it('does not navigate away when canceling while busy', () => {
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const fixture = createComponent();
        fixture.componentInstance.busy.set(true);
        fixture.componentInstance.handleCancel();
        expect(navigateSpy).not.toHaveBeenCalled();
    });

    it('looks up metadata and pre-fills manual entry on success', async () => {
        itemLookupServiceSpy.lookup.mockReturnValue(
            of([
                {
                    title: 'Metadata Title',
                    creator: 'Someone',
                    releaseYear: 2001,
                    pageCount: 320,
                    isbn13: '9780000000002',
                    isbn10: '0000000002',
                    description: 'From lookup',
                },
            ]),
        );

        const fixture = createComponent();
        fixture.componentInstance.searchForm.setValue({ category: 'book', query: '9780000000002' });
        fixture.componentInstance.handleLookupSubmit();
        await fixture.whenStable();

        expect(itemLookupServiceSpy.lookup).toHaveBeenCalledWith('9780000000002', 'book');
        expect(fixture.componentInstance.manualDraft()?.title).toBe('Metadata Title');
        expect(fixture.componentInstance.manualDraft()?.creator).toBe('Someone');
        expect(fixture.componentInstance.manualDraft()?.pageCount).toBe(320);
        expect(fixture.componentInstance.manualDraft()?.description).toBe('From lookup');
        expect(fixture.componentInstance.lookupResults().length).toBe(1);
        expect(fixture.componentInstance.lookupResults()[0]?.isbn13).toBe('9780000000002');
        expect(fixture.componentInstance.selectedTab()).toBe(0);
        expect(fixture.componentInstance.manualDraftSource()).toEqual({
            query: '9780000000002',
            label: 'Book',
        });
    });

    it('switches to the manual entry tab when using a lookup result manually', () => {
        const fixture = createComponent();
        const preview: ItemForm = {
            title: 'Metadata Title',
            creator: 'Someone',
            itemType: 'book',
            releaseYear: 2001,
            pageCount: 320,
            isbn13: '9780000000002',
            isbn10: '0000000002',
            description: 'From lookup',
            notes: '',
        } satisfies ItemForm;

        fixture.componentInstance.manualDraftSource.set({ query: '9780000000002', label: 'Book' });
        fixture.componentInstance.handleUseForManual(preview);

        expect(fixture.componentInstance.manualDraft()).toEqual(preview);
        expect(fixture.componentInstance.selectedTab()).toBe(1);
        expect(fixture.componentInstance.manualDraftSource()).toEqual({
            query: '9780000000002',
            label: 'Book',
        });
    });

    it('adds a lookup result directly to the collection', async () => {
        const mockItem = {
            id: 'item-1',
            title: 'Metadata Title',
            creator: 'Someone',
            itemType: 'book',
            notes: '',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
        } satisfies Item;

        itemServiceSpy.create.mockReturnValue(of(mockItem));
        const fixture = createComponent();
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const draft: ItemForm = {
            title: 'Metadata Title',
            creator: 'Someone',
            itemType: 'book',
            releaseYear: 2001,
            pageCount: 320,
            isbn13: '9780000000002',
            isbn10: '0000000002',
            description: 'From lookup',
            notes: '',
        } satisfies ItemForm;

        fixture.componentInstance.handleQuickAdd(draft);
        await fixture.whenStable();

        expect(itemServiceSpy.create).toHaveBeenCalledWith(draft);
        expect(navigateSpy).toHaveBeenCalledWith(['/']);
    });

    it('stores an error when lookup fails', async () => {
        itemLookupServiceSpy.lookup.mockReturnValue(throwError(() => new Error('network error')));

        const fixture = createComponent();
        fixture.componentInstance.searchForm.setValue({ category: 'book', query: 'bad' });
        fixture.componentInstance.handleLookupSubmit();
        await fixture.whenStable();

        expect(itemLookupServiceSpy.lookup).toHaveBeenCalled();
        expect(fixture.componentInstance.lookupError()).toBeTruthy();
        expect(fixture.componentInstance.manualDraft()).toBeNull();
        expect(fixture.componentInstance.lookupResults().length).toBe(0);
    });

    it('clears the lookup preview when starting fresh', async () => {
        itemLookupServiceSpy.lookup.mockReturnValue(
            of([{ title: 'Metadata Title', creator: 'Someone', releaseYear: 2001 }]),
        );

        const fixture = createComponent();
        fixture.componentInstance.searchForm.setValue({ category: 'book', query: 'test' });
        fixture.componentInstance.handleLookupSubmit();
        await fixture.whenStable();

        expect(fixture.componentInstance.lookupResults().length).toBe(1);

        fixture.componentInstance.clearManualDraft();

        expect(fixture.componentInstance.lookupResults().length).toBe(0);
        expect(fixture.componentInstance.manualDraft()).toBeNull();
    });

    it('uses the server-provided error message when available', async () => {
        itemLookupServiceSpy.lookup.mockReturnValue(
            throwError(
                () =>
                    new HttpErrorResponse({
                        status: 400,
                        error: {
                            error: 'metadata lookups for this category are not available yet',
                        },
                    }),
            ),
        );

        const fixture = createComponent();
        fixture.componentInstance.searchForm.setValue({ category: 'game', query: '123456789' });
        fixture.componentInstance.handleLookupSubmit();
        await fixture.whenStable();

        expect(fixture.componentInstance.lookupError()).toBe(
            'metadata lookups for this category are not available yet',
        );
        expect(fixture.componentInstance.manualDraft()).toBeNull();
    });

    it('uploads a CSV file and stores the summary', async () => {
        const summary = {
            totalRows: 2,
            imported: 2,
            skippedDuplicates: [],
            failed: [],
        } satisfies CsvImportSummary;
        itemServiceSpy.importCsv.mockReturnValue(of(summary));
        const fixture = createComponent();
        fixture.componentInstance.selectedCsvFile.set(
            new File(['title'], 'import.csv', { type: 'text/csv' }),
        );
        fixture.componentInstance.handleCsvImportSubmit();
        await fixture.whenStable();

        expect(itemServiceSpy.importCsv).toHaveBeenCalled();
        expect(fixture.componentInstance.importSummary()).toEqual(summary);
    });

    it('captures CSV import errors from the server', async () => {
        itemServiceSpy.importCsv.mockReturnValue(
            throwError(
                () =>
                    new HttpErrorResponse({
                        status: 400,
                        error: { error: 'missing required columns' },
                    }),
            ),
        );
        const fixture = createComponent();
        fixture.componentInstance.selectedCsvFile.set(
            new File(['title'], 'import.csv', { type: 'text/csv' }),
        );
        fixture.componentInstance.handleCsvImportSubmit();
        await fixture.whenStable();

        expect(fixture.componentInstance.importError()).toBe('missing required columns');
    });

    it('keeps the CSV import tab active when selecting a file', () => {
        const fixture = createComponent();
        fixture.componentInstance.selectedTab.set(0);
        const file = new File(['data'], 'import.csv', { type: 'text/csv' });

        fixture.componentInstance.handleCsvFileSelected(file);

        expect(fixture.componentInstance.selectedTab()).toBe(2);
        expect(fixture.componentInstance.selectedCsvFile()).toBe(file);
    });
});
