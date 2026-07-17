import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
import type { GridState } from '@/app-ui/DataGrid/createGridState';

// The toast is the only place the partial-delete semantics reach the user, and
// "3 deleted, 2 skipped" vs a bare "3 deleted" is the whole difference between an
// honest report and one that reads as complete success. That arithmetic deserves
// more than the one browser click that first proved it.

const toasts = { success: vi.fn(), info: vi.fn(), error: vi.fn() };

vi.mock('@/app-ui/Toast/useToast', () => ({ useToast: () => toasts }));

const authFetch = vi.fn<(...args: unknown[]) => Promise<unknown>>();

vi.mock('@/app-ui/Auth', () => ({
    authFetch: (...args: unknown[]): Promise<unknown> => authFetch(...args),
}));

const { usePlatformTenantsBulk }
    = await import('@/app/Platform/Composables/usePlatformTenantsBulk');

// A grid stub carrying only what the composable touches. selectedCount is
// overridable because all-filtered mode counts the whole filtered set, which is
// not the same as the enumerated ids the stub carries.
const makeGrid = (opts: { ids: string[]; allFiltered?: boolean; count?: number }): GridState<{
    name: string;
    plan: string;
}> => {
    const state = {
        selectedCount: ref(opts.count ?? opts.ids.length),
        isAllFilteredSelected: ref(opts.allFiltered ?? false),
        selectedIds: (): string[] => opts.ids,
        filters: { name: '', plan: '' },
        clearSelection: vi.fn(),
        deselect: vi.fn(),
        reload: vi.fn(),
    };

    return state as unknown as GridState<{ name: string; plan: string }>;
};

const respondAffected = (affected: number): void => {
    authFetch.mockResolvedValue({ success: true, data: { affected } });
};

describe('usePlatformTenantsBulk — the affected count drives the toast', () => {
    beforeEach(() => {
        vi.clearAllMocks();
    });

    it('names the skipped tenants when only some of the selection went', async () => {
        respondAffected(3);
        const bulk = usePlatformTenantsBulk(makeGrid({ ids: ['a', 'b', 'c', 'd', 'e'] }));

        bulk.handleBulkAction('delete');
        await bulk.runPendingBulk();

        expect(toasts.success).toHaveBeenCalledWith(
            '3 tenant(s) deleted, 2 skipped (they still have users).',
        );
    });

    it('says nothing about skipping when the whole selection went', async () => {
        respondAffected(2);
        const bulk = usePlatformTenantsBulk(makeGrid({ ids: ['a', 'b'] }));

        bulk.handleBulkAction('delete');
        await bulk.runPendingBulk();

        expect(toasts.success).toHaveBeenCalledWith('2 tenant(s) deleted.');
    });

    // affected=0 is not a success — every selected tenant survived.
    it('reports zero as info with the reason, never as success', async () => {
        respondAffected(0);
        const bulk = usePlatformTenantsBulk(makeGrid({ ids: ['a', 'b'] }));

        bulk.handleBulkAction('delete');
        await bulk.runPendingBulk();

        expect(toasts.success).not.toHaveBeenCalled();
        expect(toasts.info).toHaveBeenCalledWith(
            'No tenants were deleted — the selected tenants still have users.',
        );
    });

    // All-filtered mode: nobody enumerated the selection, so the skipped count is
    // unknowable. It must not be guessed from selectedIds (which is a page's worth,
    // not the filtered set) — that would invent a skipped number out of thin air.
    it('does not claim a skipped count in all-filtered mode', async () => {
        respondAffected(3);
        const bulk = usePlatformTenantsBulk(
            makeGrid({ ids: ['a', 'b', 'c', 'd', 'e'], allFiltered: true }),
        );

        bulk.handleBulkAction('delete');
        await bulk.runPendingBulk();

        expect(toasts.success).toHaveBeenCalledWith('3 tenant(s) deleted.');
    });

    it('posts the grid filters so all-filtered deletes exactly what was shown', async () => {
        respondAffected(1);
        const grid = makeGrid({ ids: [], allFiltered: true, count: 12 });

        grid.filters.name = 'Ghost';
        grid.filters.plan = 'free';
        const bulk = usePlatformTenantsBulk(grid);

        bulk.handleBulkAction('delete');
        await bulk.runPendingBulk();

        const [method, url, options] = authFetch.mock.calls[0] ?? [];

        expect(method).toBe('POST');
        expect(url).toBe('/api/v1/platform/tenants/bulk-delete');
        expect(options).toMatchObject({
            body: { all_filtered: true, name: 'Ghost', plan: 'free' },
        });
    });

    // The selection can vanish under an open modal (a debounced filter change
    // clears it) between arming the action and confirming it.
    it('posts nothing when the selection vanished under the modal', async () => {
        const bulk = usePlatformTenantsBulk(makeGrid({ ids: [] }));

        bulk.handleBulkAction('delete');
        await bulk.runPendingBulk();

        expect(authFetch).not.toHaveBeenCalled();
    });
});
