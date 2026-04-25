import { SimpleChange } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { of } from 'rxjs';

import { ItemFormComponent } from './item-form.component';
import { Item, BookStatus, Formats } from '../../models';
import { SeriesService } from '../../services/series.service';

describe(ItemFormComponent.name, () => {
    const mockSeriesService = {
        list: () => of({ series: [], standaloneItems: [] }),
    };

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [ItemFormComponent],
            providers: [{ provide: SeriesService, useValue: mockSeriesService }],
        }).compileComponents();
    });

    function createComponent() {
        const fixture = TestBed.createComponent(ItemFormComponent);
        fixture.detectChanges();
        return fixture;
    }

    it('populates form fields when an item input is provided', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        const existing: Item = {
            id: '1',
            title: 'Neuromancer',
            creator: 'William Gibson',
            itemType: 'book',
            releaseYear: 1984,
            pageCount: 271,
            isbn13: '9780441569595',
            isbn10: '0441569595',
            description: 'Cyberpunk classic',
            coverImage: 'https://example.com/cover.jpg',
            notes: 'Cyberpunk classic',
            createdAt: new Date().toISOString(),
            updatedAt: new Date().toISOString(),
        };

        component.item = existing;
        component.ngOnChanges({
            item: new SimpleChange(null, existing, false),
        });

        expect(component.form.value).toEqual(
            expect.objectContaining({
                title: 'Neuromancer',
                creator: 'William Gibson',
                itemType: 'book',
                releaseYear: 1984,
                pageCount: 271,
                isbn13: '9780441569595',
                isbn10: '0441569595',
                description: 'Cyberpunk classic',
                coverImage: 'https://example.com/cover.jpg',
                notes: 'Cyberpunk classic',
            }),
        );
    });

    it('marks controls as touched when attempting to submit invalid form', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        component.form.patchValue({ title: '' });
        component.submit();

        expect(component.form.get('title')?.touched).toBe(true);
        expect(component.form.valid).toBe(false);
    });

    it('emits normalized form values on submit', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        const saveSpy = vi.fn();
        component.save.subscribe(saveSpy);

        component.form.setValue({
            title: 'Arrival',
            creator: 'Denis Villeneuve',
            itemType: 'movie',
            releaseYear: null,
            pageCount: null,
            currentPage: null,
            isbn13: '',
            isbn10: '',
            description: '',
            coverImage: '',
            format: Formats.Unknown,
            genre: '',
            rating: null,
            retailPriceUsd: null,
            googleVolumeId: '',
            platform: '',
            ageGroup: '',
            playerCount: '',
            readingStatus: BookStatus.None,
            readAt: null,
            notes: '',
            seriesName: '',
            volumeNumber: null,
            totalVolumes: null,
        });

        component.submit();

        expect(saveSpy).toHaveBeenCalledWith(
            expect.objectContaining({
                title: 'Arrival',
                releaseYear: null,
                pageCount: null,
            }),
        );
    });

    it('clears the release year field when requested', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        component.form.patchValue({ releaseYear: 2020 });

        component.clearReleaseYear();

        expect(component.form.get('releaseYear')?.value).toBeNull();
    });

    it('prevents current page from exceeding the total pages', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        component.form.patchValue({
            title: 'Test Book',
            itemType: 'book',
            pageCount: 100,
            readingStatus: BookStatus.Reading,
            currentPage: 150,
        });

        component.submit();

        expect(component.form.get('currentPage')?.hasError('maxPages')).toBe(true);
    });

    it('sets cover error when onCoverErrorSet is called', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;

        component.onCoverErrorSet('Test error');

        expect(component.coverImageError).toBe('Test error');
    });

    it('clears cover error when onCoverErrorCleared is called', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        component.coverImageError = 'Some error';

        component.onCoverErrorCleared();

        expect(component.coverImageError).toBeNull();
    });
});
