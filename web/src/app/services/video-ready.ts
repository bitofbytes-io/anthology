const VIDEO_READY_TIMEOUT_MS = 3000;

function isVideoReady(video: HTMLVideoElement): boolean {
    return (
        video.readyState >= HTMLMediaElement.HAVE_CURRENT_DATA &&
        video.videoWidth > 0 &&
        video.videoHeight > 0
    );
}

/** Resolves once the video has dimensions and a frame, or after a short timeout. */
export async function waitForVideoReady(
    video: HTMLVideoElement,
    timeoutMs = VIDEO_READY_TIMEOUT_MS,
): Promise<void> {
    if (isVideoReady(video)) {
        return;
    }

    await new Promise<void>((resolve) => {
        if (typeof window === 'undefined') {
            resolve();
            return;
        }

        const onReady = () => {
            if (!isVideoReady(video)) {
                return;
            }
            cleanup();
            resolve();
        };

        const timeout = window.setTimeout(() => {
            cleanup();
            resolve();
        }, timeoutMs);

        const cleanup = () => {
            window.clearTimeout(timeout);
            video.removeEventListener('loadedmetadata', onReady);
            video.removeEventListener('loadeddata', onReady);
            video.removeEventListener('playing', onReady);
            video.removeEventListener('resize', onReady);
        };

        video.addEventListener('loadedmetadata', onReady, { passive: true });
        video.addEventListener('loadeddata', onReady, { passive: true });
        video.addEventListener('playing', onReady, { passive: true });
        video.addEventListener('resize', onReady, { passive: true });

        void Promise.resolve().then(onReady);
    });
}
