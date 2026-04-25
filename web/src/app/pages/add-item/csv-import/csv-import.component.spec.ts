import { ComponentFixture, TestBed } from '@angular/core/testing';

import { CsvImportComponent } from './csv-import.component';

describe('CsvImportComponent', () => {
    let component: CsvImportComponent;
    let fixture: ComponentFixture<CsvImportComponent>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [CsvImportComponent],
        }).compileComponents();

        fixture = TestBed.createComponent(CsvImportComponent);
        component = fixture.componentInstance;
        component.csvFields = ['title', 'creator'];
        component.csvTemplateUrl = '/template.csv';
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should display csv fields', () => {
        const fields = fixture.nativeElement.querySelectorAll('.field');
        expect(fields.length).toBe(2);
        expect(fields[0].textContent.trim()).toBe('title');
        expect(fields[1].textContent.trim()).toBe('creator');
    });

    it('should emit fileSelected when a valid file is selected', () => {
        const spy = vi.spyOn(component.fileSelected, 'emit');
        const file = new File(['test'], 'test.csv', { type: 'text/csv' });
        const event = { target: { files: [file] } } as unknown as Event;

        component.handleFileChange(event);

        expect(spy).toHaveBeenCalledWith(file);
        expect(component.selectedFile()).toBe(file);
    });

    it('should emit importSubmit when form is submitted with a file', () => {
        const spy = vi.spyOn(component.importSubmit, 'emit');
        const file = new File(['test'], 'test.csv', { type: 'text/csv' });
        component.selectedFile.set(file);

        component.handleSubmit();

        expect(spy).toHaveBeenCalled();
    });

    it('should emit cleared when handleReset is called', () => {
        const spy = vi.spyOn(component.cleared, 'emit');
        component.selectedFile.set(new File(['test'], 'test.csv'));

        component.handleReset();

        expect(spy).toHaveBeenCalled();
        expect(component.selectedFile()).toBeNull();
    });

    it('should display import summary when provided', () => {
        component.importSummary = {
            totalRows: 10,
            imported: 8,
            skippedDuplicates: [{ row: 2, title: 'Duplicate', reason: 'Already exists' }],
            failed: [{ row: 5, title: 'Failed', error: 'Invalid data' }],
        };
        fixture.detectChanges();

        const summary = fixture.nativeElement.querySelector('.csv-summary');
        expect(summary).toBeTruthy();
        expect(summary.textContent).toContain('Imported 8 of 10 rows');
    });
});
