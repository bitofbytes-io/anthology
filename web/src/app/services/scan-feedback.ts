import { signal } from '@angular/core';

/**
 * Status, error, hint, processing and flash UI state for the barcode scanner,
 * including the timers that clear them and the duplicate-detection debounce.
 */
export class ScanFeedback {
    static readonly DETECTION_DEBOUNCE_MS = 3000;
    static readonly FLASH_DURATION_MS = 640;

    readonly status = signal<string | null>(null);
    readonly error = signal<string | null>(null);
    readonly hint = signal<string | null>(null);
    readonly processing = signal(false);
    readonly flash = signal(false);

    private lastDetectedValue: string | null = null;
    private lastDetectedAt = 0;
    private statusClearTimer: number | null = null;
    private flashTimer: number | null = null;

    /**
     * Marks a detected value as being processed. Returns false when the value is
     * empty, a recent duplicate, or another detection is already in flight.
     */
    beginProcessing(value: string): boolean {
        if (!value) {
            return false;
        }

        const now = Date.now();
        if (
            this.lastDetectedValue === value &&
            now - this.lastDetectedAt < ScanFeedback.DETECTION_DEBOUNCE_MS
        ) {
            return false;
        }

        if (this.processing()) {
            return false;
        }

        this.lastDetectedValue = value;
        this.lastDetectedAt = now;
        this.processing.set(true);
        this.triggerFlash();
        this.error.set(null);
        this.status.set(`Found ${value}. Processing...`);
        this.clearStatusTimer();
        return true;
    }

    reportSuccess(title: string, clearAfterMs: number): void {
        const safeTitle = title.trim();
        this.processing.set(false);
        this.error.set(null);
        this.status.set(safeTitle ? `Found: ${safeTitle}` : 'Found item.');
        this.scheduleStatusClear(clearAfterMs, { clearError: true });
    }

    reportFailure(message: string, clearAfterMs: number): void {
        this.processing.set(false);
        this.error.set(message);
        this.scheduleStatusClear(clearAfterMs, { clearStatus: true, clearError: true });
    }

    reportComplete(): void {
        this.processing.set(false);
        this.error.set(null);
        this.status.set(null);
        this.clearStatusTimer();
    }

    /** Clears status, error and hint messages along with any pending timers. */
    clearMessages(): void {
        this.clearStatusTimer();
        this.clearFlashTimer();
        this.status.set(null);
        this.error.set(null);
        this.hint.set(null);
    }

    /** Clears every message and the processing flag, then restores the preserved ones. */
    reset(
        options: { preserveError?: boolean; preserveHint?: boolean; preserveStatus?: boolean } = {},
    ): void {
        const preservedError = options.preserveError ? this.error() : null;
        const preservedHint = options.preserveHint ? this.hint() : null;
        const preservedStatus = options.preserveStatus ? this.status() : null;

        this.processing.set(false);
        this.clearMessages();

        if (preservedStatus) {
            this.status.set(preservedStatus);
        }
        if (preservedHint) {
            this.hint.set(preservedHint);
        }
        if (preservedError) {
            this.error.set(preservedError);
        }
    }

    private scheduleStatusClear(
        ms: number,
        options: { clearStatus?: boolean; clearError?: boolean } = {},
    ): void {
        this.clearStatusTimer();

        const clearStatus = options.clearStatus ?? true;
        const clearError = options.clearError ?? false;
        const clear = () => {
            if (clearStatus) {
                this.status.set(null);
            }
            if (clearError) {
                this.error.set(null);
            }
        };

        if (ms <= 0 || typeof window === 'undefined') {
            clear();
            return;
        }

        this.statusClearTimer = window.setTimeout(() => {
            this.statusClearTimer = null;
            clear();
        }, ms);
    }

    private clearStatusTimer(): void {
        if (this.statusClearTimer !== null) {
            window.clearTimeout(this.statusClearTimer);
            this.statusClearTimer = null;
        }
    }

    private triggerFlash(): void {
        this.clearFlashTimer();

        if (typeof window === 'undefined') {
            return;
        }

        this.flash.set(true);
        this.flashTimer = window.setTimeout(() => {
            this.flashTimer = null;
            this.flash.set(false);
        }, ScanFeedback.FLASH_DURATION_MS);
    }

    private clearFlashTimer(): void {
        if (this.flashTimer !== null) {
            if (typeof window !== 'undefined') {
                window.clearTimeout(this.flashTimer);
            }
            this.flashTimer = null;
        }
        this.flash.set(false);
    }
}
