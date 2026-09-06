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
        component.resetLayoutForm();
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
        expect(page.rows.length).toBe(2);
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
            page.resetLayoutForm();
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
            expect(page.rows.length).toBe(0);
            page.startEdit();
            page.addRow();
            expect(page.rows.length).toBe(1);
            page.saveLayout();
            expect(service.updateLayout.mock.calls[1][1]).toEqual([
                expect.objectContaining({ newSlot: true, rowIndex: 0, colIndex: 0 }),
            ]);
        });
    }
});
