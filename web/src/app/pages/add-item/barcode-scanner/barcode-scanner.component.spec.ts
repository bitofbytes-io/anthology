import { ComponentFixture, TestBed } from '@angular/core/testing';

import { BarcodeScannerComponent } from './barcode-scanner.component';

describe('BarcodeScannerComponent', () => {
    let component: BarcodeScannerComponent;
    let fixture: ComponentFixture<BarcodeScannerComponent>;

    beforeEach(async () => {
        await TestBed.configureTestingModule({
            imports: [BarcodeScannerComponent],
        }).compileComponents();

        fixture = TestBed.createComponent(BarcodeScannerComponent);
        component = fixture.componentInstance;
        fixture.detectChanges();
    });

    it('should create', () => {
        expect(component).toBeTruthy();
    });

    it('should hide scanner preview when not active', () => {
        component.scannerActive = false;
        fixture.detectChanges();

        const preview = fixture.nativeElement.querySelector('.scanner-preview');
        expect(preview.classList.contains('scanner-preview--visible')).toBe(false);
    });

    it('should show scanner preview when active', () => {
        component.scannerActive = true;
        fixture.detectChanges();

        const preview = fixture.nativeElement.querySelector('.scanner-preview');
        expect(preview.classList.contains('scanner-preview--visible')).toBe(true);
    });

    it('should show scanner status when active and status is set', () => {
        component.scannerActive = true;
        component.scannerStatus = 'Scanning...';
        fixture.detectChanges();

        const status = fixture.nativeElement.querySelector('.scanner-feedback .status');
        expect(status.textContent.trim()).toBe('Scanning...');
    });

    it('should show scanner error when active and error is set', () => {
        component.scannerActive = true;
        component.scannerError = 'Camera access denied';
        fixture.detectChanges();

        const error = fixture.nativeElement.querySelector('.scanner-feedback .error');
        expect(error.textContent.trim()).toBe('Camera access denied');
    });

    it('should show scanner frame when active and ready', () => {
        component.scannerActive = true;
        component.scannerReady = true;
        fixture.detectChanges();

        const frame = fixture.nativeElement.querySelector('.scanner-frame');
        expect(frame).toBeTruthy();
    });

    it('should show busy indicator when processing', () => {
        component.scannerActive = true;
        component.scannerProcessing = true;
        fixture.detectChanges();

        const busyIndicator = fixture.nativeElement.querySelector('.busy-indicator');
        expect(busyIndicator).toBeTruthy();
    });

    it('should show flash overlay when flash is true', () => {
        component.scannerActive = true;
        component.scannerFlash = true;
        fixture.detectChanges();

        const flash = fixture.nativeElement.querySelector('.scanner-overlay--flash');
        expect(flash).toBeTruthy();
    });

    it('should provide video element via getVideoElement', () => {
        const video = component.getVideoElement();
        expect(video).toBeTruthy();
        expect(video?.tagName.toLowerCase()).toBe('video');
    });
});
