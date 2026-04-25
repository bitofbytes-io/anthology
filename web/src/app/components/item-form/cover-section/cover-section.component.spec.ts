import { TestBed } from '@angular/core/testing';
import { FormBuilder, FormGroup } from '@angular/forms';
import { NoopAnimationsModule } from '@angular/platform-browser/animations';

import { CoverSectionComponent } from './cover-section.component';

describe(CoverSectionComponent.name, () => {
    let form: FormGroup;

    beforeEach(async () => {
        const fb = new FormBuilder();
        form = fb.group({
            coverImage: [''],
        });

        await TestBed.configureTestingModule({
            imports: [CoverSectionComponent, NoopAnimationsModule],
        }).compileComponents();
    });

    function createComponent() {
        const fixture = TestBed.createComponent(CoverSectionComponent);
        fixture.componentInstance.form = form;
        fixture.detectChanges();
        return fixture;
    }

    it('creates the component', () => {
        const fixture = createComponent();
        expect(fixture.componentInstance).toBeTruthy();
    });

    it('clears the cover image when requested', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        form.patchValue({ coverImage: 'https://example.com/cover.jpg' });

        component.clearCoverImage();

        expect(form.get('coverImage')?.value).toBe('');
    });

    it('emits coverErrorCleared when clearing cover image', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        const clearedSpy = vi.fn();
        component.coverErrorCleared.subscribe(clearedSpy);
        form.patchValue({ coverImage: 'https://example.com/cover.jpg' });

        component.clearCoverImage();

        expect(clearedSpy).toHaveBeenCalled();
    });

    it('emits coverErrorCleared when clearing cover error', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        const clearedSpy = vi.fn();
        component.coverErrorCleared.subscribe(clearedSpy);

        component.clearCoverError();

        expect(clearedSpy).toHaveBeenCalled();
    });

    it('emits coverErrorCleared when opening file picker', () => {
        const fixture = createComponent();
        const component = fixture.componentInstance;
        const clearedSpy = vi.fn();
        component.coverErrorCleared.subscribe(clearedSpy);

        component.openCoverFilePicker();

        expect(clearedSpy).toHaveBeenCalled();
    });

    it('displays the cover image error when provided', () => {
        const fixture = createComponent();
        fixture.componentInstance.coverImageError = 'Image too large';
        fixture.detectChanges();

        const errorEl = fixture.nativeElement.querySelector('.cover-error');
        expect(errorEl?.textContent).toContain('Image too large');
    });
});
