import type { DownloadResult } from '@/app-ui/Fetch/types/DownloadResult';
import { buildAuthHeaders } from '@/app-ui/Fetch/buildHeaders';

// RFC 6266 Content-Disposition filename extraction with fallback.
const parseFilename = (response: Response, fallback: string): string => {
    const disposition = response.headers.get('Content-Disposition');
    const match = disposition?.match(/filename="?(.+?)"?$/);

    if (match !== null && match !== undefined) {
        return match[1] ?? fallback;
    }

    return fallback;
};

// Creates a transient <a download> click to trigger the browser's native
// "Save As" dialog without leaving the current page.
const triggerDownload = (blob: Blob, filename: string): void => {
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');

    link.href = url;
    link.download = filename;
    link.click();
    URL.revokeObjectURL(url);
};

// Fetches a binary resource as a Blob and triggers a browser download.
export const apiDownload = async (
    url: string,
    fallbackFilename: string,
): Promise<DownloadResult> => {
    const response = await fetch(url, {
        headers: buildAuthHeaders(),
        credentials: 'same-origin',
    });

    if (response.ok === false) {
        return { success: false, status: response.status, filename: null };
    }

    const blob = await response.blob();
    const filename = parseFilename(response, fallbackFilename);

    triggerDownload(blob, filename);

    return { success: true, status: response.status, filename };
};
