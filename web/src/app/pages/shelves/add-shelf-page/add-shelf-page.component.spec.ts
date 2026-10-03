import { TestBed } from '@angular/core/testing';
import { HttpErrorResponse } from '@angular/common/http';
import { provideRouter } from '@angular/router';
import { throwError } from 'rxjs';
import { AddShelfPageComponent } from './add-shelf-page.component';
import { ShelfService } from '../../../services/shelf.service';
import { NotificationService } from '../../../services/notification.service';

describe('AddShelfPageComponent', () => {
    const service = { create: vi.fn() };
    const notification = { success: vi.fn(), error: vi.fn() };

    beforeEach(async () => {
        vi.clearAllMocks();
        await TestBed.configureTestingModule({
            imports: [AddShelfPageComponent],
            providers: [
                provideRouter([]),
                { provide: ShelfService, useValue: service },
                { provide: NotificationService, useValue: notification },
            ],
        }).compileComponents();
    });

    function submit(status: number): AddShelfPageComponent {
        service.create.mockReturnValue(throwError(() => new HttpErrorResponse({ status })));
        const page = TestBed.createComponent(AddShelfPageComponent).componentInstance;
        page.form.setValue({
            name: 'Living room',
            photoUrl: 'data:image/png;base64,AA',
            description: '',
        });
        page.createShelf();
        return page;
    }

    it('explains a duplicate shelf name', () => {
        const page = submit(409);
        expect(notification.error).toHaveBeenCalledExactlyOnceWith(
            'A shelf with this name already exists',
        );
        expect(page.creating()).toBe(false);
    });

    it('falls back to a generic message for other failures', () => {
        submit(500);
        expect(notification.error).toHaveBeenCalledExactlyOnceWith('Could not create shelf');
    });
});
