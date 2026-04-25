import { TestBed } from '@angular/core/testing';
import { MatSnackBar, MatSnackBarModule } from '@angular/material/snack-bar';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';

import { NotificationService } from './notification.service';

describe('NotificationService', () => {
    let service: NotificationService;
    let snackBarSpy: { open: ReturnType<typeof vi.fn> };

    beforeEach(() => {
        snackBarSpy = {
            open: vi.fn().mockName('MatSnackBar.open'),
        };
        snackBarSpy.open.mockReturnValue({} as any);

        TestBed.configureTestingModule({
            imports: [MatSnackBarModule, NoopAnimationsModule],
            providers: [NotificationService, { provide: MatSnackBar, useValue: snackBarSpy }],
        });

        service = TestBed.inject(NotificationService);
    });

    it('should be created', () => {
        expect(service).toBeTruthy();
    });

    describe('show', () => {
        it('should open snackbar with default config', () => {
            service.show('Test message');

            expect(snackBarSpy.open).toHaveBeenCalledWith(
                'Test message',
                'Dismiss',
                expect.objectContaining({
                    duration: 4000,
                    horizontalPosition: 'center',
                    verticalPosition: 'bottom',
                }),
            );
        });

        it('should use custom duration when provided', () => {
            service.show('Test message', { duration: 2000 });

            expect(snackBarSpy.open).toHaveBeenCalledWith(
                'Test message',
                'Dismiss',
                expect.objectContaining({ duration: 2000 }),
            );
        });

        it('should use custom action when provided', () => {
            service.show('Test message', { action: 'Undo' });

            expect(snackBarSpy.open).toHaveBeenCalledWith(
                'Test message',
                'Undo',
                expect.any(Object),
            );
        });
    });

    describe('success', () => {
        it('should add success panel class', () => {
            service.success('Item saved');

            expect(snackBarSpy.open).toHaveBeenCalledWith(
                'Item saved',
                'Dismiss',
                expect.objectContaining({
                    panelClass: ['notification-success'],
                }),
            );
        });
    });

    describe('error', () => {
        it('should add error panel class and longer duration', () => {
            service.error('Something went wrong');

            expect(snackBarSpy.open).toHaveBeenCalledWith(
                'Something went wrong',
                'Dismiss',
                expect.objectContaining({
                    duration: 5000,
                    panelClass: ['notification-error'],
                }),
            );
        });
    });

    describe('warn', () => {
        it('should add warn panel class', () => {
            service.warn('Duplicate check failed');

            expect(snackBarSpy.open).toHaveBeenCalledWith(
                'Duplicate check failed',
                'Dismiss',
                expect.objectContaining({
                    panelClass: ['notification-warn'],
                }),
            );
        });
    });

    describe('info', () => {
        it('should use default styling', () => {
            service.info('FYI message');

            expect(snackBarSpy.open).toHaveBeenCalledWith(
                'FYI message',
                'Dismiss',
                expect.objectContaining({
                    duration: 4000,
                }),
            );
        });
    });
});
