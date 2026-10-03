import { HttpErrorResponse } from '@angular/common/http';
import { Component, DestroyRef, inject, signal } from '@angular/core';

import { FormBuilder, ReactiveFormsModule, Validators } from '@angular/forms';
import { Router, RouterModule } from '@angular/router';
import { MatButtonModule } from '@angular/material/button';
import { MatCardModule } from '@angular/material/card';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';

import { ShelfService } from '../../../services/shelf.service';
import {
    PhotoUploadComponent,
    PhotoUploadResult,
} from '../../../components/shelves/photo-upload/photo-upload.component';
import { NotificationService } from '../../../services/notification.service';

@Component({
    selector: 'app-add-shelf-page',
    standalone: true,
    imports: [
        ReactiveFormsModule,
        RouterModule,
        MatButtonModule,
        MatCardModule,
        MatFormFieldModule,
        MatInputModule,
        PhotoUploadComponent,
    ],
    templateUrl: './add-shelf-page.component.html',
    styleUrl: './add-shelf-page.component.scss',
})
export class AddShelfPageComponent {
    private readonly shelfService = inject(ShelfService);
    private readonly notification = inject(NotificationService);
    private readonly router = inject(Router);
    private readonly fb = inject(FormBuilder);
    private readonly destroyRef = inject(DestroyRef);

    readonly creating = signal(false);
    readonly photoTouched = signal(false);

    readonly form = this.fb.group({
        name: ['', [Validators.required]],
        photoUrl: ['', [Validators.required]],
        description: [''],
    });

    createShelf(): void {
        this.photoTouched.set(true);
        if (this.form.invalid) {
            this.form.markAllAsTouched();
            return;
        }

        this.creating.set(true);
        const payload = this.form.value as { name: string; photoUrl: string; description?: string };
        this.shelfService
            .create(payload)
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe({
                next: (shelf) => {
                    this.creating.set(false);
                    this.notification.success('Shelf created');
                    this.resetForm();
                    this.router.navigate(['/shelves', shelf.shelf.id]);
                },
                error: (error: unknown) => {
                    this.creating.set(false);
                    const duplicate = error instanceof HttpErrorResponse && error.status === 409;
                    this.notification.error(
                        duplicate
                            ? 'A shelf with this name already exists'
                            : 'Could not create shelf',
                    );
                },
            });
    }

    handlePhotoSelected(result: PhotoUploadResult): void {
        this.form.patchValue({ photoUrl: result.dataUrl });
        this.photoTouched.set(true);
    }

    handlePhotoCleared(): void {
        this.form.patchValue({ photoUrl: '' });
    }

    private resetForm(): void {
        this.form.reset({ name: '', photoUrl: '', description: '' });
        this.photoTouched.set(false);
    }
}
