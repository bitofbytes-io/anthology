import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';

import { CsvImportResultComponent } from './import-result.component';
import { ImportOutcome, OutcomeRow } from '../csv-import-review';
import { libraryMatch } from '../csv-import.fixtures';

function row(overrides: Partial<OutcomeRow>): OutcomeRow {
    return {
        row: 2,
        status: 'added',
        title: 'The Lantern Archive',
        identifier: '',
        message: '',
        libraryMatches: [],
        sent: true,
        ...overrides,
    };
}

function outcome(rows: OutcomeRow[]): ImportOutcome {
    const counts = { added: 0, skipped: 0, failed: 0, interrupted: 0, unprocessed: 0, unknown: 0 };
    for (const entry of rows) {
        counts[entry.status]++;
    }
    return { rows, counts };
}

describe('CsvImportResultComponent', () => {
    let fixture: ComponentFixture<CsvImportResultComponent>;

    const tiles = (): string[] =>
        Array.from(fixture.nativeElement.querySelectorAll('.tile') as NodeListOf<HTMLElement>).map(
            (tile) =>
                Array.from(tile.querySelectorAll('span'))
                    .map((part) => part.textContent?.trim())
                    .join(' '),
        );

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [CsvImportResultComponent],
            providers: [provideRouter([])],
        }).compileComponents();
        fixture = TestBed.createComponent(CsvImportResultComponent);
    });

    it('shows totals and a line per row with links', () => {
        fixture.componentRef.setInput(
            'outcome',
            outcome([
                row({ row: 2, itemId: 'new-2' }),
                row({
                    row: 3,
                    status: 'skipped',
                    message: 'Same title as an item already in your library',
                    libraryMatches: [libraryMatch()],
                    sent: false,
                }),
                row({
                    row: 4,
                    status: 'unprocessed',
                    message: 'Not selected for import.',
                    sent: false,
                }),
            ]),
        );
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('h4').textContent).toContain('Import finished');
        expect(tiles()).toEqual(['1 Added', '1 Skipped', '0 Failed', '1 Not imported']);
        const failedTile = fixture.nativeElement.querySelectorAll('.tile')[2] as HTMLElement;
        expect(failedTile.classList.contains('tile-failed')).toBe(false);
        const links = Array.from(
            fixture.nativeElement.querySelectorAll(
                '.outcome-link',
            ) as NodeListOf<HTMLAnchorElement>,
        ).map((link) => [link.textContent?.trim(), link.getAttribute('href')]);
        expect(links).toEqual([
            ['Open', '/items/new-2/edit'],
            ['Open existing', '/items/existing-1/edit'],
        ]);
    });

    it('explains selected rows the server skipped as already saved', () => {
        fixture.componentRef.setInput(
            'outcome',
            outcome([
                row({
                    row: 2,
                    status: 'skipped',
                    message: 'Same title as an item already in your library when the import ran.',
                    libraryMatches: [libraryMatch()],
                }),
                row({ row: 3, status: 'skipped', sent: false }),
            ]),
        );
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('.result-note')?.textContent).toContain(
            '1 selected row was already in your library when the import ran',
        );
    });

    it('says when the import stopped early and keeps interrupted rows distinct', () => {
        fixture.componentRef.setInput(
            'outcome',
            outcome([
                row({ row: 2 }),
                row({ row: 3, status: 'failed', message: 'This row could not be saved.' }),
                row({ row: 4, status: 'interrupted', message: 'may or may not have been saved' }),
                row({ row: 5, status: 'unprocessed', message: 'Not attempted' }),
            ]),
        );
        fixture.detectChanges();

        expect(fixture.nativeElement.querySelector('h4').textContent).toContain(
            'Import stopped early',
        );
        expect(tiles()).toEqual([
            '1 Added',
            '0 Skipped',
            '1 Failed',
            '1 Stopped while saving',
            '1 Not imported',
        ]);
    });
});
