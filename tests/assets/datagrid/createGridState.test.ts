import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { nextTick } from 'vue';
import type { GridLoadArgs, GridLoadOutcome } from '@/app-ui/DataGrid/createGridState';
import { createGridState } from '@/app-ui/DataGrid/createGridState';

type Filters = { nickname: string; role: string };

type LoadCall = GridLoadArgs<Filters>;

const makeGrid = (
    overrides: { syncUrl?: boolean; total?: number } = {},
): { grid: ReturnType<typeof createGridState<Filters>>; calls: LoadCall[] } => {
    const calls: LoadCall[] = [];
    const grid = createGridState<Filters>({
        defaultSort: { column: 'nickname', direction: 'ASC' },
        filters: { nickname: '', role: '' },
        syncUrl: overrides.syncUrl ?? false,
        load: (args): Promise<GridLoadOutcome> => {
            calls.push(args);

            return Promise.resolve({ ok: true, total: overrides.total ?? 42 });
        },
    });

    return { grid, calls };
};

describe('createGridState', () => {
    beforeEach((): void => {
        vi.useFakeTimers();
        window.history.replaceState(null, '', '/admin/users');
    });

    afterEach((): void => {
        vi.useRealTimers();
    });

    it('init loads and stores the total', async (): Promise<void> => {
        const { grid, calls } = makeGrid();

        await grid.init();

        expect(calls).toHaveLength(1);
        expect(calls[0]).toMatchObject({ page: 1, perPage: 25 });
        expect(grid.total.value).toBe(42);
        expect(grid.isLoading.value).toBe(false);
    });

    it('sort cycles ASC → DESC → default and resets the page', async (): Promise<void> => {
        const { grid, calls } = makeGrid();

        await grid.init();
        grid.handleSort('role');

        expect(grid.sort.value).toEqual({ column: 'role', direction: 'ASC' });

        grid.handleSort('role');

        expect(grid.sort.value).toEqual({ column: 'role', direction: 'DESC' });

        grid.handleSort('role');

        expect(grid.sort.value).toEqual({ column: 'nickname', direction: 'ASC' });
        expect(calls).toHaveLength(4);
    });

    it('filter edits debounce into ONE reload with page reset', async (): Promise<void> => {
        const { grid, calls } = makeGrid();

        await grid.init();
        grid.handlePageChange(3);

        expect(calls).toHaveLength(2);

        grid.filters.nickname = 'a';
        await nextTick();
        grid.filters.nickname = 'al';
        await nextTick();

        expect(calls).toHaveLength(2);

        vi.advanceTimersByTime(400);
        await vi.runAllTimersAsync();

        expect(calls).toHaveLength(3);
        expect(calls[2]).toMatchObject({ page: 1, filters: { nickname: 'al', role: '' } });
    });

    it('clearFilters empties every filter and reports inactive', async (): Promise<void> => {
        const { grid } = makeGrid();

        grid.filters.nickname = 'x';
        grid.filters.role = 'admin';
        await nextTick();

        expect(grid.hasActiveFilters.value).toBe(true);

        grid.clearFilters();
        await nextTick();

        expect(grid.filters).toEqual({ nickname: '', role: '' });
        expect(grid.hasActiveFilters.value).toBe(false);
    });

    it('manual selection toggles rows and pages', (): void => {
        const { grid } = makeGrid();

        grid.toggleRow('a');
        grid.toggleRow('b');

        expect(grid.selectedCount.value).toBe(2);
        expect(grid.isSelected('a')).toBe(true);

        grid.toggleRow('a');

        expect(grid.isSelected('a')).toBe(false);

        grid.togglePage(['x', 'y', 'z']);

        expect(grid.selectedCount.value).toBe(4);

        grid.togglePage(['x', 'y', 'z']);

        expect(grid.selectedCount.value).toBe(1);
        expect(grid.selectedIds()).toEqual(['b']);
    });

    it('allFiltered selection counts the TOTAL without enumerating ids', async (): Promise<void> => {
        const { grid } = makeGrid({ total: 1234 });

        await grid.init();
        grid.selectAllFiltered();

        expect(grid.isAllFilteredSelected.value).toBe(true);
        expect(grid.selectedCount.value).toBe(1234);
        expect(grid.isSelected('anything')).toBe(true);

        grid.clearSelection();

        expect(grid.selectedCount.value).toBe(0);
    });

    it('filter change clears the selection (stale ids must not survive)', async (): Promise<void> => {
        const { grid } = makeGrid();

        await grid.init();
        grid.toggleRow('a');
        grid.filters.role = 'admin';
        await nextTick();
        vi.advanceTimersByTime(400);
        await vi.runAllTimersAsync();

        expect(grid.selectedCount.value).toBe(0);
    });

    it('syncUrl mirrors non-default state into the query string and reads it back', async (): Promise<void> => {
        const { grid } = makeGrid({ syncUrl: true });

        await grid.init();

        expect(window.location.search).toBe('');

        grid.handleSort('role');
        grid.handlePageChange(2);
        grid.filters.nickname = 'bob';
        await nextTick();
        vi.advanceTimersByTime(400);
        await vi.runAllTimersAsync();

        const params = new URLSearchParams(window.location.search);

        expect(params.get('sort_by')).toBe('role');
        expect(params.get('sort_dir')).toBe('ASC');
        expect(params.get('nickname')).toBe('bob');
        // The filter edit reset paging back to 1 — default values stay out of the URL.
        expect(params.get('page')).toBeNull();

        const fresh = makeGrid({ syncUrl: true });

        await fresh.grid.init();

        expect(fresh.grid.sort.value).toEqual({ column: 'role', direction: 'ASC' });
        expect(fresh.grid.filters.nickname).toBe('bob');
        expect(fresh.calls[0]).toMatchObject({ filters: { nickname: 'bob', role: '' } });
    });
});
