import { Injectable, signal, inject } from '@angular/core';
import { ScanFeedback } from './scan-feedback';
import { ScanFeedbackService } from './scan-feedback.service';
import { waitForVideoReady } from './video-ready';
import { ZxingDetector } from './zxing-detector.service';

type SupportedBarcodeFormat = 'ean_13' | 'ean_8' | 'code_128' | 'upc_a' | 'upc_e';

interface BarcodeDetectionResult {
    rawValue?: string;
}

interface BarcodeDetectorOptions {
    formats?: SupportedBarcodeFormat[];
}

interface BarcodeDetector {
    detect(source: ImageBitmapSource): Promise<BarcodeDetectionResult[]>;
}

interface BarcodeDetectorConstructor {
    new (options?: BarcodeDetectorOptions): BarcodeDetector;
    getSupportedFormats(): Promise<SupportedBarcodeFormat[]>;
}

declare const BarcodeDetector: BarcodeDetectorConstructor;

export interface BarcodeScanResult {
    rawValue: string;
}

export type ScannerMode = 'native' | 'zxing';

const READ_FAILURE_MESSAGE =
    'Unable to read the barcode. Try adjusting lighting or typing the code.';

@Injectable({
    providedIn: 'root',
})
export class BarcodeScannerService {
    private readonly preferredBarcodeFormats: SupportedBarcodeFormat[] = [
        'ean_13',
        'ean_8',
        'code_128',
        'upc_a',
        'upc_e',
    ];

    private barcodeDetector: BarcodeDetector | null = null;
    private scanStream: MediaStream | null = null;
    private scanFrameId: number | null = null;
    private scannerMode: ScannerMode | null = null;
    private videoElement: HTMLVideoElement | null = null;
    private onScanCallback: ((result: BarcodeScanResult) => void) | null = null;

    private readonly scanSound = inject(ScanFeedbackService);
    private readonly zxing = inject(ZxingDetector);
    private readonly feedback = new ScanFeedback();

    readonly scannerActive = signal(false);
    readonly scannerStatus = this.feedback.status;
    readonly scannerError = this.feedback.error;
    readonly scannerSupported = signal<boolean | null>(null);
    readonly scannerHint = this.feedback.hint;
    readonly scannerProcessing = this.feedback.processing;
    readonly scannerFlash = this.feedback.flash;
    readonly scannerReady = signal(false);

    async startScanner(
        videoElement: HTMLVideoElement,
        onScan: (result: BarcodeScanResult) => void,
    ): Promise<void> {
        if (this.scannerActive()) {
            return;
        }

        if (typeof navigator === 'undefined' || !navigator.mediaDevices?.getUserMedia) {
            this.scannerSupported.set(false);
            this.scannerError.set('Camera access is not available in this browser.');
            return;
        }

        this.videoElement = videoElement;
        this.onScanCallback = onScan;
        this.scannerProcessing.set(false);
        this.scannerReady.set(false);
        this.feedback.clearMessages();
        this.scannerStatus.set('Checking camera support...');
        this.scannerActive.set(true);

        const scannerMode = await this.resolveBarcodeScanner();
        if (!this.scannerActive()) {
            return;
        }

        if (!scannerMode) {
            this.scannerActive.set(false);
            this.scannerStatus.set(null);
            return;
        }

        try {
            if (!this.scannerActive()) {
                return;
            }

            this.scannerStatus.set('Starting camera...');
            const stream = await navigator.mediaDevices.getUserMedia({
                video: { facingMode: 'environment' },
                audio: false,
            });

            if (!this.scannerActive()) {
                stream.getTracks().forEach((track) => track.stop());
                return;
            }

            this.scanStream = stream;
            this.videoElement.srcObject = this.scanStream;
            await this.videoElement.play();
            await waitForVideoReady(this.videoElement);

            if (!this.scannerActive()) {
                this.stopScannerStream();
                this.videoElement.pause();
                this.videoElement.srcObject = null;
                return;
            }

            this.scannerMode = scannerMode;
            this.scannerReady.set(true);
            this.scannerHint.set('Align an ISBN barcode within the frame.');
            this.scannerStatus.set(null);

            if (scannerMode === 'native') {
                this.scheduleNextScan();
            } else {
                this.startZxingDetection();
            }
        } catch (error) {
            console.error('Unable to start barcode scanner', error);
            this.scannerError.set('Camera access failed. Confirm permissions and try again.');
            this.scannerStatus.set(null);
            this.stopScanner({ preserveError: true });
        }
    }

    stopScanner(
        options: { preserveError?: boolean; preserveHint?: boolean; preserveStatus?: boolean } = {},
    ): void {
        this.scannerActive.set(false);
        this.scannerReady.set(false);
        this.feedback.reset(options);
        this.clearScannerAnimation();
        this.zxing.stop();
        this.stopScannerStream();
        this.scannerMode = null;
        this.onScanCallback = null;

        if (this.videoElement) {
            this.videoElement.pause();
            this.videoElement.srcObject = null;
            this.videoElement = null;
        }
    }

    reportScanSuccess(title: string, clearAfterMs = 2500): void {
        this.feedback.reportSuccess(title, clearAfterMs);
    }

    reportScanFailure(message: string, clearAfterMs = 4000): void {
        this.feedback.reportFailure(message, clearAfterMs);
    }

    reportScanComplete(): void {
        this.feedback.reportComplete();
    }

    clearScanMessage(): void {
        this.feedback.clearMessages();
    }

    private scheduleNextScan(): void {
        this.scanFrameId = requestAnimationFrame(() => {
            void this.detectBarcodeFrame();
        });
    }

    private async detectBarcodeFrame(): Promise<void> {
        if (!this.scannerActive() || !this.barcodeDetector || !this.videoElement) {
            return;
        }

        if (this.videoElement.readyState < HTMLMediaElement.HAVE_ENOUGH_DATA) {
            this.scheduleNextScan();
            return;
        }

        try {
            const codes = await this.barcodeDetector.detect(this.videoElement);
            const found = codes.find((code: BarcodeDetectionResult) =>
                (code.rawValue ?? '').trim(),
            );

            if (found?.rawValue) {
                this.handleDetectedValue(found.rawValue);
                this.scheduleNextScan();
                return;
            }
        } catch (error) {
            this.failDetection(error);
            return;
        }

        this.scheduleNextScan();
    }

    private startZxingDetection(): void {
        const started =
            !!this.videoElement &&
            this.zxing.start(
                this.videoElement,
                (value) => this.handleDetectedValue(value),
                (error) => this.failDetection(error),
            );

        if (!started) {
            this.scannerError.set('Barcode scanning is not available on this device.');
            this.stopScanner({ preserveError: true });
        }
    }

    private handleDetectedValue(rawValue: string): void {
        const value = rawValue.trim();
        if (!this.feedback.beginProcessing(value)) {
            return;
        }

        this.scanSound.playScanSuccess();
        this.onScanCallback?.({ rawValue: value });
    }

    private failDetection(error: unknown): void {
        console.error('Barcode detection failed', error);
        this.scannerError.set(READ_FAILURE_MESSAGE);
        this.stopScanner({ preserveError: true });
    }

    private stopScannerStream(): void {
        if (this.scanStream) {
            this.scanStream.getTracks().forEach((track) => track.stop());
            this.scanStream = null;
        }
    }

    private clearScannerAnimation(): void {
        if (this.scanFrameId !== null) {
            cancelAnimationFrame(this.scanFrameId);
            this.scanFrameId = null;
        }
    }

    private async resolveBarcodeScanner(): Promise<ScannerMode | null> {
        if (typeof window === 'undefined') {
            this.scannerSupported.set(false);
            this.scannerError.set('Barcode scanning is not supported in this browser.');
            this.scannerStatus.set(null);
            return null;
        }

        if (typeof BarcodeDetector !== 'undefined') {
            try {
                const supportedFormats = await BarcodeDetector.getSupportedFormats();
                const availableFormats = this.preferredBarcodeFormats.filter((format) =>
                    supportedFormats.includes(format),
                );

                if (availableFormats.length) {
                    this.barcodeDetector = new BarcodeDetector({ formats: availableFormats });
                    this.scannerSupported.set(true);
                    return 'native';
                }
            } catch (error) {
                console.warn(
                    'Native barcode detector unavailable, falling back to library.',
                    error,
                );
            }
        }

        try {
            this.zxing.prepare();
            this.scannerSupported.set(true);
            return 'zxing';
        } catch (error) {
            console.error('Barcode scanner unavailable', error);
            this.scannerSupported.set(false);
            this.scannerError.set('Barcode scanning is not available on this device.');
            this.scannerStatus.set(null);
            return null;
        }
    }
}
