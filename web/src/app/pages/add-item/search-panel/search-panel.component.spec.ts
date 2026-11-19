import { HttpErrorResponse } from '@angular/common/http';
import { TestBed } from '@angular/core/testing';
import { provideNoopAnimations } from '@angular/platform-browser/animations';
import { of, throwError } from 'rxjs';

import { ItemLookupService } from '../../../services/item-lookup.service';
import { LookupCompletedEvent, SearchPanelComponent } from './search-panel.component';

describe(SearchPanelComponent.name, () => {
    let itemLookupServiceSpy: { lookup: ReturnType<typeof vi.fn> };

    beforeEach(async () => {
        itemLookupServiceSpy = { lookup: vi.fn().mockName('ItemLookupService.lookup') };

        await TestBed.configureTestingModule({
            imports: [SearchPanelComponent],
            providers: [
                provideNoopAnimations(),
                { provide: ItemLookupService, useValue: itemLookupServiceSpy },
            ],
        }).compileComponents();
    });

    function createComponent() {
        const fixture = TestBed.createComponent(SearchPanelComponent);
        const completed: LookupCompletedEvent[] = [];
        fixture.componentInstance.lookupCompleted.subscribe((event) => completed.push(event));
        fixture.detectChanges();
        return { fixture, panel: fixture.componentInstance, completed };
    }

    it('triggers a lookup when a barcode is detected', async () => {
        itemLookupServiceSpy.lookup.mockReturnValue(of([]));
        const { fixture, panel } = createComponent();
        const submitSpy = vi.spyOn(panel, 'handleLookupSubmit');

        panel.handleDetectedBarcode('9781234567890');
        await fixture.whenStable();

        expect(panel.searchForm.get('query')?.value).toBe('9781234567890');
        expect(submitSpy).toHaveBeenCalledWith('scanner');
        expect(itemLookupServiceSpy.lookup).toHaveBeenCalledWith('9781234567890', 'book');
    });

    it('ends the scan session and shows the error when a scanned barcode has no match', async () => {
        itemLookupServiceSpy.lookup.mockReturnValue(of([]));
        const { fixture, panel, completed } = createComponent();
        panel.scanning.set(true);

        panel.handleDetectedBarcode('9781234567890');
        await fixture.whenStable();

        expect(panel.scanning()).toBe(false);
        expect(panel.lookupError()).toBe('No results found. Try another barcode or type the ISBN.');
        expect(completed).toEqual([
            { drafts: [], query: '9781234567890', label: 'Book', summary: null },
        ]);
    });

    it('ends the scan session and shows the error when the scanner fails', () => {
        const { panel } = createComponent();
        panel.toggleScanning();
        expect(panel.scanning()).toBe(true);

        panel.handleScannerFailed('Camera access failed.');

        expect(panel.scanning()).toBe(false);
        expect(panel.lookupError()).toBe('Camera access failed.');
    });

    it('stores an error when lookup fails', async () => {
        itemLookupServiceSpy.lookup.mockReturnValue(throwError(() => new Error('network error')));
        const { fixture, panel, completed } = createComponent();

        panel.searchForm.setValue({ category: 'book', query: 'bad' });
        panel.handleLookupSubmit();
        await fixture.whenStable();

        expect(itemLookupServiceSpy.lookup).toHaveBeenCalled();
        expect(panel.lookupError()).toBeTruthy();
        expect(completed.map((event) => event.drafts.length)).toEqual([0]);
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
        const { fixture, panel, completed } = createComponent();

        panel.searchForm.setValue({ category: 'game', query: '123456789' });
        panel.handleLookupSubmit();
        await fixture.whenStable();

        expect(panel.lookupError()).toBe(
            'metadata lookups for this category are not available yet',
        );
        expect(completed.map((event) => event.drafts.length)).toEqual([0]);
    });
});
