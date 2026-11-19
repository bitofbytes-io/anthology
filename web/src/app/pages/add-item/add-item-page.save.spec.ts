import { TestBed } from '@angular/core/testing';
import { Router } from '@angular/router';
import { of, throwError } from 'rxjs';

import { AddItemPageComponent } from './add-item-page.component';
import {
    AddItemPageHarness,
    sampleForm,
    sampleItem,
    setupAddItemPageTest,
} from './add-item-page.testing';

describe(`${AddItemPageComponent.name} saving`, () => {
    let harness: AddItemPageHarness;

    beforeEach(async () => {
        harness = await setupAddItemPageTest();
    });

    it('creates an item and routes back to the library on save', async () => {
        const { itemServiceSpy, snackBarSpy } = harness;
        itemServiceSpy.create.mockReturnValue(of(sampleItem()));
        snackBarSpy.open.mockClear();
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const fixture = harness.createComponent();
        await fixture.componentInstance.handleSave(sampleForm());
        await fixture.whenStable();
        expect(itemServiceSpy.create).toHaveBeenCalled();
        expect(snackBarSpy.open).toHaveBeenCalled();
        expect(navigateSpy).toHaveBeenCalledWith(['/']);
    });

    it('shows a snack bar message on failure', async () => {
        const { itemServiceSpy, snackBarSpy } = harness;
        snackBarSpy.open.mockClear();
        const consoleErrorSpy = vi.spyOn(console, 'error');
        itemServiceSpy.create.mockReturnValue(throwError(() => new Error('fail')));
        const router = TestBed.inject(Router);
        const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);
        const fixture = harness.createComponent();
        await fixture.componentInstance.handleSave(sampleForm());
        await fixture.whenStable();

        expect(snackBarSpy.open).toHaveBeenCalled();
        expect(navigateSpy).not.toHaveBeenCalled();
        expect(consoleErrorSpy).toHaveBeenCalledWith('Failed to save item', expect.any(Error));
    });

    describe('duplicate check', () => {
        const mockDuplicate = {
            id: 'dup-1',
            title: 'Test',
            primaryIdentifier: '',
            identifierType: '',
            updatedAt: new Date().toISOString(),
        };

        it('prompts for duplicates and only creates when confirmed', async () => {
            const { itemServiceSpy, dialogSpy } = harness;
            itemServiceSpy.checkDuplicates.mockReturnValue(of([mockDuplicate]));
            dialogSpy.open.mockReturnValue({
                afterClosed: () => of<'add' | 'cancel'>('add'),
            } as any);
            itemServiceSpy.create.mockReturnValue(of(sampleItem()));

            const fixture = harness.createComponent();
            const router = TestBed.inject(Router);
            const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);

            await fixture.componentInstance.handleSave(sampleForm());
            await fixture.whenStable();

            expect(dialogSpy.open).toHaveBeenCalled();
            expect(itemServiceSpy.create).toHaveBeenCalled();
            expect(navigateSpy).toHaveBeenCalledWith(['/']);
        });

        it('does not create when duplicate dialog is cancelled', async () => {
            const { itemServiceSpy, dialogSpy } = harness;
            itemServiceSpy.checkDuplicates.mockReturnValue(of([mockDuplicate]));
            dialogSpy.open.mockReturnValue({
                afterClosed: () => of<'add' | 'cancel'>('cancel'),
            } as any);

            const fixture = harness.createComponent();
            const router = TestBed.inject(Router);
            const navigateSpy = vi.spyOn(router, 'navigate').mockResolvedValue(true);

            await fixture.componentInstance.handleSave(sampleForm());
            await fixture.whenStable();

            expect(dialogSpy.open).toHaveBeenCalled();
            expect(itemServiceSpy.create).not.toHaveBeenCalled();
            expect(navigateSpy).not.toHaveBeenCalled();
        });
    });
});
