import { TestBed } from '@angular/core/testing';

import { BarcodeScannerService } from './barcode-scanner.service';
import { ScanFeedbackService } from './scan-feedback.service';

describe('BarcodeScannerService', () => {
    const originalBarcodeDetector = (globalThis as any).BarcodeDetector;
    const originalMediaDevicesDescriptor =
        Object.getOwnPropertyDescriptor(navigator, 'mediaDevices') ??
        Object.getOwnPropertyDescriptor(Object.getPrototypeOf(navigator), 'mediaDevices');
    const originalRequestAnimationFrame = globalThis.requestAnimationFrame;
    let hadOwnMediaDevices = false;

    beforeEach(() => {
        TestBed.configureTestingModule({
            providers: [
                BarcodeScannerService,
                {
                    provide: ScanFeedbackService,
                    useValue: {
                        playScanSuccess: () => undefined,
                        playScanFailure: () => undefined,
                    },
                },
            ],
        });

        (globalThis as any).BarcodeDetector = class MockBarcodeDetector {
            static async getSupportedFormats(): Promise<string[]> {
                return ['ean_13'];
            }

            detect(): Promise<
                Array<{
                    rawValue?: string;
                }>
            > {
                return Promise.resolve([]);
            }
        };

        const mockMediaDevices = {
            getUserMedia: async () =>
                ({
                    getTracks: () => [{ stop: () => undefined }],
                }) as MediaStream,
        };

        hadOwnMediaDevices = Object.prototype.hasOwnProperty.call(navigator, 'mediaDevices');
        Object.defineProperty(navigator, 'mediaDevices', {
            configurable: true,
            get: () => mockMediaDevices,
        });

        globalThis.requestAnimationFrame = () => 1;
    });

    afterEach(() => {
        vi.restoreAllMocks();
        (globalThis as any).BarcodeDetector = originalBarcodeDetector;
        if (!hadOwnMediaDevices) {
            delete (navigator as any).mediaDevices;
        } else if (originalMediaDevicesDescriptor) {
            Object.defineProperty(navigator, 'mediaDevices', originalMediaDevicesDescriptor);
        }
        globalThis.requestAnimationFrame = originalRequestAnimationFrame;
    });

    it('clears initial support status after camera starts', async () => {
        const service = TestBed.inject(BarcodeScannerService);
        const video = document.createElement('video');

        Object.defineProperty(video, 'srcObject', {
            configurable: true,
            writable: true,
            value: null,
        });

        vi.spyOn(video, 'play').mockResolvedValue();
        if (!video.pause) {
            (video as any).pause = () => undefined;
        }
        vi.spyOn(video, 'pause').mockImplementation(() => undefined);

        await service.startScanner(video, () => undefined);

        expect(service.scannerActive()).toBe(true);
        expect(service.scannerSupported()).toBe(true);
        expect(service.scannerStatus()).toBeNull();
        expect(service.scannerHint()).toContain('Align an ISBN barcode');
    });

    it('preserves start-up error when camera access fails', async () => {
        const service = TestBed.inject(BarcodeScannerService);
        const video = document.createElement('video');
        const consoleSpy = vi.spyOn(console, 'error');

        Object.defineProperty(video, 'srcObject', {
            configurable: true,
            writable: true,
            value: null,
        });

        Object.defineProperty(navigator, 'mediaDevices', {
            configurable: true,
            get: () => ({
                getUserMedia: async () => {
                    throw new Error('permission denied');
                },
            }),
        });

        await service.startScanner(video, () => undefined);

        expect(service.scannerActive()).toBe(false);
        expect(service.scannerError()).toContain('Camera access failed');
        expect(consoleSpy).toHaveBeenCalledWith(
            'Unable to start barcode scanner',
            expect.any(Error),
        );
    });

    it('clears processing status when scan completes', () => {
        const service = TestBed.inject(BarcodeScannerService);

        (service as any).beginProcessing('9781234567890');
        expect(service.scannerStatus()).toContain('Processing');

        service.reportScanComplete();
        expect(service.scannerStatus()).toBeNull();
    });
});
