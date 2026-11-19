import { Injectable } from '@angular/core';
import { BrowserMultiFormatReader, IScannerControls } from '@zxing/browser';
import {
    BarcodeFormat,
    DecodeHintType,
    Exception,
    NotFoundException,
    Result,
} from '@zxing/library';

/** ZXing-based barcode detection used when the native BarcodeDetector is unavailable. */
@Injectable({
    providedIn: 'root',
})
export class ZxingDetector {
    private reader: BrowserMultiFormatReader | null = null;
    private controls: IScannerControls | null = null;

    /** Creates the reader. Throws if the ZXing library cannot be initialised. */
    prepare(): void {
        const hints = new Map();
        hints.set(DecodeHintType.POSSIBLE_FORMATS, [
            BarcodeFormat.EAN_13,
            BarcodeFormat.EAN_8,
            BarcodeFormat.CODE_128,
            BarcodeFormat.UPC_A,
            BarcodeFormat.UPC_E,
        ]);

        this.reader = new BrowserMultiFormatReader(hints);
    }

    /**
     * Starts decoding frames from the video element. Returns false when the reader
     * has not been prepared. `onValue` receives each trimmed decoded value and
     * `onError` any failure other than "no barcode in this frame".
     */
    start(
        video: HTMLVideoElement,
        onValue: (value: string) => void,
        onError: (error: unknown) => void,
    ): boolean {
        if (!this.reader) {
            return false;
        }

        this.reader
            .decodeFromVideoElement(
                video,
                (
                    result: Result | null | undefined,
                    error: Exception | null | undefined,
                    controls,
                ) => {
                    this.controls = controls;

                    if (result?.getText()) {
                        onValue(result.getText().trim());
                        return;
                    }

                    if (error && !(error instanceof NotFoundException)) {
                        onError(error);
                    }
                },
            )
            .catch(onError);

        return true;
    }

    stop(): void {
        if (this.controls) {
            this.controls.stop();
            this.controls = null;
        }

        this.reader = null;
    }
}
