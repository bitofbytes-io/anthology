import { By } from '@angular/platform-browser';
import { TestBed } from '@angular/core/testing';
import { Router } from '@angular/router';
import { of } from 'rxjs';

import { AddItemPageComponent } from './add-item-page.component';
import {
    AddItemPageHarness,
    createParamMap,
    sampleForm,
    sampleItem,
    setupAddItemPageTest,
} from './add-item-page.testing';
import { SearchPanelComponent } from './search-panel/search-panel.component';
import { ItemForm } from '../../models';

describe(AddItemPageComponent.name, () => {
    let harness: AddItemPageHarness;

    beforeEach(async () => {
        harness = await setupAddItemPageTest();
    });

    function prefillSeries(seriesName: string[], volumeNumber: string[]): void {
        harness.queryParamMapSubject.next(
            createParamMap({ prefill: ['series'], seriesName, volumeNumber }),
        );
    }

    function lookupViaSearchPanel(
        fixture: ReturnType<AddItemPageHarness['createComponent']>,
        query: string,
    ): void {
        const panel = fixture.debugElement.query(By.directive(SearchPanelComponent))
            .componentInstance as SearchPanelComponent;
        panel.searchForm.setValue({ category: 'book', query });
        panel.handleLookupSubmit();
    }

    it('prefills series info and navigates to Search tab using the last repeated query param value', async () => {
        prefillSeries(['First Series', 'Second Series'], ['1', '2']);

        const fixture = harness.createComponent();
        await fixture.whenStable();

        expect(fixture.componentInstance.seriesPrefill()?.seriesName).toBe('Second Series');
        expect(fixture.componentInstance.seriesPrefill()?.volumeNumber).toBe(2);
        expect(fixture.componentInstance.selectedTab()).toBe(0);
    });

    it('merges series prefill into quick add result', async () => {
        prefillSeries(['Harry Potter'], ['3']);
        harness.itemServiceSpy.create.mockReturnValue(
            of(sampleItem({ title: 'Prisoner of Azkaban', creator: 'J.K. Rowling' })),
        );
        const fixture = harness.createComponent();
        await fixture.whenStable();

        const router = TestBed.inject(Router);
        vi.spyOn(router, 'navigate').mockResolvedValue(true);

        fixture.componentInstance.handleQuickAdd(
            sampleForm({
                title: 'Prisoner of Azkaban',
                creator: 'J.K. Rowling',
                releaseYear: 1999,
                pageCount: 317,
                isbn13: '9780439136365',
                seriesName: '',
                volumeNumber: null,
                totalVolumes: null,
            }),
        );
        await fixture.whenStable();

        expect(harness.itemServiceSpy.create).toHaveBeenCalled();
        const [createCall] = harness.itemServiceSpy.create.mock.calls[0]!;
        expect(createCall.seriesName).toBe('Harry Potter');
        expect(createCall.volumeNumber).toBe(3);
    });

    it('merges series prefill when using lookup result for manual entry', async () => {
        prefillSeries(['Harry Potter'], ['3']);

        const fixture = harness.createComponent();
        await fixture.whenStable();

        fixture.componentInstance.handleUseForManual(
            sampleForm({
                title: 'Prisoner of Azkaban',
                creator: 'J.K. Rowling',
                releaseYear: 1999,
                pageCount: 317,
                isbn13: '9780439136365',
                seriesName: '',
                volumeNumber: null,
                totalVolumes: null,
            }),
        );

        expect(fixture.componentInstance.manualDraft()?.seriesName).toBe('Harry Potter');
        expect(fixture.componentInstance.manualDraft()?.volumeNumber).toBe(3);
        expect(fixture.componentInstance.selectedTab()).toBe(1);
    });

    it('navigates back when cancel is invoked while idle', () => {
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const fixture = harness.createComponent();
        fixture.componentInstance.handleCancel();
        expect(navigateSpy).toHaveBeenCalledWith(['/']);
    });

    it('does not navigate away when canceling while busy', () => {
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const fixture = harness.createComponent();
        fixture.componentInstance.busy.set(true);
        fixture.componentInstance.handleCancel();
        expect(navigateSpy).not.toHaveBeenCalled();
    });

    it('looks up metadata and pre-fills manual entry on success', async () => {
        harness.itemLookupServiceSpy.lookup.mockReturnValue(
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

        const fixture = harness.createComponent();
        lookupViaSearchPanel(fixture, '9780000000002');
        await fixture.whenStable();

        const page = fixture.componentInstance;
        expect(harness.itemLookupServiceSpy.lookup).toHaveBeenCalledWith('9780000000002', 'book');
        expect(page.manualDraft()?.title).toBe('Metadata Title');
        expect(page.manualDraft()?.creator).toBe('Someone');
        expect(page.manualDraft()?.pageCount).toBe(320);
        expect(page.manualDraft()?.description).toBe('From lookup');
        expect(page.lookupResults().length).toBe(1);
        expect(page.lookupResults()[0]?.isbn13).toBe('9780000000002');
        expect(page.selectedTab()).toBe(0);
        expect(page.manualDraftSource()).toEqual({ query: '9780000000002', label: 'Book' });
        expect(page.lastLookupSummary()).toBe('Metadata loaded for "9780000000002".');
    });

    it('switches to the manual entry tab when using a lookup result manually', () => {
        const fixture = harness.createComponent();
        const preview: ItemForm = sampleForm({
            title: 'Metadata Title',
            creator: 'Someone',
            releaseYear: 2001,
            pageCount: 320,
            isbn13: '9780000000002',
            isbn10: '0000000002',
            description: 'From lookup',
        });

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
        harness.itemServiceSpy.create.mockReturnValue(
            of(sampleItem({ id: 'item-1', title: 'Metadata Title', creator: 'Someone' })),
        );
        const fixture = harness.createComponent();
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const draft: ItemForm = sampleForm({
            title: 'Metadata Title',
            creator: 'Someone',
            releaseYear: 2001,
            pageCount: 320,
            isbn13: '9780000000002',
            isbn10: '0000000002',
            description: 'From lookup',
        });

        fixture.componentInstance.handleQuickAdd(draft);
        await fixture.whenStable();

        expect(harness.itemServiceSpy.create).toHaveBeenCalledWith(draft);
        expect(navigateSpy).toHaveBeenCalledWith(['/']);
    });

    it('clears the lookup preview when starting fresh', async () => {
        harness.itemLookupServiceSpy.lookup.mockReturnValue(
            of([{ title: 'Metadata Title', creator: 'Someone', releaseYear: 2001 }]),
        );

        const fixture = harness.createComponent();
        lookupViaSearchPanel(fixture, 'test');
        await fixture.whenStable();

        expect(fixture.componentInstance.lookupResults().length).toBe(1);

        fixture.componentInstance.clearManualDraft();

        expect(fixture.componentInstance.lookupResults().length).toBe(0);
        expect(fixture.componentInstance.manualDraft()).toBeNull();
    });
});
