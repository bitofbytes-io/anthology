import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal, WritableSignal } from '@angular/core';
import { ItemsEmptyStateComponent } from './items-empty-state.component';

describe('ItemsEmptyStateComponent', () => {
    let component: ItemsEmptyStateComponent;
    let fixture: ComponentFixture<ItemsEmptyStateComponent>;
    let isUnfiltered: WritableSignal<boolean>;
    let loading: WritableSignal<boolean>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [ItemsEmptyStateComponent],
        }).compileComponents();

        isUnfiltered = signal(true);
        loading = signal(false);

        fixture = TestBed.createComponent(ItemsEmptyStateComponent);
        component = fixture.componentInstance;
        component.isUnfiltered = isUnfiltered;
        component.loading = loading;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should show empty state when unfiltered and not loading', () => {
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.empty-state')).toBeTruthy();
        expect(compiled.querySelector('.no-results')).toBeFalsy();
    });

    it('should show no results when filtered and not loading', () => {
        isUnfiltered.set(false);
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.no-results')).toBeTruthy();
        expect(compiled.querySelector('.empty-state')).toBeFalsy();
    });

    it('should hide content when loading', () => {
        loading.set(true);
        fixture.detectChanges();
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.empty-state')).toBeFalsy();
        expect(compiled.querySelector('.no-results')).toBeFalsy();
    });

    it('should emit createRequested when create button clicked', () => {
        const spy = vi.spyOn(component.createRequested, 'emit');
        const button = fixture.nativeElement.querySelector('button');
        button.click();
        expect(spy).toHaveBeenCalled();
    });
});
