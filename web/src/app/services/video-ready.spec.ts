import { waitForVideoReady } from './video-ready';

function createVideo(ready: boolean): HTMLVideoElement {
    const video = document.createElement('video');
    const state = { readyState: ready ? 4 : 0, videoWidth: ready ? 640 : 0, videoHeight: 480 };
    for (const [key, value] of Object.entries(state)) {
        Object.defineProperty(video, key, { configurable: true, writable: true, value });
    }
    return video;
}

describe('waitForVideoReady', () => {
    afterEach(() => {
        vi.useRealTimers();
    });

    it('resolves immediately when the video already has a frame', async () => {
        await expect(waitForVideoReady(createVideo(true))).resolves.toBeUndefined();
    });

    it('resolves once a media event fires after the video becomes ready', async () => {
        const video = createVideo(false);
        const pending = waitForVideoReady(video);

        (video as any).readyState = 4;
        (video as any).videoWidth = 640;
        video.dispatchEvent(new Event('loadeddata'));

        await expect(pending).resolves.toBeUndefined();
    });

    it('resolves after the timeout when the video never becomes ready', async () => {
        vi.useFakeTimers();
        const video = createVideo(false);
        const removeSpy = vi.spyOn(video, 'removeEventListener');
        const pending = waitForVideoReady(video, 100);

        await vi.advanceTimersByTimeAsync(100);

        await expect(pending).resolves.toBeUndefined();
        expect(removeSpy).toHaveBeenCalledWith('loadeddata', expect.any(Function));
    });
});
