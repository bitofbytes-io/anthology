import { ComponentFixture, TestBed } from '@angular/core/testing';

import { LookupResultsComponent } from './lookup-results.component';
import { ItemForm } from '../../../models';

describe('LookupResultsComponent', () => {
    let component: LookupResultsComponent;
    let fixture: ComponentFixture<LookupResultsComponent>;

    const mockResult: ItemForm = {
        title: 'Test Book',
        creator: 'Test Author',
        itemType: 'book',
        releaseYear: 2023,
        pageCount: 300,
        isbn13: '9781234567890',
        isbn10: '1234567890',
        description: 'A test book description',
        coverImage: '',
        genre: undefined,
        retailPriceUsd: null,
        googleVolumeId: '',
        platform: '',
        ageGroup: '',
        playerCount: '',
        notes: '',
    };

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [LookupResultsComponent],
        }).compileComponents();

        fixture = TestBed.createComponent(LookupResultsComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should not display preview when results are empty', () => {
        component.results = [];
        fixture.detectChanges();

        const preview = fixture.nativeElement.querySelector('.lookup-preview');
        expect(preview).toBeFalsy();
    });

    it('should display preview when results are present', () => {
        component.results = [mockResult];
        fixture.detectChanges();

        const preview = fixture.nativeElement.querySelector('.lookup-preview');
        expect(preview).toBeTruthy();
    });

    it('should display result fields correctly', () => {
        component.results = [mockResult];
        fixture.detectChanges();

        const fields = fixture.nativeElement.querySelectorAll('.preview-field .value');
        expect(fields[0].textContent.trim()).toBe('Test Book');
        expect(fields[1].textContent.trim()).toBe('Test Author');
        expect(fields[2].textContent.trim()).toBe('2023');
        expect(fields[3].textContent.trim()).toBe('300');
        expect(fields[4].textContent.trim()).toBe('9781234567890');
        expect(fields[5].textContent.trim()).toBe('1234567890');
    });

    it('should emit quickAdd when add button is clicked', () => {
        const spy = vi.spyOn(component.quickAdd, 'emit');
        component.results = [mockResult];
        fixture.detectChanges();

        const addButton = fixture.nativeElement.querySelector('.preview-add');
        addButton.click();

        expect(spy).toHaveBeenCalledWith(mockResult);
    });

    it('should not emit quickAdd when busy', () => {
        const spy = vi.spyOn(component.quickAdd, 'emit');
        component.results = [mockResult];
        component.busy = true;
        fixture.detectChanges();

        component.handleQuickAdd(mockResult);

        expect(spy).not.toHaveBeenCalled();
    });

    it('should emit useForManual when manual entry button is clicked', () => {
        const spy = vi.spyOn(component.useForManual, 'emit');
        component.results = [mockResult];
        fixture.detectChanges();

        const buttons = fixture.nativeElement.querySelectorAll('.preview-actions button');
        const manualButton = buttons[1];
        manualButton.click();

        expect(spy).toHaveBeenCalledWith(mockResult);
    });

    it('should display description when present', () => {
        component.results = [mockResult];
        fixture.detectChanges();

        const description = fixture.nativeElement.querySelector('.preview-description p');
        expect(description.textContent.trim()).toBe('A test book description');
    });

    it('should not display description section when description is empty', () => {
        component.results = [{ ...mockResult, description: '' }];
        fixture.detectChanges();

        const description = fixture.nativeElement.querySelector('.preview-description');
        expect(description).toBeFalsy();
    });
});
