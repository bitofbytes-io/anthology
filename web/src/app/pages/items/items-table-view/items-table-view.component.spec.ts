import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal, WritableSignal } from '@angular/core';
import { ItemsTableViewComponent } from './items-table-view.component';
import { Item, ItemTypes } from '../../../models';
import { LetterGroup } from '../items-page.component';

describe('ItemsTableViewComponent', () => {
    let component: ItemsTableViewComponent;
    let fixture: ComponentFixture<ItemsTableViewComponent>;
    let groupedItemsSignal: WritableSignal<LetterGroup[]>;

    const mockItems: Item[] = [
        {
            id: '1',
            title: 'Alpha Book',
            creator: 'Author A',
            itemType: ItemTypes.Book,
            releaseYear: 2023,
            notes: '',
            createdAt: '2023-01-01T00:00:00Z',
            updatedAt: '2023-06-15T00:00:00Z',
        },
        {
            id: '2',
            title: 'Beta Book',
            creator: 'Author B',
            itemType: ItemTypes.Game,
            releaseYear: 2022,
            notes: '',
            createdAt: '2023-01-01T00:00:00Z',
            updatedAt: '2023-06-15T00:00:00Z',
        },
    ];

    const mockGroupedItems: LetterGroup[] = [
        { letter: 'A', items: [mockItems[0]] },
        { letter: 'B', items: [mockItems[1]] },
    ];

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [ItemsTableViewComponent],
        }).compileComponents();

        fixture = TestBed.createComponent(ItemsTableViewComponent);
        component = fixture.componentInstance;
        groupedItemsSignal = signal(mockGroupedItems);
        component.groupedItems = groupedItemsSignal;
        component.typeFilter = signal('all');
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should render letter sections', () => {
        const compiled = fixture.nativeElement as HTMLElement;
        const sections = compiled.querySelectorAll('.letter-section');
        expect(sections.length).toBe(2);
    });

    it('should render letter headers', () => {
        const compiled = fixture.nativeElement as HTMLElement;
        const letters = compiled.querySelectorAll('.header-letter');
        expect(letters[0].textContent).toContain('A');
        expect(letters[1].textContent).toContain('B');
    });

    it('should render tables', () => {
        const compiled = fixture.nativeElement as HTMLElement;
        const tables = compiled.querySelectorAll('.items-table');
        expect(tables.length).toBe(2);
    });

    it('should emit itemSelected when row is clicked', () => {
        const spy = vi.spyOn(component.itemSelected, 'emit');
        component.onItemSelected(mockItems[0]);
        expect(spy).toHaveBeenCalledWith(mockItems[0]);
    });

    it('should emit typeFilterRequested when type chip is clicked', () => {
        const spy = vi.spyOn(component.typeFilterRequested, 'emit');
        const event = new MouseEvent('click');
        vi.spyOn(event, 'stopPropagation');
        component.onTypeFilterRequested(ItemTypes.Book, event);
        expect(spy).toHaveBeenCalledWith(ItemTypes.Book);
        expect(event.stopPropagation).toHaveBeenCalled();
    });

    it('should emit shelfLocationRequested when shelf button clicked', () => {
        const spy = vi.spyOn(component.shelfLocationRequested, 'emit');
        const event = new MouseEvent('click');
        component.onShelfLocationRequested(mockItems[0], event);
        expect(spy).toHaveBeenCalledWith({ item: mockItems[0], event });
    });

    it('should emit itemSelected on Enter key', () => {
        const spy = vi.spyOn(component.itemSelected, 'emit');
        const event = new KeyboardEvent('keydown', { key: 'Enter' });
        component.handleRowKeydown(event, mockItems[0]);
        expect(spy).toHaveBeenCalledWith(mockItems[0]);
    });

    it('should emit itemSelected on Space key', () => {
        const spy = vi.spyOn(component.itemSelected, 'emit');
        const event = new KeyboardEvent('keydown', { key: ' ' });
        component.handleRowKeydown(event, mockItems[0]);
        expect(spy).toHaveBeenCalledWith(mockItems[0]);
    });

    it('should have data-letter attribute on sections', () => {
        const compiled = fixture.nativeElement as HTMLElement;
        const sections = compiled.querySelectorAll('.letter-section');
        expect(sections[0].getAttribute('data-letter')).toBe('A');
        expect(sections[1].getAttribute('data-letter')).toBe('B');
    });

    it('should emit seriesRequested when series button clicked', () => {
        const itemWithSeries: Item = {
            ...mockItems[0],
            seriesName: 'Test Series',
        };
        groupedItemsSignal.set([{ letter: 'A', items: [itemWithSeries] }]);
        fixture.detectChanges();

        const spy = vi.spyOn(component.seriesRequested, 'emit');
        const button = fixture.nativeElement.querySelector('.series-button') as HTMLButtonElement;
        const event = new MouseEvent('click');
        vi.spyOn(event, 'stopPropagation');
        button.dispatchEvent(event);

        expect(spy).toHaveBeenCalledWith({ item: itemWithSeries, event });
        expect(event.stopPropagation).toHaveBeenCalled();
    });
});
