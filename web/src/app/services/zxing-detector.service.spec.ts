import { BrowserMultiFormatReader } from '@zxing/browser';
import { NotFoundException } from '@zxing/library';

import { ZxingDetector } from './zxing-detector.service';

type DecodeCallback = (
    result: { getText(): string } | null,
    error: unknown,
    controls: { stop(): void },
) => void;

describe('ZxingDetector', () => {
    let detector: ZxingDetector;
    let callback: DecodeCallback;
    let decode: ReturnType<typeof vi.fn>;
    let stop: ReturnType<typeof vi.fn<() => void>>;
    let controls: { stop(): void };

    function install(): void {
        decode = vi.fn((_video: HTMLVideoElement, cb: DecodeCallback) => {
            callback = cb;
            return Promise.resolve(controls);
        });
        (detector as any).reader = { decodeFromVideoElement: decode };
    }

    beforeEach(() => {
        detector = new ZxingDetector();
        stop = vi.fn<() => void>();
        controls = { stop };
    });

    it('starts once a reader has been prepared', () => {
        const decodeSpy = vi
            .spyOn(BrowserMultiFormatReader.prototype, 'decodeFromVideoElement')
            .mockResolvedValue(controls);
        const video = document.createElement('video');

        try {
            detector.prepare();

            expect(detector.start(video, vi.fn(), vi.fn())).toBe(true);
            expect(decodeSpy).toHaveBeenCalledWith(video, expect.any(Function));
        } finally {
            decodeSpy.mockRestore();
        }
    });

    it('does not start before the reader is prepared', () => {
        const video = document.createElement('video');

        expect(detector.start(video, vi.fn(), vi.fn())).toBe(false);
    });

    it('reports trimmed decoded values', () => {
        install();
        const onValue = vi.fn();
        const onError = vi.fn();

        expect(detector.start(document.createElement('video'), onValue, onError)).toBe(true);
        callback({ getText: () => ' 9781234567890 ' }, null, controls);

        expect(onValue).toHaveBeenCalledWith('9781234567890');
        expect(onError).not.toHaveBeenCalled();
    });

    it('ignores frames without a barcode but reports other errors', () => {
        install();
        const onValue = vi.fn();
        const onError = vi.fn();
        const failure = new Error('decode failed');

        detector.start(document.createElement('video'), onValue, onError);
        callback(null, new NotFoundException(), controls);
        expect(onError).not.toHaveBeenCalled();

        callback(null, failure, controls);
        expect(onError).toHaveBeenCalledWith(failure);
        expect(onValue).not.toHaveBeenCalled();
    });

    it('reports a rejected decode promise as an error', async () => {
        install();
        const failure = new Error('camera lost');
        decode.mockReturnValue(Promise.reject(failure));
        const onError = vi.fn();

        detector.start(document.createElement('video'), vi.fn(), onError);
        await Promise.resolve();
        await Promise.resolve();

        expect(onError).toHaveBeenCalledWith(failure);
    });

    it('stops the controls and discards the reader', () => {
        install();
        detector.start(document.createElement('video'), vi.fn(), vi.fn());
        callback(null, new NotFoundException(), controls);

        detector.stop();

        expect(stop).toHaveBeenCalledTimes(1);
        expect(detector.start(document.createElement('video'), vi.fn(), vi.fn())).toBe(false);
    });
});
