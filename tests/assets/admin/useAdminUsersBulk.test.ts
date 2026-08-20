import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createGridState } from '@/app-ui/DataGrid/createGridState';
import { useAdminUsersBulk } from '@/app/Admin/Composables/useAdminUsersBulk';

// The composable talks to the API through authFetch — the mock lets the test
// pin the CONFIRM GATE: no request may leave before runPendingBulk /
// runActivate.
type AuthFetchCall = (
    method: string,
    url: string,
    options?: { body?: Record<string, unknown> },
) => Promise<{ success: true; status: number; data: { affected: number } }>;

const authFetchMock = vi.hoisted(() => vi.fn<AuthFetchCall>());

vi.mock('@/app-ui/Auth', () => ({
    authFetch: authFetchMock,
}));

vi.mock('@/app-ui/Toast/useToast', () => ({
    useToast: () => ({ success: vi.fn(), info: vi.fn(), error: vi.fn() }),
}));

const makeGrid = (): ReturnType<typeof createGridState<{
    nickname: string;
    email: string;
    role: string;
    active: string;
}>> =>
    createGridState({
        defaultSort: { column: 'nickname', direction: 'ASC' },
        filters: { nickname: '', email: '', role: '', active: '' },
        load: () => Promise.resolve({ ok: true, total: 2 }),
    });

describe('useAdminUsersBulk', () => {
    beforeEach(() => {
        authFetchMock.mockReset();
        authFetchMock.mockResolvedValue({ success: true, status: 200, data: { affected: 1 } });
    });

    it('offers only deactivate and delete as bulk actions', () => {
        const bulk = useAdminUsersBulk(makeGrid());

        expect(bulk.bulkActions.value.map((a: { key: string }) => a.key)).toEqual(['deactivate', 'delete']);

        // Activation is a per-row action — the bulk path must ignore it.
        bulk.handleBulkAction('activate');

        expect(bulk.bulkConfirm.value).toBeNull();
    });

    it('every bulk action arms the confirm modal and fires NOTHING until confirmed', () => {
        const grid = makeGrid();

        grid.toggleRow('u1');

        const bulk = useAdminUsersBulk(grid);

        for (const key of ['deactivate', 'delete']) {
            bulk.handleBulkAction(key);

            expect(bulk.bulkConfirm.value).not.toBeNull();
            expect(authFetchMock).not.toHaveBeenCalled();

            bulk.cancelPendingBulk();

            expect(bulk.bulkConfirm.value).toBeNull();
        }
    });

    it('describes the pending action with the selected count', () => {
        const grid = makeGrid();

        grid.toggleRow('u1');
        grid.toggleRow('u2');

        const bulk = useAdminUsersBulk(grid);

        bulk.handleBulkAction('deactivate');

        expect(bulk.bulkConfirm.value?.title).toBe('Deactivate selected users');
        expect(bulk.bulkConfirm.value?.message).toContain('2 selected users');
        expect(bulk.bulkConfirm.value?.confirmText).toBe('Deactivate');
    });

    it('runs the confirmed action against the right endpoint and disarms', async (): Promise<void> => {
        const grid = makeGrid();

        grid.toggleRow('u1');

        const bulk = useAdminUsersBulk(grid);

        bulk.handleBulkAction('delete');
        await bulk.runPendingBulk();

        expect(authFetchMock).toHaveBeenCalledTimes(1);
        const call = authFetchMock.mock.calls[0];

        expect(call?.[0]).toBe('POST');
        expect(call?.[1]).toBe('/api/v1/admin/users/bulk-delete');
        expect(call?.[2]?.body).toEqual({
            ids: ['u1'],
            all_filtered: false,
            nickname: '',
            email: '',
            role: '',
            active: '',
        });
        expect(bulk.bulkConfirm.value).toBeNull();
    });

    it('routes deactivate to bulk-active with set_active false', async (): Promise<void> => {
        const grid = makeGrid();

        grid.toggleRow('u1');

        const bulk = useAdminUsersBulk(grid);

        bulk.handleBulkAction('deactivate');
        await bulk.runPendingBulk();

        const call = authFetchMock.mock.calls[0];

        expect(call?.[0]).toBe('POST');
        expect(call?.[1]).toBe('/api/v1/admin/users/bulk-active');
        expect(call?.[2]?.body).toEqual({
            ids: ['u1'],
            all_filtered: false,
            nickname: '',
            email: '',
            role: '',
            active: '',
            set_active: false,
        });
    });

    it('activates a single row only after the confirm', async (): Promise<void> => {
        const bulk = useAdminUsersBulk(makeGrid());

        bulk.askActivate({ id: 'u7', nickname: 'alice' });

        expect(bulk.userToActivate.value?.nickname).toBe('alice');
        expect(authFetchMock).not.toHaveBeenCalled();

        await bulk.runActivate();

        const call = authFetchMock.mock.calls[0];

        expect(call?.[0]).toBe('POST');
        expect(call?.[1]).toBe('/api/v1/admin/users/bulk-active');
        expect(call?.[2]?.body).toEqual({
            ids: ['u7'],
            all_filtered: false,
            nickname: '',
            email: '',
            role: '',
            active: '',
            set_active: true,
        });
        expect(bulk.userToActivate.value).toBeNull();
    });

    it('cancelActivate disarms without a request', () => {
        const bulk = useAdminUsersBulk(makeGrid());

        bulk.askActivate({ id: 'u7', nickname: 'alice' });
        bulk.cancelActivate();

        expect(bulk.userToActivate.value).toBeNull();
        expect(authFetchMock).not.toHaveBeenCalled();
    });

    it('disarms the confirm modal and fires nothing when the selection clears underneath it', async (): Promise<void> => {
        const grid = makeGrid();

        grid.toggleRow('u1');

        const bulk = useAdminUsersBulk(grid);

        bulk.handleBulkAction('delete');

        expect(bulk.bulkConfirm.value).not.toBeNull();

        // A debounced filter change clears the selection while the modal is open.
        grid.clearSelection();

        expect(bulk.bulkConfirm.value).toBeNull();

        await bulk.runPendingBulk();

        expect(authFetchMock).not.toHaveBeenCalled();
    });
});
