import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { provideHttpClient } from '@angular/common/http';
import { MatDialog } from '@angular/material/dialog';
import { of, Subject } from 'rxjs';
import { ShelfDetailPageComponent } from './shelf-detail-page.component';
import { ShelfService } from '../../../services/shelf.service';
import { ShelfWithLayout } from '../../../models/shelf';

function layout(): ShelfWithLayout {
    return {
        shelf: {
            id: 'shelf',
            name: 'Shelf',
            description: '',
            photoUrl: '',
            createdAt: '',
            updatedAt: '',
        },
        rows: [0, 1, 2].map((rowIndex) => ({
            id: `row${rowIndex}`,
            shelfId: 'shelf',
            rowIndex,
            yStartNorm: 0,
            yEndNorm: 1,
            columns: [
                {
                    id: `col${rowIndex}`,
                    shelfRowId: `row${rowIndex}`,
                    colIndex: 0,
                    xStartNorm: 0,
                    xEndNorm: 1,
                },
            ],
        })),
        slots: [0, 1, 2].map((rowIndex) => ({
            id: `slot${rowIndex}`,
            shelfId: 'shelf',
            shelfRowId: `row${rowIndex}`,
            shelfColumnId: `col${rowIndex}`,
            rowIndex,
            colIndex: 0,
            xStartNorm: 0,
            xEndNorm: 1,
            yStartNorm: 0,
            yEndNorm: 1,
        })),
        placements: [0, 1].map((i) => ({
            item: {
                id: `book${i}`,
                title: `Book ${i}`,
                creator: '',
                itemType: 'book',
                notes: '',
                createdAt: '',
                updatedAt: '',
            },
            placement: {
                id: `p${i}`,
                itemId: `book${i}`,
                shelfId: 'shelf',
                shelfSlotId: `slot${i}`,
                createdAt: '',
            },
        })),
        unplaced: [],
    };
}

describe('Shelf layout removal confirmation', () => {
    const dialog = { open: vi.fn() };
    const service = { updateLayout: vi.fn() };
    let decision: Subject<'confirm' | 'cancel'>;

    beforeEach(async () => {
        vi.clearAllMocks();
        decision = new Subject();
        dialog.open.mockReturnValue({ afterClosed: () => decision });
        service.updateLayout.mockReturnValue(of({ shelf: layout(), displaced: [] }));
        await TestBed.configureTestingModule({
            imports: [ShelfDetailPageComponent],
            providers: [
                provideRouter([]),
                provideHttpClient(),
                { provide: MatDialog, useValue: dialog },
                { provide: ShelfService, useValue: service },
            ],
        }).compileComponents();
    });

    function component(): ShelfDetailPageComponent {
        const component = TestBed.createComponent(ShelfDetailPageComponent).componentInstance;
        component.shelf.set(layout());
        component.mode.set('edit');
        component.resetLayout();
        return component;
    }

    it('counts distinct books across staged row removals and waits for confirmation', () => {
        const page = component();
        const original = page.shelf()!;
        original.placements.push(original.placements[0]);
        page.removeRow(0);
        page.removeRow(0);
        page.saveLayout();
        expect(dialog.open.mock.calls[0][1].data.itemCount).toBe(2);
        expect(dialog.open.mock.calls[0][1].data.message).toContain(
            'They will remain in your library.',
        );
        expect(service.updateLayout).not.toHaveBeenCalled();
        decision.next('confirm');
        expect(service.updateLayout).toHaveBeenCalledExactlyOnceWith('shelf', [
            expect.objectContaining({ slotId: 'slot2', rowIndex: 0 }),
        ]);
    });

    it('cancellation preserves the draft and performs no update', () => {
        const page = component();
        page.removeRow(0);
        page.saveLayout();
        decision.next('cancel');
        expect(service.updateLayout).not.toHaveBeenCalled();
        expect(page.layoutRows().length).toBe(2);
        expect(page.mode()).toBe('edit');
    });

    it('saves directly when only an empty row is removed', () => {
        const page = component();
        page.removeRow(2);
        page.saveLayout();
        expect(dialog.open).not.toHaveBeenCalled();
        expect(service.updateLayout).toHaveBeenCalledOnce();
    });
    it('marks a replacement row as new so its old position cannot retain books', () => {
        const page = component();
        page.removeRow(0);
        page.addRow();
        page.saveLayout();
        decision.next('confirm');
        const submitted = service.updateLayout.mock.calls[0][1];
        expect(submitted[0].slotId).toBe('slot1');
        expect(submitted[2].slotId).toBeUndefined();
        expect(submitted[2].newSlot).toBe(true);
    });
    for (const removal of ['row', 'column']) {
        it(`saves the last ${removal} removal and can edit the empty shelf again`, () => {
            const page = component();
            const single = layout();
            single.rows = single.rows.slice(0, 1);
            single.slots = single.slots.slice(0, 1);
            single.placements = single.placements.slice(0, 1);
            page.shelf.set(single);
            page.selectedSlot.set(single.slots[0]);
            page.resetLayout();
            if (removal === 'row') {
                page.removeRow(0);
            } else {
                page.removeColumn({ rowIndex: 0, colIndex: 0 });
            }
            const empty = { ...single, rows: [], slots: [], placements: [], unplaced: [] };
            service.updateLayout.mockReturnValue(
                of({ shelf: empty, displaced: single.placements }),
            );
            page.saveLayout();
            expect(dialog.open.mock.calls[0][1].data.itemCount).toBe(1);
            expect(service.updateLayout).not.toHaveBeenCalled();
            decision.next('confirm');
            expect(service.updateLayout).toHaveBeenCalledExactlyOnceWith('shelf', []);
            expect(page.selectedSlot()).toBeNull();
            expect(page.layoutRows().length).toBe(0);
            page.startEdit();
            page.addRow();
            expect(page.layoutRows().length).toBe(1);
            page.saveLayout();
            expect(service.updateLayout.mock.calls[1][1]).toEqual([
                expect.objectContaining({ newSlot: true, rowIndex: 0, colIndex: 0 }),
            ]);
        });
    }
    it('keeps a row whose last column was removed until the row itself is removed', () => {
        const page = component();
        page.removeColumn({ rowIndex: 1, colIndex: 0 });
        expect(page.layoutRows()).toEqual([
            { rowIndex: 0, columns: [{ colIndex: 0 }] },
            { rowIndex: 1, columns: [] },
            { rowIndex: 2, columns: [{ colIndex: 0 }] },
        ]);
        expect(page.layoutSlots().map((slot) => [slot.slotId, slot.rowIndex])).toEqual([
            ['slot0', 0],
            ['slot2', 2],
        ]);
        page.removeRow(1);
        expect(page.layoutSlots().map((slot) => [slot.slotId, slot.rowIndex])).toEqual([
            ['slot0', 0],
            ['slot2', 1],
        ]);
    });
    it('copies the last column into a new column and saves resized slots', () => {
        const page = component();
        page.onSlotPositionChanged({
            rowIndex: 0,
            colIndex: 0,
            position: { xStartNorm: 0.1, xEndNorm: 0.4, yStartNorm: 0.2, yEndNorm: 0.3 },
        });
        page.addColumn(0);
        page.onLayoutSlotSelected({ rowIndex: 0, colIndex: 1 });
        page.removeColumn({ rowIndex: 0, colIndex: 1 });
        expect(page.activeLayoutSelection()).toBeNull();
        page.addColumn(0);
        page.saveLayout();
        expect(service.updateLayout.mock.calls[0][1].slice(0, 2)).toEqual([
            {
                newSlot: undefined,
                slotId: 'slot0',
                rowIndex: 0,
                colIndex: 0,
                xStartNorm: 0.1,
                xEndNorm: 0.4,
                yStartNorm: 0.2,
                yEndNorm: 0.3,
            },
            {
                newSlot: true,
                slotId: undefined,
                rowIndex: 0,
                colIndex: 1,
                xStartNorm: 0.1,
                xEndNorm: 0.4,
                yStartNorm: 0.2,
                yEndNorm: 0.3,
            },
        ]);
    });
    it('numbers slots by their position when stored row indexes have a gap', () => {
        const page = component();
        const gapped = layout();
        gapped.rows = [gapped.rows[0], gapped.rows[2]];
        gapped.slots = [gapped.slots[0], gapped.slots[2]];
        page.shelf.set(gapped);
        page.resetLayout();
        expect(page.layoutSlots().map((slot) => [slot.slotId, slot.rowIndex])).toEqual([
            ['slot0', 0],
            ['slot2', 1],
        ]);
        page.onSlotPositionChanged({
            rowIndex: 1,
            colIndex: 0,
            position: { xStartNorm: 0.5, xEndNorm: 1, yStartNorm: 0, yEndNorm: 1 },
        });
        expect(page.layoutSlots()[1].position.xStartNorm).toBe(0.5);
    });
    it('shows displaced books and a library link after the final slot is removed', () => {
        const fixture = TestBed.createComponent(ShelfDetailPageComponent);
        const page = fixture.componentInstance;
        const original = layout();
        page.shelf.set(original);
        page.resetLayout();
        page.removeRow(2);
        page.removeRow(1);
        page.removeRow(0);
        service.updateLayout.mockReturnValue(
            of({
                shelf: { ...original, rows: [], slots: [], placements: [], unplaced: [] },
                displaced: original.placements,
            }),
        );
        page.saveLayout();
        decision.next('confirm');
        fixture.detectChanges();
        const feedback: HTMLElement = fixture.nativeElement.querySelector('[role="status"]');
        expect(page.selectedSlot()).toBeNull();
        expect(feedback.textContent).toContain('The layout was saved.');
        expect(feedback.textContent).toContain('Book 0');
        expect(feedback.textContent).toContain('Book 1');
        expect(feedback.querySelector('a')?.getAttribute('href')).toBe('/');
        expect(fixture.nativeElement.querySelector('app-slot-sidebar')).toBeNull();
    });
});
