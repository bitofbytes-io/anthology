import { TestBed } from '@angular/core/testing';

import { AddItemPageComponent } from './add-item-page.component';
import { addItemDeactivateGuard } from './add-item.guard';
import { AddItemPageHarness, setupAddItemPageTest } from './add-item-page.testing';

describe(`${AddItemPageComponent.name} CSV import tab`, () => {
    let harness: AddItemPageHarness;

    beforeEach(async () => {
        harness = await setupAddItemPageTest();
    });

    it('disables the other tabs while a CSV import is uploading', () => {
        const fixture = harness.createComponent();
        const tabs = (): HTMLElement[] =>
            Array.from(fixture.nativeElement.querySelectorAll('[role="tab"]'));

        fixture.componentInstance.csvImportBusy.set(true);
        fixture.detectChanges();
        expect(tabs().map((tab) => tab.getAttribute('aria-disabled'))).toEqual([
            'true',
            'true',
            'false',
        ]);

        fixture.componentInstance.csvImportBusy.set(false);
        fixture.detectChanges();
        expect(tabs().every((tab) => tab.getAttribute('aria-disabled') === 'false')).toBe(true);
    });

    it('keeps the CSV tab selected while a review is open', () => {
        const fixture = harness.createComponent();
        const tabs = (): HTMLElement[] =>
            Array.from(fixture.nativeElement.querySelectorAll('[role="tab"]'));

        fixture.componentInstance.csvReviewActive.set(true);
        fixture.detectChanges();
        expect(tabs().map((tab) => tab.getAttribute('aria-disabled'))).toEqual([
            'true',
            'true',
            'false',
        ]);
    });

    it('blocks leaving the page only while CSV rows are saving', () => {
        const fixture = harness.createComponent();
        const page = fixture.componentInstance;

        page.csvReviewActive.set(true);
        expect(page.canDeactivate()).toBe(true);

        page.csvImportBusy.set(true);
        harness.snackBarSpy.open.mockClear();
        expect(page.canDeactivate()).toBe(false);
        expect(harness.snackBarSpy.open).toHaveBeenCalledWith(
            'Your CSV import is still saving. Stay on this page until it finishes.',
            'Dismiss',
            expect.anything(),
        );
        expect(
            TestBed.runInInjectionContext(() =>
                addItemDeactivateGuard(page, {} as never, {} as never, { url: '/' } as never),
            ),
        ).toBe(false);
    });
});
