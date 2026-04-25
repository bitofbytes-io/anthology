import { ComponentFixture, TestBed } from '@angular/core/testing';
import { signal } from '@angular/core';

import { AlphaRailComponent, LetterHistogram } from './alpha-rail.component';

describe('AlphaRailComponent', () => {
    let component: AlphaRailComponent;
    let fixture: ComponentFixture<AlphaRailComponent>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [AlphaRailComponent],
        }).compileComponents();

        fixture = TestBed.createComponent(AlphaRailComponent);
        component = fixture.componentInstance;
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should show only letters with items', () => {
        const histogram: LetterHistogram = { A: 5, C: 3, Z: 1 };
        component.histogram = signal(histogram);
        fixture.detectChanges();

        const visibleLetters = component.visibleLetters();
        expect(visibleLetters).toEqual(['A', 'C', 'Z']);
    });

    it('should include # when non-alphabetic items exist', () => {
        const histogram: LetterHistogram = { A: 5, '#': 2 };
        component.histogram = signal(histogram);
        fixture.detectChanges();

        const visibleLetters = component.visibleLetters();
        expect(visibleLetters).toContain('#');
    });

    it('should emit letterSelected when letter clicked', () => {
        const histogram: LetterHistogram = { A: 5 };
        component.histogram = signal(histogram);
        fixture.detectChanges();

        const emitSpy = vi.spyOn(component.letterSelected, 'emit');
        component.selectLetter('A');

        expect(emitSpy).toHaveBeenCalledWith('A');
    });

    it('should correctly identify active letter', () => {
        component.activeLetter = signal('B');
        fixture.detectChanges();

        expect(component.isActive('B')).toBe(true);
        expect(component.isActive('A')).toBe(false);
    });

    it('should return count for letter', () => {
        const histogram: LetterHistogram = { A: 5, B: 10 };
        component.histogram = signal(histogram);
        fixture.detectChanges();

        expect(component.getCount('A')).toBe(5);
        expect(component.getCount('B')).toBe(10);
        expect(component.getCount('C')).toBe(0);
    });
});
