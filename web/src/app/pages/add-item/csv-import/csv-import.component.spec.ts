import { HttpErrorResponse } from '@angular/common/http';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, Subject, throwError } from 'rxjs';

import { CsvImportComponent } from './csv-import.component';
import { CsvImportSummary } from '../../../models/import';
import { ItemService } from '../../../services/item.service';

describe('CsvImportComponent', () => {
    let component: CsvImportComponent;
    let fixture: ComponentFixture<CsvImportComponent>;
    let itemService: { importCsv: ReturnType<typeof vi.fn> };

    const csvFile = () => new File(['title'], 'import.csv', { type: 'text/csv' });
    const select = (file: File) =>
        component.handleFileChange({ target: { files: [file] } } as unknown as Event);
    const text = (selector: string): string =>
        fixture.nativeElement.querySelector(selector)?.textContent ?? '';

    beforeEach(async () => {
        itemService = { importCsv: vi.fn() };
        await TestBed.configureTestingModule({
            imports: [CsvImportComponent],
            providers: [{ provide: ItemService, useValue: itemService }],
        }).compileComponents();

        fixture = TestBed.createComponent(CsvImportComponent);
        component = fixture.componentInstance;
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

    it('uploads the file and shows the summary', () => {
        const summary = {
            totalRows: 10,
            imported: 8,
            skippedDuplicates: [{ row: 2, title: 'Duplicate', reason: 'Already exists' }],
            failed: [{ row: 5, title: 'Failed', error: 'Invalid data' }],
        } satisfies CsvImportSummary;
        itemService.importCsv.mockReturnValue(of(summary));
        const file = csvFile();
        select(file);

        component.handleSubmit();
        fixture.detectChanges();

        expect(itemService.importCsv).toHaveBeenCalledExactlyOnceWith(file);
        expect(component.summary()).toEqual(summary);
        expect(component.selectedFile()).toBeNull();
        expect(text('.csv-summary')).toContain('Imported 8 of 10 rows');
    });

    it('keeps the previous summary when a rejected file is selected', () => {
        const summary = {
            totalRows: 1,
            imported: 1,
            skippedDuplicates: [],
            failed: [],
        } satisfies CsvImportSummary;
        itemService.importCsv.mockReturnValue(of(summary));
        select(csvFile());
        component.handleSubmit();

        select(new File(['x'], 'notes.txt', { type: 'text/plain' }));

        expect(component.summary()).toEqual(summary);
        expect(component.error()).toBe('Only CSV files are allowed.');
    });

    it('reports busy while uploading', () => {
        const response = new Subject<CsvImportSummary>();
        itemService.importCsv.mockReturnValue(response);
        const busy: boolean[] = [];
        component.busyChange.subscribe((value) => busy.push(value));
        select(csvFile());

        component.handleSubmit();
        expect(component.busy()).toBe(true);
        component.handleSubmit();
        expect(itemService.importCsv).toHaveBeenCalledTimes(1);

        response.next({ totalRows: 0, imported: 0, skippedDuplicates: [], failed: [] });
        response.complete();
        expect(component.busy()).toBe(false);
        expect(busy).toEqual([true, false]);
    });

    it('shows the server error message', () => {
        itemService.importCsv.mockReturnValue(
            throwError(
                () =>
                    new HttpErrorResponse({
                        status: 400,
                        error: { error: 'missing required columns' },
                    }),
            ),
        );
        select(csvFile());

        component.handleSubmit();
        fixture.detectChanges();

        expect(component.error()).toBe('missing required columns');
        expect(text('.csv-feedback')).toContain('missing required columns');
    });

    it('falls back to a generic error message', () => {
        itemService.importCsv.mockReturnValue(throwError(() => new Error('network')));
        select(csvFile());

        component.handleSubmit();

        expect(component.error()).toBe('Import failed. Confirm the CSV matches the template.');
    });

    it('clears the file, summary and error on reset', () => {
        select(new File(['x'], 'notes.txt'));
        component.handleReset();

        expect(component.selectedFile()).toBeNull();
        expect(component.summary()).toBeNull();
        expect(component.error()).toBeNull();
    });
});
