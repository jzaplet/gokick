import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { apiDownload } from '@/app-ui/Fetch';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';

// apiDownload uses fetch + Blob + a transient <a download> click. jsdom has no
// URL.createObjectURL/revokeObjectURL and clicking an anchor would try to
// navigate, so we stub those three before each test.
const blobResponse = (status: number, headers: Record<string, string> = {}): Response => {
    return new Response(new Blob(['file-bytes']), { status, headers });
};

describe('apiDownload', () => {
    const createObjectUrl = vi.fn((): string => 'blob:mock-url');
    const revokeObjectUrl = vi.fn();
    const clickMock = vi.fn();

    beforeEach((): void => {
        setAccessToken(null);
        createObjectUrl.mockClear();
        revokeObjectUrl.mockClear();
        clickMock.mockClear();
        // Object-URL APIs are absent in jsdom — provide stubs.
        URL.createObjectURL = createObjectUrl;
        URL.revokeObjectURL = revokeObjectUrl;
        // Prevent the real anchor navigation; just record the click happened.
        vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(clickMock);
    });

    afterEach((): void => {
        vi.restoreAllMocks();
    });

    it('derives the filename from the Content-Disposition header', async (): Promise<void> => {
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            blobResponse(200, { 'Content-Disposition': 'attachment; filename="report-2026.csv"' }),
        );

        const result = await apiDownload('/api/v1/export', 'fallback.csv');

        expect(result.success).toBe(true);
        expect(result.status).toBe(200);
        expect(result.filename).toBe('report-2026.csv');
    });

    it('handles an unquoted Content-Disposition filename', async (): Promise<void> => {
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            blobResponse(200, { 'Content-Disposition': 'attachment; filename=data.json' }),
        );

        const result = await apiDownload('/api/v1/export', 'fallback.json');

        expect(result.filename).toBe('data.json');
    });

    it('falls back to the provided filename when the header is absent', async (): Promise<void> => {
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(blobResponse(200));

        const result = await apiDownload('/api/v1/export', 'fallback-name.pdf');

        expect(result.success).toBe(true);
        expect(result.filename).toBe('fallback-name.pdf');
    });

    it('triggers a browser download (object URL + anchor click) on success', async (): Promise<void> => {
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            blobResponse(200, { 'Content-Disposition': 'attachment; filename="x.bin"' }),
        );

        await apiDownload('/api/v1/export', 'fallback.bin');

        expect(createObjectUrl).toHaveBeenCalledTimes(1);
        expect(clickMock).toHaveBeenCalledTimes(1);
        expect(revokeObjectUrl).toHaveBeenCalledWith('blob:mock-url');
    });

    it('returns a failure result and does not download on a non-ok response', async (): Promise<void> => {
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ message: 'forbidden' }), { status: 403 }),
        );

        const result = await apiDownload('/api/v1/export', 'fallback.csv');

        expect(result.success).toBe(false);
        expect(result.status).toBe(403);
        expect(result.filename).toBeNull();
        expect(clickMock).not.toHaveBeenCalled();
        expect(createObjectUrl).not.toHaveBeenCalled();
    });

    it('sends the Authorization header when an access token is set', async (): Promise<void> => {
        const fetchSpy = vi.spyOn(globalThis, 'fetch').mockResolvedValue(blobResponse(200));

        setAccessToken('dl-token');
        await apiDownload('/api/v1/export', 'fallback.csv');

        const requestInit = fetchSpy.mock.calls[0]?.[1];
        const headers = requestInit?.headers as Record<string, string> | undefined;

        expect(headers?.['Authorization']).toBe('Bearer dl-token');
    });
});
