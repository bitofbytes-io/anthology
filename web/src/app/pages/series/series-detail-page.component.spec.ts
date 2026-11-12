import { TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { provideHttpClient, withXhr } from '@angular/common/http';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';

import { SeriesDetailPageComponent } from './series-detail-page.component';

describe(SeriesDetailPageComponent.name, () => {
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [SeriesDetailPageComponent, NoopAnimationsModule],
            providers: [provideRouter([]), provideHttpClient(withXhr())],
        }).compileComponents();
    });

    it('creates the component', () => {
        const fixture = TestBed.createComponent(SeriesDetailPageComponent);
        expect(fixture.componentInstance).toBeTruthy();
    });

    it('returns correct status class for complete', () => {
        const fixture = TestBed.createComponent(SeriesDetailPageComponent);
        const component = fixture.componentInstance;
        expect(component.getStatusClass('complete')).toBe('status-complete');
    });

    it('returns correct status class for incomplete', () => {
        const fixture = TestBed.createComponent(SeriesDetailPageComponent);
        const component = fixture.componentInstance;
        expect(component.getStatusClass('incomplete')).toBe('status-incomplete');
    });

    it('returns correct status label for complete', () => {
        const fixture = TestBed.createComponent(SeriesDetailPageComponent);
        const component = fixture.componentInstance;
        expect(component.getStatusLabel('complete')).toBe('Complete');
    });

    it('returns correct status label for incomplete', () => {
        const fixture = TestBed.createComponent(SeriesDetailPageComponent);
        const component = fixture.componentInstance;
        expect(component.getStatusLabel('incomplete')).toBe('Incomplete');
    });
});
