import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { apiUpload } from '@/app-ui/Fetch';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';
import type { UploadProgress } from '@/app-ui/Fetch/types/UploadProgress';

// apiUpload is XHR-based (fetch has no upload-progress API), so to exercise it
// we install a minimal fake XMLHttpRequest on globalThis. The fake records the
// handlers apiUpload attaches (upload.onprogress, onload, onerror) and exposes
// helpers so each test can drive a progress tick and/or completion.
type ProgressInit = { loaded: number; total: number; lengthComputable: boolean };

class FakeXhr {
    static instances: FakeXhr[] = [];

    upload: { onprogress: ((event: ProgressInit) => void) | null } = { onprogress: null };
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;

    method = '';
    url = '';
    sentBody: unknown = null;
    requestHeaders: Record<string, string> = {};

    status = 0;
    statusText = '';
    responseText = '';

    constructor() {
        FakeXhr.instances.push(this);
    }

    open(method: string, url: string): void {
        this.method = method;
        this.url = url;
    }

    setRequestHeader(key: string, value: string): void {
        this.requestHeaders[key] = value;
    }

    send(body: unknown): void {
        this.sentBody = body;
    }

    // Test helpers (not part of the real XHR API).
    fireProgress(init: ProgressInit): void {
        this.upload.onprogress?.(init);
    }

    fireLoad(status: number, responseText: string, statusText = ''): void {
        this.status = status;
        this.statusText = statusText;
        this.responseText = responseText;
        this.onload?.();
    }

    fireError(status = 0, responseText = ''): void {
        this.status = status;
        this.responseText = responseText;
        this.onerror?.();
    }
}

const lastXhr = (): FakeXhr => {
    const xhr = FakeXhr.instances[FakeXhr.instances.length - 1];

    if (xhr === undefined) {
        throw new Error('no XMLHttpRequest was constructed');
    }

    return xhr;
};

type UploadData = { id: string };

describe('apiUpload', () => {
    let originalXhr: typeof XMLHttpRequest;

    beforeEach((): void => {
        setAccessToken(null);
        FakeXhr.instances = [];
        originalXhr = globalThis.XMLHttpRequest;
        // FakeXhr is structurally compatible with the slice of XHR apiUpload uses.
        globalThis.XMLHttpRequest = FakeXhr as unknown as typeof XMLHttpRequest;
    });

    afterEach((): void => {
        globalThis.XMLHttpRequest = originalXhr;
        vi.restoreAllMocks();
    });

    it('reports progress with percent (0-100), loaded and total bytes', async (): Promise<void> => {
        const seen: UploadProgress[] = [];
        const promise = apiUpload<UploadData>('/api/v1/files', new FormData(), (stats) => {
            seen.push(stats);
        });

        const xhr = lastXhr();

        // Two ticks: half-way then complete.
        xhr.fireProgress({ loaded: 50, total: 200, lengthComputable: true });
        xhr.fireProgress({ loaded: 200, total: 200, lengthComputable: true });
        xhr.fireLoad(200, JSON.stringify({ id: 'f-1' }));

        await promise;

        expect(seen).toEqual([
            { percent: 25, loaded: 50, total: 200 },
            { percent: 100, loaded: 200, total: 200 },
        ]);
    });

    it('computes percent as (loaded / total) * 100 for non-round values', async (): Promise<void> => {
        const seen: UploadProgress[] = [];
        const promise = apiUpload<UploadData>('/api/v1/files', new FormData(), (stats) => {
            seen.push(stats);
        });

        const xhr = lastXhr();

        xhr.fireProgress({ loaded: 1, total: 8, lengthComputable: true });
        xhr.fireLoad(200, JSON.stringify({ id: 'f-1' }));

        await promise;

        const stats = seen[0];

        expect(stats).toBeDefined();
        expect(stats?.percent).toBeCloseTo(12.5, 5);
        expect(stats?.loaded).toBe(1);
        expect(stats?.total).toBe(8);
    });

    it('does NOT invoke onProgress when length is not computable', async (): Promise<void> => {
        const onProgress = vi.fn();
        const promise = apiUpload<UploadData>('/api/v1/files', new FormData(), onProgress);

        const xhr = lastXhr();

        xhr.fireProgress({ loaded: 10, total: 0, lengthComputable: false });
        xhr.fireLoad(200, JSON.stringify({ id: 'f-1' }));

        await promise;

        expect(onProgress).not.toHaveBeenCalled();
    });

    it('resolves with the typed success payload on 2xx', async (): Promise<void> => {
        const promise = apiUpload<UploadData>('/api/v1/files', new FormData());

        lastXhr().fireLoad(201, JSON.stringify({ id: 'f-42' }));

        const result = await promise;

        expect(result.success).toBe(true);
        expect(result.status).toBe(201);

        if (result.success === true) {
            expect(result.data.id).toBe('f-42');
        }
    });

    it('works without an onProgress callback (no upload handler attached)', async (): Promise<void> => {
        const promise = apiUpload<UploadData>('/api/v1/files', new FormData());

        const xhr = lastXhr();

        // With no callback supplied, apiUpload must not register upload.onprogress.
        expect(xhr.upload.onprogress).toBeNull();

        xhr.fireLoad(200, JSON.stringify({ id: 'f-1' }));

        const result = await promise;

        expect(result.success).toBe(true);
    });

    it('attaches the Authorization header when an access token is set', async (): Promise<void> => {
        setAccessToken('upload-token');

        const promise = apiUpload<UploadData>('/api/v1/files', new FormData());
        const xhr = lastXhr();

        expect(xhr.method).toBe('POST');
        expect(xhr.url).toBe('/api/v1/files');
        expect(xhr.requestHeaders['Authorization']).toBe('Bearer upload-token');

        xhr.fireLoad(200, JSON.stringify({ id: 'f-1' }));
        await promise;
    });

    it('resolves a failure response on network error', async (): Promise<void> => {
        const promise = apiUpload<UploadData>('/api/v1/files', new FormData());

        lastXhr().fireError(0, '');

        const result = await promise;

        expect(result.success).toBe(false);

        if (result.success === false) {
            expect(result.data.message).toBe('Network error');
        }
    });
});
