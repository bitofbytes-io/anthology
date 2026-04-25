import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';

import { ItemsSeriesViewComponent } from './items-series-view.component';
import { SeriesSummary } from '../../../models';

describe(ItemsSeriesViewComponent.name, () => {
    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [ItemsSeriesViewComponent, NoopAnimationsModule],
        }).compileComponents();
    });

    function createComponent() {
        const fixture = TestBed.createComponent(ItemsSeriesViewComponent);
        fixture.componentInstance.seriesData = signal<SeriesSummary[]>([]);
        fixture.componentInstance.expandedSeries = signal<Set<string>>(new Set());
        fixture.detectChanges();
        return fixture;
    }

    it('creates the component', () => {
        const fixture = createComponent();
        expect(fixture.componentInstance).toBeTruthy();
    });

    it('emits seriesToggled when panel is toggled', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        const spy = vi.fn();
        component.seriesToggled.subscribe(spy);

        component.onPanelToggle('Harry Potter');

        expect(spy).toHaveBeenCalledWith('Harry Potter');
    });

    it('emits addMissingVolume with series name and volume number', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        const spy = vi.fn();
        component.addMissingVolume.subscribe(spy);

        component.onAddMissingVolume('Harry Potter', 3);

        expect(spy).toHaveBeenCalledWith({ seriesName: 'Harry Potter', volumeNumber: 3 });
    });

    it('returns correct status class for complete', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;

        expect(component.getStatusClass('complete')).toBe('status-complete');
    });

    it('returns correct status class for incomplete', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;

        expect(component.getStatusClass('incomplete')).toBe('status-incomplete');
    });

    it('returns correct status class for unknown', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;

        expect(component.getStatusClass('unknown')).toBe('status-unknown');
    });

    it('returns correct status label for complete', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;

        expect(component.getStatusLabel('complete')).toBe('Complete');
    });

    it('returns correct status label for incomplete', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;

        expect(component.getStatusLabel('incomplete')).toBe('Incomplete');
    });

    it('returns correct status label for unknown', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;

        expect(component.getStatusLabel('unknown')).toBe('Unknown');
    });
});
