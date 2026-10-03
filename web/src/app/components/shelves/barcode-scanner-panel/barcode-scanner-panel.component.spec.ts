import { signal } from '@angular/core';
import { ComponentFixture, TestBed } from '@angular/core/testing';

import { BarcodeScannerPanelComponent } from './barcode-scanner-panel.component';
import { BarcodeScannerService } from '../../../services/barcode-scanner.service';

describe('BarcodeScannerPanelComponent', () => {
    let fixture: ComponentFixture<BarcodeScannerPanelComponent>;
    let scanner: {
        scannerSupported: ReturnType<typeof signal<boolean | null>>;
        scannerActive: ReturnType<typeof signal<boolean>>;
        scannerReady: ReturnType<typeof signal<boolean>>;
        scannerStatus: ReturnType<typeof signal<string | null>>;
        scannerError: ReturnType<typeof signal<string | null>>;
        scannerHint: ReturnType<typeof signal<string | null>>;
        scannerProcessing: ReturnType<typeof signal<boolean>>;
        scannerFlash: ReturnType<typeof signal<boolean>>;
        startScanner: ReturnType<typeof vi.fn>;
        stopScanner: ReturnType<typeof vi.fn>;
        reportScanComplete: ReturnType<typeof vi.fn>;
    };

    function query(selector: string): HTMLElement | null {
        return fixture.nativeElement.querySelector(selector);
    }

    beforeEach(async () => {
        scanner = {
            scannerSupported: signal<boolean | null>(null),
            scannerActive: signal(false),
            scannerReady: signal(false),
            scannerStatus: signal<string | null>(null),
            scannerError: signal<string | null>(null),
            scannerHint: signal<string | null>(null),
            scannerProcessing: signal(false),
            scannerFlash: signal(false),
            startScanner: vi.fn().mockResolvedValue(undefined),
            stopScanner: vi.fn(),
            reportScanComplete: vi.fn(),
        };

        await TestBed.configureTestingModule({
            imports: [BarcodeScannerPanelComponent],
            providers: [{ provide: BarcodeScannerService, useValue: scanner }],
        }).compileComponents();

        fixture = TestBed.createComponent(BarcodeScannerPanelComponent);
        fixture.detectChanges();
    });

    it('shows the default shelf instructions', () => {
        expect(query('.scanner-instructions')?.textContent?.trim()).toBe(
            'Scan ISBN barcodes to automatically add and place items in this slot.',
        );
    });

    it('shows custom instructions', () => {
        fixture.componentRef.setInput('instructions', 'Custom instructions.');
        fixture.detectChanges();

        expect(query('.scanner-instructions')?.textContent?.trim()).toBe('Custom instructions.');
    });

    it('shows the scanner status', () => {
        scanner.scannerStatus.set('Starting camera...');
        fixture.detectChanges();

        expect(query('.scanner-status')?.textContent?.trim()).toBe('Starting camera...');
    });

    it('shows the hint unless there is an error', () => {
        scanner.scannerHint.set('Align an ISBN barcode within the frame.');
        fixture.detectChanges();
        expect(query('.scanner-hint')?.textContent?.trim()).toBe(
            'Align an ISBN barcode within the frame.',
        );

        scanner.scannerError.set('Camera access denied');
        fixture.detectChanges();
        expect(query('.scanner-hint')).toBeNull();
        expect(query('.scanner-error')?.textContent?.trim()).toBe('Camera access denied');
    });

    it('shows the scan frame only once the camera is ready', () => {
        scanner.scannerActive.set(true);
        fixture.detectChanges();
        expect(query('.scanner-frame')).toBeNull();

        scanner.scannerReady.set(true);
        fixture.detectChanges();
        expect(query('.scanner-frame')).toBeTruthy();
    });

    it('shows the busy and flash overlays', () => {
        scanner.scannerProcessing.set(true);
        scanner.scannerFlash.set(true);
        fixture.detectChanges();

        expect(query('.busy-indicator')).toBeTruthy();
        expect(query('.scanner-overlay--flash')).toBeTruthy();
    });

    it('starts the scanner when activated and emits scanned barcodes', async () => {
        vi.useFakeTimers();
        const scrollIntoView = vi.fn();
        const section = query('.scanner-section') as HTMLElement;
        section.scrollIntoView = scrollIntoView;
        try {
            const emitted: string[] = [];
            fixture.componentInstance.barcodeScanned.subscribe((value) => emitted.push(value));
            fixture.componentRef.setInput('active', true);
            fixture.detectChanges();

            await vi.advanceTimersByTimeAsync(100);

            expect(scrollIntoView).toHaveBeenCalled();
            expect(scanner.startScanner).toHaveBeenCalledTimes(1);
            const [video, onScan] = scanner.startScanner.mock.calls[0];
            expect((video as HTMLVideoElement).tagName.toLowerCase()).toBe('video');

            onScan({ rawValue: '9781234567890' });
            expect(emitted).toEqual(['9781234567890']);
        } finally {
            vi.useRealTimers();
        }
    });

    it('does not start the scanner when deactivated before the start delay', async () => {
        vi.useFakeTimers();
        try {
            fixture.componentRef.setInput('active', true);
            fixture.detectChanges();
            fixture.componentRef.setInput('active', false);
            fixture.detectChanges();

            await vi.advanceTimersByTimeAsync(200);

            expect(scanner.startScanner).not.toHaveBeenCalled();
        } finally {
            vi.useRealTimers();
        }
    });

    it('does not start the scanner when destroyed before the start delay', async () => {
        vi.useFakeTimers();
        try {
            fixture.componentRef.setInput('active', true);
            fixture.detectChanges();
            fixture.destroy();

            await vi.advanceTimersByTimeAsync(200);

            expect(scanner.startScanner).not.toHaveBeenCalled();
        } finally {
            vi.useRealTimers();
        }
    });

    it('emits scannerFailed when the scanner stops with an error after starting', async () => {
        const failures: string[] = [];
        fixture.componentInstance.scannerFailed.subscribe((message) => failures.push(message));
        scanner.startScanner.mockImplementation(async () => {
            scanner.scannerError.set('Camera access failed.');
        });
        (query('.scanner-section') as HTMLElement).scrollIntoView = vi.fn();

        await fixture.componentInstance.startScanner();
        fixture.detectChanges();

        expect(failures).toEqual(['Camera access failed.']);
    });

    it('does not emit scannerFailed for errors it did not start', () => {
        const failures: string[] = [];
        fixture.componentInstance.scannerFailed.subscribe((message) => failures.push(message));

        scanner.scannerError.set('Some error');
        fixture.detectChanges();

        expect(failures).toEqual([]);
    });

    it('stops the scanner when destroyed', () => {
        fixture.destroy();

        expect(scanner.stopScanner).toHaveBeenCalled();
    });
});
