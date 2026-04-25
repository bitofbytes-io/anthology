import { TestBed } from '@angular/core/testing';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';
import { of } from 'rxjs';

import { BookDetailsComponent } from './book-details.component';
import { BookStatus, Formats } from '../../../models';
import { SeriesService } from '../../../services/series.service';

describe(BookDetailsComponent.name, () => {
    let form: FormGroup;

    const mockSeriesService = {
        list: () => of({ series: [], standaloneItems: [] }),
    };

    beforeEach(async () => {
        const fb = new FormBuilder();
        form = fb.group({
            title: ['', [Validators.required]],
            pageCount: [null, [Validators.min(1)]],
            currentPage: [null, [Validators.min(0)]],
            isbn13: [''],
            isbn10: [''],
            description: [''],
            format: [Formats.Unknown],
            genre: [''],
            rating: [null],
            retailPriceUsd: [null],
            googleVolumeId: [''],
            readingStatus: [BookStatus.None],
            readAt: [null],
            seriesName: [''],
            volumeNumber: [null],
            totalVolumes: [null],
        });

        await TestBed.configureTestingModule({
            imports: [BookDetailsComponent, NoopAnimationsModule],
            providers: [{ provide: SeriesService, useValue: mockSeriesService }],
        }).compileComponents();
    });

    function createComponent({
        hasSeriesData = false,
    }: {
        hasSeriesData?: boolean;
    } = {}) {
        const fixture = TestBed.createComponent(BookDetailsComponent);
        fixture.componentInstance.form = form;
        fixture.componentInstance.bookStatusOptions = [
            { value: BookStatus.None, label: 'Not set' },
            { value: BookStatus.Read, label: 'Read' },
            { value: BookStatus.Reading, label: 'Currently reading' },
        ];
        fixture.componentInstance.formatOptions = [];
        fixture.componentInstance.genreOptions = [];
        fixture.componentInstance.hasSeriesData = hasSeriesData;
        fixture.detectChanges();
        return fixture;
    }

    it('clears the page count when requested', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ pageCount: 400 });

        component.clearPageCount();

        expect(form.get('pageCount')?.value).toBeNull();
    });

    it('clears the current page when requested', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ currentPage: 50 });

        component.clearCurrentPage();

        expect(form.get('currentPage')?.value).toBeNull();
    });

    it('clears the rating when requested', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ rating: 8 });

        component.clearRating();

        expect(form.get('rating')?.value).toBeNull();
    });

    it('clears the retail price when requested', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ retailPriceUsd: 19.99 });

        component.clearRetailPrice();

        expect(form.get('retailPriceUsd')?.value).toBeNull();
    });

    it('clears readAt when status changes from Read', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ readingStatus: BookStatus.Read, readAt: new Date() });

        component.onStatusChange(BookStatus.None);

        expect(form.get('readAt')?.value).toBeNull();
    });

    it('clears currentPage when status changes from Reading', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ readingStatus: BookStatus.Reading, currentPage: 50 });

        component.onStatusChange(BookStatus.Read);

        expect(form.get('currentPage')?.value).toBeNull();
    });

    it('returns true for isReadStatus when reading status is Read', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ readingStatus: BookStatus.Read });

        expect(component.isReadStatus).toBe(true);
    });

    it('returns true for isReadingStatus when reading status is Reading', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ readingStatus: BookStatus.Reading });

        expect(component.isReadingStatus).toBe(true);
    });

    it('clears the series name when requested', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ seriesName: 'Harry Potter' });

        component.clearSeriesName();

        expect(form.get('seriesName')?.value).toBe('');
    });

    it('clears the volume number when requested', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ volumeNumber: 3 });

        component.clearVolumeNumber();

        expect(form.get('volumeNumber')?.value).toBeNull();
    });

    it('clears the total volumes when requested', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ totalVolumes: 7 });

        component.clearTotalVolumes();

        expect(form.get('totalVolumes')?.value).toBeNull();
    });

    describe('series toggle', () => {
        it('should start with series section collapsed by default', () => {
            const fixture = createComponent();
            const component = fixture.componentInstance;

            expect(component.seriesExpanded()).toBe(false);
        });

        it('should auto-expand series section when hasSeriesData is true', () => {
            const fixture = createComponent({ hasSeriesData: true });

            expect(fixture.componentInstance.seriesExpanded()).toBe(true);
        });

        it('should toggle series section visibility', () => {
            const fixture = createComponent();
            const component = fixture.componentInstance;

            expect(component.seriesExpanded()).toBe(false);

            component.toggleSeriesSection();
            expect(component.seriesExpanded()).toBe(true);

            component.toggleSeriesSection();
            expect(component.seriesExpanded()).toBe(false);
        });

        it('should show series fields when expanded', () => {
            const fixture = createComponent();
            fixture.componentInstance.seriesExpanded.set(true);
            fixture.detectChanges();

            const seriesGrid = fixture.nativeElement.querySelector('.series-grid');
            expect(seriesGrid).toBeTruthy();
        });

        it('should hide series fields when collapsed', () => {
            const fixture = createComponent();
            fixture.componentInstance.seriesExpanded.set(false);
            fixture.detectChanges();

            const seriesGrid = fixture.nativeElement.querySelector('.series-grid');
            expect(seriesGrid).toBeNull();
        });
    });
});
