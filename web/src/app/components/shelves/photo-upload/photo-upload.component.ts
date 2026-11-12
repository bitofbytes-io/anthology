import {
    Component,
    EventEmitter,
    Input,
    Output,
    signal,
    ChangeDetectionStrategy,
} from '@angular/core';

import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

export interface PhotoUploadResult {
    dataUrl: string;
    fileName: string;
}

@Component({
    selector: 'app-photo-upload',
    standalone: true,
    imports: [MatButtonModule, MatIconModule],
    templateUrl: './photo-upload.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './photo-upload.component.scss',
})
export class PhotoUploadComponent {
    private static readonly MAX_PHOTO_BYTES = 5 * 1024 * 1024;
    private static readonly ALLOWED_IMAGE_TYPES = [
        'image/jpeg',
        'image/png',
        'image/gif',
        'image/webp',
        'image/svg+xml',
    ];
    private static readonly ALLOWED_IMAGE_EXTENSIONS = [
        '.jpg',
        '.jpeg',
        '.png',
        '.gif',
        '.webp',
        '.svg',
    ];

    @Input() label = 'Photo';
    @Input() hint = 'Upload a JPG or PNG up to 5MB.';
    @Input() disabled = false;
    @Input() required = false;
    @Input() touched = false;

    @Output() photoSelected = new EventEmitter<PhotoUploadResult>();
    @Output() photoCleared = new EventEmitter<void>();

    readonly uploadError = signal<string | null>(null);
    readonly selectedFileName = signal<string | null>(null);

    get hasError(): boolean {
        return this.required && this.touched && !this.selectedFileName() && !this.uploadError();
    }

    get showRequiredError(): boolean {
        return this.required && this.touched && !this.selectedFileName() && !this.uploadError();
    }

    openFilePicker(input: HTMLInputElement): void {
        this.uploadError.set(null);
        input.click();
    }

    handleFileChange(input: HTMLInputElement): void {
        const file = input.files?.[0];
        if (!file) {
            return;
        }

        const validationError = this.validateImageFile(file);
        if (validationError) {
            this.clearSelection(false, input);
            this.uploadError.set(validationError);
            return;
        }

        const reader = new FileReader();
        reader.onload = () => {
            const result = reader.result as string;
            this.uploadError.set(null);
            this.selectedFileName.set(file.name);
            this.photoSelected.emit({ dataUrl: result, fileName: file.name });
        };
        reader.readAsDataURL(file);
    }

    clearSelection(clearError = true, input?: HTMLInputElement): void {
        if (clearError) {
            this.uploadError.set(null);
        }
        this.selectedFileName.set(null);
        if (input) {
            input.value = '';
        }
        this.photoCleared.emit();
    }

    private validateImageFile(file: File): string | null {
        const fileName = file.name.toLowerCase();
        const hasValidExtension = PhotoUploadComponent.ALLOWED_IMAGE_EXTENSIONS.some((ext) =>
            fileName.endsWith(ext),
        );
        if (!hasValidExtension) {
            return 'Only image files (JPEG, PNG, GIF, WebP, SVG) are allowed.';
        }

        if (file.type && !PhotoUploadComponent.ALLOWED_IMAGE_TYPES.includes(file.type)) {
            return 'Only image files (JPEG, PNG, GIF, WebP, SVG) are allowed.';
        }

        if (file.size > PhotoUploadComponent.MAX_PHOTO_BYTES) {
            return 'Photos must be under 5MB.';
        }

        return null;
    }
}
