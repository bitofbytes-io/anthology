import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideNoopAnimations } from '@angular/platform-browser/animations';
import { provideRouter } from '@angular/router';

import { CsvImportReviewComponent, RowChoice, RowToggle } from './import-review.component';
import { reviewRows } from '../csv-import-review';
import { demoPreview } from '../csv-import.fixtures';

describe('CsvImportReviewComponent', () => {
    let fixture: ComponentFixture<CsvImportReviewComponent>;
    let toggles: RowToggle[];
    let choices: RowChoice[];

    const rowElement = (row: number): HTMLElement =>
        Array.from(
            fixture.nativeElement.querySelectorAll('.review-row') as NodeListOf<HTMLElement>,
        ).find(
            (element) => element.querySelector('.row-number')?.textContent?.trim() === `Row ${row}`,
        )!;
    const detailsButton = (row: number): HTMLButtonElement =>
        fixture.nativeElement.querySelector(`[aria-controls="csv-row-details-${row}"]`);

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [CsvImportReviewComponent],
            providers: [provideNoopAnimations(), provideRouter([])],
        }).compileComponents();

        fixture = TestBed.createComponent(CsvImportReviewComponent);
        fixture.componentRef.setInput('rows', reviewRows(demoPreview().rows, new Map(), new Set()));
        toggles = [];
        choices = [];
        fixture.componentInstance.rowToggle.subscribe((value) => toggles.push(value));
        fixture.componentInstance.choose.subscribe((value) => choices.push(value));
        fixture.detectChanges();
    });

    it('shows each row with its file row number, status, and explanation', () => {
        const duplicate = rowElement(4);
        expect(duplicate.querySelector('.status-badge')?.textContent).toContain(
            'Possible duplicate',
        );
        expect(duplicate.querySelector('.row-summary')?.textContent).toContain(
            'Same title as an item already in your library. It will be skipped.',
        );
        const problem = rowElement(9);
        expect(problem.querySelector('.status-badge')?.textContent).toContain('Needs match');
        expect(problem.querySelector('.row-summary')?.textContent).toContain(
            'No metadata found for 9783333333333. Fix it in your CSV and preview again.',
        );
        expect(rowElement(5).querySelector('.row-meta')?.textContent).toContain('Game');
    });

    it('only lets ready rows be selected, with labelled checkboxes', () => {
        const inputs = Array.from(
            fixture.nativeElement.querySelectorAll(
                '.review-row input[type="checkbox"]',
            ) as NodeListOf<HTMLInputElement>,
        );
        expect(inputs.map((input) => input.disabled)).toEqual([
            false,
            false,
            true,
            false,
            true,
            false,
            true,
            true,
            false,
            false,
        ]);
        expect(inputs.filter((input) => input.checked)).toHaveLength(6);
        expect(inputs[0].getAttribute('aria-label')).toBe('Import row 2: The Lantern Archive');

        inputs[0].click();
        expect(toggles).toEqual([{ row: 2, selected: false }]);
    });

    it('discloses duplicate comparisons with keyboard-operable buttons', () => {
        const button = detailsButton(4);
        expect(button.tagName).toBe('BUTTON');
        expect(button.getAttribute('aria-expanded')).toBe('false');

        button.click();
        fixture.detectChanges();

        expect(button.getAttribute('aria-expanded')).toBe('true');
        const region = fixture.nativeElement.querySelector('#csv-row-details-4');
        expect(region.getAttribute('role')).toBe('region');
        const labels = Array.from(
            region.querySelectorAll('.compared-label') as NodeListOf<HTMLElement>,
        ).map((label) => label.textContent?.trim());
        expect(labels).toEqual(['This row', 'In your library']);
        const link = region.querySelector('a.open-existing') as HTMLAnchorElement;
        expect(link.getAttribute('href')).toBe('/items/existing-1/edit');
        expect(link.getAttribute('target')).toBe('_blank');
        expect(region.textContent).toContain('Duplicates are skipped. Open the existing item');
        expect(region.textContent).toContain(
            "If this row's details in your CSV are wrong, correct them and preview the file again.",
        );
        expect(region.textContent).not.toContain('anyway');

        detailsButton(8).click();
        fixture.detectChanges();
        const fileLabels = Array.from(
            fixture.nativeElement.querySelectorAll(
                '#csv-row-details-8 .compared-label',
            ) as NodeListOf<HTMLElement>,
        ).map((label) => label.textContent?.trim());
        expect(fileLabels).toEqual(['This row', 'Row 3 of this file']);
    });

    it('offers every catalog edition and emits the choice', () => {
        detailsButton(6).click();
        fixture.detectChanges();

        const options = fixture.nativeElement.querySelectorAll(
            '#csv-row-details-6 mat-radio-button',
        ) as NodeListOf<HTMLElement>;
        expect(options).toHaveLength(3);
        expect(options[0].textContent).toContain('keeps your CSV values for creator');
        expect(options[2].textContent).toContain(
            'Same title as an item already in your library, so it would be skipped',
        );

        (options[1].querySelector('input') as HTMLInputElement).click();
        expect(choices).toEqual([{ row: 6, index: 1 }]);
    });

    it('filters rows by status with pressed-state buttons', () => {
        const filters = Array.from(
            fixture.nativeElement.querySelectorAll('.filter') as NodeListOf<HTMLButtonElement>,
        );
        expect(filters.map((button) => button.textContent?.trim())).toEqual([
            'All (10)',
            'Ready (6)',
            'Possible duplicates (2)',
            'Needs match (2)',
        ]);

        filters[3].click();
        fixture.detectChanges();

        expect(filters[3].getAttribute('aria-pressed')).toBe('true');
        expect(fixture.nativeElement.querySelectorAll('.review-row')).toHaveLength(2);
    });

    it('disables every control while busy', () => {
        fixture.componentRef.setInput(
            'rows',
            reviewRows(demoPreview().rows, new Map([[6, 0]]), new Set()),
        );
        fixture.componentRef.setInput('busy', true);
        detailsButton(6).click();
        fixture.detectChanges();

        const inputs = fixture.nativeElement.querySelectorAll(
            '.review-row input',
        ) as NodeListOf<HTMLInputElement>;
        expect(Array.from(inputs).every((input) => input.disabled)).toBe(true);
        const selectionButtons = fixture.nativeElement.querySelectorAll(
            '.selection-actions button',
        ) as NodeListOf<HTMLButtonElement>;
        expect(Array.from(selectionButtons).every((button) => button.disabled)).toBe(true);
    });
});
