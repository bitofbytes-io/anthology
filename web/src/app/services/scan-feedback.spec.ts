import { ScanFeedback } from './scan-feedback';

describe('ScanFeedback', () => {
    let feedback: ScanFeedback;

    beforeEach(() => {
        vi.useFakeTimers();
        feedback = new ScanFeedback();
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    it('marks a value as processing and flashes briefly', () => {
        expect(feedback.beginProcessing('9781234567890')).toBe(true);

        expect(feedback.processing()).toBe(true);
        expect(feedback.flash()).toBe(true);
        expect(feedback.status()).toBe('Found 9781234567890. Processing...');

        vi.advanceTimersByTime(ScanFeedback.FLASH_DURATION_MS);
        expect(feedback.flash()).toBe(false);
    });

    it('rejects empty values and detections while one is in flight', () => {
        expect(feedback.beginProcessing('')).toBe(false);
        expect(feedback.beginProcessing('111')).toBe(true);
        expect(feedback.beginProcessing('222')).toBe(false);
    });

    it('debounces repeated detections of the same value', () => {
        expect(feedback.beginProcessing('111')).toBe(true);
        feedback.reportComplete();
        expect(feedback.beginProcessing('111')).toBe(false);

        vi.advanceTimersByTime(ScanFeedback.DETECTION_DEBOUNCE_MS + 1);
        expect(feedback.beginProcessing('111')).toBe(true);
    });

    it('reports success then clears the message and error after the delay', () => {
        feedback.error.set('old error');
        feedback.reportSuccess('  Dune  ', 1000);

        expect(feedback.processing()).toBe(false);
        expect(feedback.error()).toBeNull();
        expect(feedback.status()).toBe('Found: Dune');

        vi.advanceTimersByTime(1000);
        expect(feedback.status()).toBeNull();
    });

    it('uses a generic success message when the title is blank', () => {
        feedback.reportSuccess('   ', 1000);
        expect(feedback.status()).toBe('Found item.');
    });

    it('reports failure and clears status and error after the delay', () => {
        feedback.status.set('Found 111. Processing...');
        feedback.reportFailure('No match', 500);

        expect(feedback.error()).toBe('No match');

        vi.advanceTimersByTime(500);
        expect(feedback.error()).toBeNull();
        expect(feedback.status()).toBeNull();
    });

    it('clears immediately when the delay is not positive', () => {
        feedback.reportFailure('No match', 0);

        expect(feedback.error()).toBeNull();
    });

    it('cancels a pending clear when processing begins again', () => {
        feedback.reportSuccess('Dune', 1000);
        vi.advanceTimersByTime(ScanFeedback.DETECTION_DEBOUNCE_MS);
        feedback.beginProcessing('222');

        vi.advanceTimersByTime(1000);
        expect(feedback.status()).toBe('Found 222. Processing...');
    });

    it('reportComplete clears status, error and processing', () => {
        feedback.beginProcessing('111');
        feedback.reportComplete();

        expect(feedback.processing()).toBe(false);
        expect(feedback.status()).toBeNull();
        expect(feedback.error()).toBeNull();
    });

    it('clearMessages clears messages, hint and flash', () => {
        feedback.beginProcessing('111');
        feedback.hint.set('hint');
        feedback.error.set('error');

        feedback.clearMessages();

        expect(feedback.status()).toBeNull();
        expect(feedback.error()).toBeNull();
        expect(feedback.hint()).toBeNull();
        expect(feedback.flash()).toBe(false);
    });

    it('reset clears everything except preserved messages', () => {
        feedback.beginProcessing('111');
        feedback.hint.set('hint');
        feedback.error.set('error');

        feedback.reset({ preserveError: true });

        expect(feedback.processing()).toBe(false);
        expect(feedback.status()).toBeNull();
        expect(feedback.hint()).toBeNull();
        expect(feedback.error()).toBe('error');

        feedback.hint.set('hint');
        feedback.status.set('status');
        feedback.reset({ preserveHint: true, preserveStatus: true });

        expect(feedback.hint()).toBe('hint');
        expect(feedback.status()).toBe('status');
        expect(feedback.error()).toBeNull();
    });
});
