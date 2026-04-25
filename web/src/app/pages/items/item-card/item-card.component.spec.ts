import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ItemCardComponent } from './item-card.component';
import { BookStatus, Item, ItemTypes } from '../../../models';

describe('ItemCardComponent', () => {
    let component: ItemCardComponent;
    let fixture: ComponentFixture<ItemCardComponent>;

    const mockItem: Item = {
        id: '1',
        title: 'Test Book',
        creator: 'Test Author',
        itemType: ItemTypes.Book,
        releaseYear: 2023,
        coverImage: 'http://example.com/cover.jpg',
        readingStatus: BookStatus.Reading,
        currentPage: 50,
        pageCount: 200,
        notes: '',
        createdAt: '2023-01-01T00:00:00Z',
        updatedAt: '2023-06-15T00:00:00Z',
    };
    const itemWithSeries: Item = {
        ...mockItem,
        seriesName: 'Test Series',
    };

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [ItemCardComponent],
        }).compileComponents();

        fixture = TestBed.createComponent(ItemCardComponent);
        component = fixture.componentInstance;
    });

    it('should create', () => {
        component.item = mockItem;
        fixture.detectChanges();
        expect(component).toBeTruthy();
    });

    it('should display item title', () => {
        component.item = mockItem;
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('h4')?.textContent).toContain('Test Book');
    });

    it('should display item creator', () => {
        component.item = mockItem;
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.card-meta')?.textContent).toContain('Test Author');
    });

    it('should emit cardClicked when card is clicked', () => {
        component.item = mockItem;
        fixture.detectChanges();
        const spy = vi.spyOn(component.cardClicked, 'emit');
        const card = fixture.nativeElement.querySelector('.item-card');
        card.click();
        expect(spy).toHaveBeenCalledWith(mockItem);
    });

    it('should emit cardClicked on Enter key', () => {
        component.item = mockItem;
        fixture.detectChanges();
        const spy = vi.spyOn(component.cardClicked, 'emit');
        const event = new KeyboardEvent('keydown', { key: 'Enter' });
        component.handleCardKeydown(event);
        expect(spy).toHaveBeenCalledWith(mockItem);
    });

    it('should emit cardClicked on Space key', () => {
        component.item = mockItem;
        fixture.detectChanges();
        const spy = vi.spyOn(component.cardClicked, 'emit');
        const event = new KeyboardEvent('keydown', { key: ' ' });
        component.handleCardKeydown(event);
        expect(spy).toHaveBeenCalledWith(mockItem);
    });

    it('should display placeholder when no cover image', () => {
        const itemWithoutCover: Item = { ...mockItem, coverImage: undefined };
        component.item = itemWithoutCover;
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.card-cover-placeholder')).toBeTruthy();
    });

    it('should display cover image when provided', () => {
        component.item = mockItem;
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.card-cover img')).toBeTruthy();
    });

    it('should show shelf location button when item has shelf placement', () => {
        const itemWithShelf: Item = {
            ...mockItem,
            shelfPlacement: {
                shelfId: 's1',
                shelfName: 'Main Shelf',
                slotId: 'slot1',
                rowIndex: 0,
                colIndex: 1,
            },
        };
        component.item = itemWithShelf;
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.shelf-location-button')).toBeTruthy();
    });

    it('should emit shelfLocationClicked when shelf button clicked', () => {
        const itemWithShelf: Item = {
            ...mockItem,
            shelfPlacement: {
                shelfId: 's1',
                shelfName: 'Main Shelf',
                slotId: 'slot1',
                rowIndex: 0,
                colIndex: 1,
            },
        };
        component.item = itemWithShelf;
        fixture.detectChanges();
        const spy = vi.spyOn(component.shelfLocationClicked, 'emit');
        const button = fixture.nativeElement.querySelector('.shelf-location-button');
        button.click();
        expect(spy).toHaveBeenCalled();
    });

    it('should display reading progress for books in progress', () => {
        component.item = mockItem;
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.textContent).toContain('25%');
        expect(compiled.textContent).toContain('50/200 pages');
    });

    it('should show series button when item has seriesName', () => {
        component.item = itemWithSeries;
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.series-button')).toBeTruthy();
    });

    it('should not show series button when item has no seriesName', () => {
        component.item = mockItem;
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.series-button')).toBeFalsy();
    });

    it('should emit seriesClicked when series button clicked', () => {
        component.item = itemWithSeries;
        fixture.detectChanges();
        const spy = vi.spyOn(component.seriesClicked, 'emit');
        const button = fixture.nativeElement.querySelector('.series-button');
        button.click();
        expect(spy).toHaveBeenCalledWith(
            expect.objectContaining({
                item: itemWithSeries,
                event: expect.any(MouseEvent),
            }),
        );
    });

    it('should have proper aria-label on series button', () => {
        component.item = itemWithSeries;
        fixture.detectChanges();
        const button = fixture.nativeElement.querySelector('.series-button');
        expect(button.getAttribute('aria-label')).toBe('View series: Test Series');
    });
});
