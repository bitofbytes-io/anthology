import { ComponentFixture, TestBed } from '@angular/core/testing';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';

import {
    DuplicateDialogComponent,
    DuplicateDialogData,
    DuplicateDialogResult,
} from './duplicate-dialog.component';

describe('DuplicateDialogComponent', () => {
    let component: DuplicateDialogComponent;
    let fixture: ComponentFixture<DuplicateDialogComponent>;
    let dialogRefSpy: { close: ReturnType<typeof vi.fn> };

    const mockDialogData: DuplicateDialogData = {
        duplicates: [
            {
                id: '123e4567-e89b-12d3-a456-426614174000',
                title: 'Test Book',
                primaryIdentifier: '9780123456789',
                identifierType: 'ISBN-13',
                coverUrl: 'https://example.com/cover.jpg',
                location: 'Shelf A',
                updatedAt: '2024-01-15T10:30:00Z',
            },
        ],
        totalCount: 1,
    };

    beforeEach(async () => {
        dialogRefSpy = {
            close: vi.fn().mockName('MatDialogRef.close'),
        };

        await TestBed.configureTestingModule({
            imports: [DuplicateDialogComponent, NoopAnimationsModule],
            providers: [
                { provide: MatDialogRef, useValue: dialogRefSpy },
                { provide: MAT_DIALOG_DATA, useValue: mockDialogData },
            ],
        }).compileComponents();

        fixture = TestBed.createComponent(DuplicateDialogComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should display duplicate items', () => {
        const compiled = fixture.nativeElement as HTMLElement;
        expect(compiled.querySelector('.duplicate-title')?.textContent).toContain('Test Book');
    });

    it('should close dialog with "add" when Add Anyway is clicked', () => {
        component.handleAddAnyway();
        expect(dialogRefSpy.close).toHaveBeenCalledWith('add');
    });

    it('should close dialog with "cancel" when Cancel is clicked', () => {
        component.handleCancel();
        expect(dialogRefSpy.close).toHaveBeenCalledWith('cancel');
    });

    it('should format dates correctly', () => {
        const formatted = component.formatDate('2024-01-15T10:30:00Z');
        expect(formatted).toBeTruthy();
        expect(formatted).toContain('2024');
    });

    it('should not show more duplicates message when totalCount equals duplicates length', () => {
        expect(component.hasMoreDuplicates).toBe(false);
    });

    describe('with more duplicates', () => {
        beforeEach(async () => {
            const dataWithMore: DuplicateDialogData = {
                ...mockDialogData,
                totalCount: 7,
            };

            await TestBed.resetTestingModule()
                .configureTestingModule({
                    imports: [DuplicateDialogComponent, NoopAnimationsModule],
                    providers: [
                        { provide: MatDialogRef, useValue: dialogRefSpy },
                        { provide: MAT_DIALOG_DATA, useValue: dataWithMore },
                    ],
                })
                .compileComponents();

            fixture = TestBed.createComponent(DuplicateDialogComponent);
            component = fixture.componentInstance;
            fixture.detectChanges();
        });

        it('should show more duplicates message when totalCount exceeds duplicates length', () => {
            expect(component.hasMoreDuplicates).toBe(true);
            expect(component.additionalCount).toBe(6);
        });
    });
});
