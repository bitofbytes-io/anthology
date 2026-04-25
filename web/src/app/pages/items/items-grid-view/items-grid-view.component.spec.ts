import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';
import { ItemsGridViewComponent } from './items-grid-view.component';
import { Item, ItemTypes } from '../../../models';
import { LetterGroup } from '../items-page.component';

describe('ItemsGridViewComponent', () => {
    let component: ItemsGridViewComponent;
    let fixture: ComponentFixture<ItemsGridViewComponent>;

    const mockItems: Item[] = [
        {
            id: '1',
            title: 'Alpha Book',
            creator: 'Author A',
            itemType: ItemTypes.Book,
            notes: '',
            createdAt: '2023-01-01T00:00:00Z',
            updatedAt: '2023-06-15T00:00:00Z',
        },
        {
            id: '2',
            title: 'Beta Book',
            creator: 'Author B',
            itemType: ItemTypes.Book,
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
            imports: [ItemsGridViewComponent],
        }).compileComponents();

        fixture = TestBed.createComponent(ItemsGridViewComponent);
        component = fixture.componentInstance;
        component.groupedItems = signal(mockGroupedItems);
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
        const headers = compiled.querySelectorAll('.letter-header');
        expect(headers[0].textContent).toContain('A');
        expect(headers[1].textContent).toContain('B');
    });

    it('should render item cards', () => {
        const compiled = fixture.nativeElement as HTMLElement;
        const cards = compiled.querySelectorAll('app-item-card');
        expect(cards.length).toBe(2);
    });

    it('should emit itemSelected when card is clicked', () => {
        const spy = vi.spyOn(component.itemSelected, 'emit');
        component.onItemSelected(mockItems[0]);
        expect(spy).toHaveBeenCalledWith(mockItems[0]);
    });

    it('should emit shelfLocationRequested when shelf location is clicked', () => {
        const spy = vi.spyOn(component.shelfLocationRequested, 'emit');
        const event = new MouseEvent('click');
        component.onShelfLocationRequested({ item: mockItems[0], event });
        expect(spy).toHaveBeenCalledWith({ item: mockItems[0], event });
    });

    it('should have data-letter attribute on sections', () => {
        const compiled = fixture.nativeElement as HTMLElement;
        const sections = compiled.querySelectorAll('.letter-section');
        expect(sections[0].getAttribute('data-letter')).toBe('A');
        expect(sections[1].getAttribute('data-letter')).toBe('B');
    });
});
