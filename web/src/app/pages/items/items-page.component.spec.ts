import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideNoopAnimations } from '@angular/platform-browser/animations';
import { provideRouter, withDisabledInitialNavigation } from '@angular/router';
import { of, throwError } from 'rxjs';

import { ItemsPageComponent } from './items-page.component';
import { ItemService } from '../../services/item.service';
import { NotificationService } from '../../services/notification.service';
import { LibraryActionsService } from '../../services/library-actions.service';
import { SeriesService } from '../../services/series.service';
import { BookStatusFilters, ItemTypes, ShelfStatusFilters } from '../../models';

class IntersectionObserverStub {
    constructor(_: IntersectionObserverCallback) {}

    observe(): void {}

    unobserve(): void {}

    disconnect(): void {}

    takeRecords(): IntersectionObserverEntry[] {
        return [];
    }
}

describe(ItemsPageComponent.name, () => {
    let fixture: ComponentFixture<ItemsPageComponent>;
    let itemServiceSpy: {
        list: ReturnType<typeof vi.fn>;
        getHistogram: ReturnType<typeof vi.fn>;
        exportCsv: ReturnType<typeof vi.fn>;
    };
    let notificationSpy: {
        info: ReturnType<typeof vi.fn>;
        error: ReturnType<typeof vi.fn>;
    };
    let libraryActions: LibraryActionsService;

    beforeEach(async () => {
        (window as any).IntersectionObserver = IntersectionObserverStub;

        itemServiceSpy = {
            list: vi.fn().mockName('ItemService.list'),
            getHistogram: vi.fn().mockName('ItemService.getHistogram'),
            exportCsv: vi.fn().mockName('ItemService.exportCsv'),
        };
        itemServiceSpy.list.mockReturnValue(of([]));
        itemServiceSpy.getHistogram.mockReturnValue(of({}));
        itemServiceSpy.exportCsv.mockReturnValue(of(new Blob(['test'])));

        notificationSpy = {
            info: vi.fn().mockName('NotificationService.info'),
            error: vi.fn().mockName('NotificationService.error'),
        };

        const seriesServiceStub = {
            list: () => of({ series: [], standaloneItems: [] }),
        };

        await TestBed.configureTestingModule({
            imports: [ItemsPageComponent],
            providers: [
                provideNoopAnimations(),
                { provide: ItemService, useValue: itemServiceSpy },
                { provide: NotificationService, useValue: notificationSpy },
                { provide: SeriesService, useValue: seriesServiceStub },
                provideRouter([], withDisabledInitialNavigation()),
            ],
        }).compileComponents();

        fixture = TestBed.createComponent(ItemsPageComponent);
        libraryActions = TestBed.inject(LibraryActionsService);
        fixture.detectChanges();
    });

    it('exports with current filters when export is requested', () => {
        const component = fixture.componentInstance;
        const downloadSpy = vi.spyOn(component as any, 'downloadBlob');

        component.setTypeFilter(ItemTypes.Book);
        component.setStatusFilter(BookStatusFilters.Reading);
        component.setShelfStatusFilter(ShelfStatusFilters.On);
        fixture.detectChanges();

        libraryActions.requestExport();

        expect(itemServiceSpy.exportCsv).toHaveBeenCalledWith({
            itemType: ItemTypes.Book,
            status: BookStatusFilters.Reading,
            shelfStatus: ShelfStatusFilters.On,
        });
        expect(downloadSpy).toHaveBeenCalled();
        expect(notificationSpy.info).toHaveBeenCalledWith('Library exported successfully');
    });

    it('shows an error when export fails', () => {
        itemServiceSpy.exportCsv.mockReturnValue(throwError(() => new Error('fail')));
        const component = fixture.componentInstance;
        const downloadSpy = vi.spyOn(component as any, 'downloadBlob');

        libraryActions.requestExport();

        expect(notificationSpy.error).toHaveBeenCalledWith('Failed to export library');
        expect(downloadSpy).not.toHaveBeenCalled();
    });
});
