import {
    Component,
    ElementRef,
    EventEmitter,
    Input,
    Output,
    ViewChild,
    ChangeDetectionStrategy,
} from '@angular/core';
import { FormGroup, ReactiveFormsModule } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';

@Component({
    selector: 'app-cover-section',
    standalone: true,
    imports: [
        MatButtonModule,
        MatFormFieldModule,
        MatIconModule,
        MatInputModule,
        ReactiveFormsModule,
    ],
    templateUrl: './cover-section.component.html',
    changeDetection: ChangeDetectionStrategy.Eager,
    styleUrl: './cover-section.component.scss',
})
export class CoverSectionComponent {
    private static readonly MAX_COVER_BYTES = 500 * 1024;
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

    @ViewChild('coverInput') coverInput?: ElementRef<HTMLInputElement>;

    @Input({ required: true }) form!: FormGroup;
    @Input() coverImageError: string | null = null;

    @Output() readonly coverErrorCleared = new EventEmitter<void>();
    @Output() readonly coverErrorSet = new EventEmitter<string>();

    clearCoverImage(): void {
        this.form.patchValue({ coverImage: '' });
        this.coverErrorCleared.emit();
        this.resetCoverInput();
    }

    clearCoverError(): void {
        this.coverErrorCleared.emit();
    }

    openCoverFilePicker(): void {
        this.coverErrorCleared.emit();
        this.coverInput?.nativeElement?.click();
    }

    handleCoverFileChange(event: Event): void {
        const input = event.target as HTMLInputElement | null;
        const file = input?.files?.[0];
        if (!file) {
            return;
        }

        const validationError = this.validateImageFile(
            file,
            CoverSectionComponent.MAX_COVER_BYTES,
            '500KB',
        );
        if (validationError) {
            this.coverErrorSet.emit(validationError);
            this.resetCoverInput();
            return;
        }

        const reader = new FileReader();
        reader.onload = () => {
            const result = reader.result as string;
            this.form.patchValue({ coverImage: result });
            this.coverErrorCleared.emit();
        };
        reader.readAsDataURL(file);
    }

    private validateImageFile(file: File, maxBytes: number, maxSizeLabel: string): string | null {
        const fileName = file.name.toLowerCase();
        const hasValidExtension = CoverSectionComponent.ALLOWED_IMAGE_EXTENSIONS.some((ext) =>
            fileName.endsWith(ext),
        );
        if (!hasValidExtension) {
            return 'Only image files (JPEG, PNG, GIF, WebP, SVG) are allowed.';
        }

        if (file.type && !CoverSectionComponent.ALLOWED_IMAGE_TYPES.includes(file.type)) {
            return 'Only image files (JPEG, PNG, GIF, WebP, SVG) are allowed.';
        }

        if (file.size > maxBytes) {
            return `Cover images must be under ${maxSizeLabel}.`;
        }

        return null;
    }

    private resetCoverInput(): void {
        if (this.coverInput?.nativeElement) {
            this.coverInput.nativeElement.value = '';
        }
    }
}
