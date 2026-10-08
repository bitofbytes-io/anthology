import { HttpErrorResponse } from '@angular/common/http';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideNoopAnimations } from '@angular/platform-browser/animations';
import { provideRouter } from '@angular/router';
import { Subject, of, throwError } from 'rxjs';

import { CsvImportComponent } from './csv-import.component';
import { CsvCommitResult, CsvCommitRow, CsvImportPreview } from '../../../models/import';
import { ItemService } from '../../../services/item.service';
import { demoPreview } from './csv-import.fixtures';

describe('CsvImportComponent', () => {
    let component: CsvImportComponent;
    let fixture: ComponentFixture<CsvImportComponent>;
    let itemService: {
        previewCsvImport: ReturnType<typeof vi.fn>;
        commitCsvImport: ReturnType<typeof vi.fn>;
    };
    let busy: boolean[];
    let reviewActive: boolean[];

    const csvFile = () => new File(['title'], 'import.csv', { type: 'text/csv' });
    const select = (file: File) =>
        component.handleFileChange({ target: { files: [file] } } as unknown as Event);
    const text = (selector: string): string =>
        fixture.nativeElement.querySelector(selector)?.textContent ?? '';
    const commitButton = (): HTMLButtonElement =>
        fixture.nativeElement.querySelector('.commit-button');

    function allAdded(rows: CsvCommitRow[]): CsvCommitResult {
        return {
            added: rows.length,
            skipped: 0,
            failed: 0,
            interrupted: 0,
            unprocessed: 0,
            rows: rows.map((row) => ({
                row: row.row,
                status: 'added',
                title: row.item.title,
                identifier: '',
                itemId: `new-${row.row}`,
            })),
        };
    }

    function startReview(preview: CsvImportPreview = demoPreview()): File {
        itemService.previewCsvImport.mockReturnValue(of(preview));
        const file = csvFile();
        select(file);
        component.handlePreview();
        fixture.detectChanges();
        return file;
    }

    beforeEach(async () => {
        itemService = { previewCsvImport: vi.fn(), commitCsvImport: vi.fn() };
        await TestBed.configureTestingModule({
            imports: [CsvImportComponent],
            providers: [
                provideNoopAnimations(),
                provideRouter([]),
                { provide: ItemService, useValue: itemService },
            ],
        }).compileComponents();

        fixture = TestBed.createComponent(CsvImportComponent);
        component = fixture.componentInstance;
        busy = [];
        reviewActive = [];
        component.busyChange.subscribe((value) => busy.push(value));
        component.reviewActiveChange.subscribe((value) => reviewActive.push(value));
        fixture.detectChanges();
    });

    it('lists the template columns', () => {
        const fields = fixture.nativeElement.querySelectorAll('.field');
        expect(fields.length).toBe(component.csvFields.length);
        expect(fields[0].textContent.trim()).toBe('title');
    });

    it('selects a valid CSV file', () => {
        const file = csvFile();
        select(file);

        expect(component.selectedFile()).toBe(file);
        expect(component.error()).toBeNull();
    });

    it('shows why a selected file was rejected', () => {
        select(new File(['x'], 'notes.txt', { type: 'text/plain' }));
        fixture.detectChanges();

        expect(component.selectedFile()).toBeNull();
        expect(text('.csv-feedback')).toContain('Only CSV files are allowed.');
    });

    it('rejects files over the size limit', () => {
        const big = new File(['x'], 'big.csv', { type: 'text/csv' });
        Object.defineProperty(big, 'size', { value: 6 * 1024 * 1024 });
        select(big);
        fixture.detectChanges();

        expect(text('.csv-feedback')).toContain('File size exceeds 5 MB limit.');
    });

    it('previews the file without importing anything', () => {
        const file = startReview();

        expect(itemService.previewCsvImport).toHaveBeenCalledExactlyOnceWith(file);
        expect(itemService.commitCsvImport).not.toHaveBeenCalled();
        expect(component.stage()).toBe('review');
        expect(text('.summary-chips')).toContain('6 ready');
        expect(text('.summary-chips')).toContain('2 possible duplicates');
        expect(text('.summary-chips')).toContain('2 need a match');
        expect(commitButton().textContent).toContain('Import 6 selected rows');
        expect(reviewActive).toEqual([true]);
        expect(busy).toEqual([]);
    });

    it('shows the server error when a preview is rejected', () => {
        itemService.previewCsvImport.mockReturnValue(
            throwError(
                () =>
                    new HttpErrorResponse({
                        status: 400,
                        error: { error: 'invalid csv upload: missing required columns: notes' },
                    }),
            ),
        );
        select(csvFile());

        component.handlePreview();
        fixture.detectChanges();

        expect(component.stage()).toBe('upload');
        expect(text('.csv-feedback')).toContain('missing required columns: notes');
    });

    it('cancels a preview in progress without importing anything', () => {
        const response = new Subject<CsvImportPreview>();
        itemService.previewCsvImport.mockReturnValue(response);
        select(csvFile());

        component.handlePreview();
        expect(component.previewBusy()).toBe(true);
        component.handlePreview();
        expect(itemService.previewCsvImport).toHaveBeenCalledTimes(1);

        component.handleCancelPreview();

        expect(response.observed).toBe(false);
        expect(component.previewBusy()).toBe(false);
        expect(component.stage()).toBe('upload');
        expect(itemService.commitCsvImport).not.toHaveBeenCalled();
    });

    it('imports a chosen edition and keeps the choice when details close', () => {
        startReview();
        const toggle = (): HTMLButtonElement =>
            fixture.nativeElement.querySelector(
                '[aria-controls="csv-row-details-6"]',
            ) as HTMLButtonElement;

        toggle().click();
        fixture.detectChanges();
        expect(toggle().getAttribute('aria-expanded')).toBe('true');
        component.handleChoose({ row: 6, index: 0 });
        fixture.detectChanges();
        toggle().click();
        fixture.detectChanges();
        expect(fixture.nativeElement.querySelector('#csv-row-details-6')).toBeNull();
        toggle().click();
        fixture.detectChanges();

        const checked = fixture.nativeElement.querySelector(
            '#csv-row-details-6 mat-radio-button.mat-mdc-radio-checked',
        );
        expect(checked?.textContent).toContain('Paper Moons');
        expect(commitButton().textContent).toContain('Import 7 selected rows');
        expect(component.announcement()).toContain('Row 6 is ready to import.');
    });

    it('explains when a chosen edition makes another row a duplicate', () => {
        startReview();

        component.handleChoose({ row: 6, index: 1 });
        fixture.detectChanges();

        expect(component.announcement()).toContain(
            'Row 10 will now be skipped: Same title as row 6 of this file.',
        );
        expect(component.counts()).toEqual({
            ready: 6,
            duplicate: 3,
            needsMatch: 1,
            selected: 6,
        });
    });

    it('lets ready rows be deselected but never selects duplicates', () => {
        startReview();

        component.handleToggle({ row: 11, selected: false });
        component.handleToggle({ row: 4, selected: true });
        fixture.detectChanges();

        expect(component.counts().selected).toBe(5);
        expect(component.rows().find((row) => row.source.row === 4)?.selected).toBe(false);
        const checkboxes = fixture.nativeElement.querySelectorAll(
            '.review-row input[type="checkbox"]',
        ) as NodeListOf<HTMLInputElement>;
        expect(checkboxes[2].disabled).toBe(true);

        component.handleSelectNone();
        expect(component.counts().selected).toBe(0);
        fixture.detectChanges();
        expect(commitButton().disabled).toBe(true);
        component.handleSelectAll();
        expect(component.counts().selected).toBe(6);
    });

    it('commits the reviewed rows once and shows every outcome', () => {
        startReview();
        component.handleChoose({ row: 6, index: 0 });
        const response = new Subject<CsvCommitResult>();
        itemService.commitCsvImport.mockReturnValue(response);

        component.handleCommit();
        component.handleCommit();
        fixture.detectChanges();

        expect(itemService.commitCsvImport).toHaveBeenCalledTimes(1);
        const sent = itemService.commitCsvImport.mock.calls[0][0] as CsvCommitRow[];
        expect(sent.map((row) => row.row)).toEqual([2, 3, 5, 6, 7, 10, 11]);
        expect(sent[3].item.googleVolumeId).toBe('vol-first');
        expect(sent[3].item.notes).toBe('Signed copy');
        expect(component.stage()).toBe('committing');
        expect(busy).toEqual([true]);
        expect(commitButton().disabled).toBe(true);
        expect(text('.committing')).toContain('Importing 7 rows');

        response.next(allAdded(sent));
        response.complete();
        fixture.detectChanges();

        expect(component.stage()).toBe('result');
        expect(busy).toEqual([true, false]);
        expect(reviewActive.at(-1)).toBe(false);
        const tiles = Array.from(
            fixture.nativeElement.querySelectorAll('.tile') as NodeListOf<HTMLElement>,
        ).map((tile) =>
            Array.from(tile.querySelectorAll('span'))
                .map((part) => part.textContent?.trim())
                .join(' '),
        );
        expect(tiles).toEqual(['7 Added', '2 Skipped', '0 Failed', '1 Not imported']);
        expect(fixture.nativeElement.querySelectorAll('.outcome-row').length).toBe(10);
    });

    it('ignores review changes while saving', () => {
        startReview();
        itemService.commitCsvImport.mockReturnValue(new Subject<CsvCommitResult>());
        component.handleCommit();
        fixture.detectChanges();

        component.handleToggle({ row: 2, selected: false });
        component.handleChoose({ row: 6, index: 0 });
        component.handleReset();
        component.handleSelectNone();
        fixture.detectChanges();

        expect(component.stage()).toBe('committing');
        expect(component.counts().selected).toBe(6);
        const inputs = fixture.nativeElement.querySelectorAll(
            '.review-row input[type="checkbox"]',
        ) as NodeListOf<HTMLInputElement>;
        expect(Array.from(inputs).every((input) => input.disabled)).toBe(true);
    });

    it('warns before unloading only while saving', () => {
        startReview();
        const reviewEvent = {
            preventDefault: vi.fn(),
            returnValue: undefined,
        } as unknown as BeforeUnloadEvent;
        component.handleBeforeUnload(reviewEvent);
        expect(reviewEvent.preventDefault).not.toHaveBeenCalled();

        itemService.commitCsvImport.mockReturnValue(new Subject<CsvCommitResult>());
        component.handleCommit();
        const savingEvent = {
            preventDefault: vi.fn(),
            returnValue: undefined,
        } as unknown as BeforeUnloadEvent;
        component.handleBeforeUnload(savingEvent);
        expect(savingEvent.preventDefault).toHaveBeenCalled();
    });

    it('reports an unknown outcome when the connection fails while saving', () => {
        const file = startReview();
        itemService.commitCsvImport.mockReturnValue(
            throwError(() => new HttpErrorResponse({ status: 0, statusText: 'Unknown Error' })),
        );

        component.handleCommit();
        fixture.detectChanges();

        expect(component.stage()).toBe('unknown');
        expect(itemService.commitCsvImport).toHaveBeenCalledTimes(1);
        expect(busy).toEqual([true, false]);
        const message = text('.unknown-result');
        expect(message).toContain("We couldn't confirm the import result");
        expect(message).toContain('some of them may have been added');
        expect(message).not.toMatch(/0 added|nothing was imported/i);

        itemService.previewCsvImport.mockReturnValue(of(demoPreview()));
        component.handlePreviewAgain();

        expect(itemService.previewCsvImport).toHaveBeenLastCalledWith(file);
        expect(itemService.previewCsvImport).toHaveBeenCalledTimes(2);
        expect(component.stage()).toBe('review');
    });

    it('treats a gateway timeout as an unknown outcome', () => {
        startReview();
        itemService.commitCsvImport.mockReturnValue(
            throwError(() => new HttpErrorResponse({ status: 504 })),
        );

        component.handleCommit();

        expect(component.stage()).toBe('unknown');
    });

    it('returns to the review when the server rejects the request before saving', () => {
        startReview();
        itemService.commitCsvImport.mockReturnValue(
            throwError(
                () =>
                    new HttpErrorResponse({
                        status: 400,
                        error: {
                            error: 'invalid import request: row 2 is included more than once',
                        },
                    }),
            ),
        );

        component.handleCommit();
        fixture.detectChanges();

        expect(component.stage()).toBe('review');
        expect(text('.csv-feedback')).toContain(
            'Nothing was imported. invalid import request: row 2 is included more than once',
        );
        expect(component.counts().selected).toBe(6);
    });

    it('cancels a review without importing and starts over', () => {
        startReview();
        component.handleChoose({ row: 6, index: 0 });

        component.handleReset();
        fixture.detectChanges();

        expect(component.stage()).toBe('upload');
        expect(component.preview()).toBeNull();
        expect(itemService.commitCsvImport).not.toHaveBeenCalled();
        expect(reviewActive.at(-1)).toBe(false);
        expect(fixture.nativeElement.querySelector('input[type="file"]')).not.toBeNull();
    });

    it('starts another import from the result', () => {
        startReview();
        itemService.commitCsvImport.mockImplementation((rows: CsvCommitRow[]) =>
            of(allAdded(rows)),
        );
        component.handleCommit();
        fixture.detectChanges();

        const back = fixture.nativeElement.querySelector('.result-actions a[href="/"]');
        expect(back?.textContent).toContain('Back to library');
        const another = Array.from(
            fixture.nativeElement.querySelectorAll(
                '.result-actions button',
            ) as NodeListOf<HTMLElement>,
        ).find((button) => button.textContent?.includes('Import another CSV'));
        another?.click();
        fixture.detectChanges();

        expect(component.stage()).toBe('upload');
        expect(component.outcome()).toBeNull();
    });
});
