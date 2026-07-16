import type { ComputedRef, Ref, UnwrapNestedRefs } from 'vue';
import { computed, nextTick, reactive, readonly, ref, watch } from 'vue';
import { createDebounce } from '@/app-ui/Debounce/createDebounce';

// Headless server-side grid state (ported from aibobr): one factory holds
// page/sort/filters/selection and calls the injected load() callback — it
// knows NOTHING about HTTP or rendering. The view owns the rows and the
// fetch; DataGrid.vue renders the chrome. Filters are debounced (typing in a
// search box must not fire a request per keystroke) and optionally mirrored
// into the URL so a filtered/sorted page survives reload and can be shared.

type SortDirection = 'ASC' | 'DESC';

export type GridSort = {
    column: string;
    direction: SortDirection;
};

export type GridColumn = {
    key: string;
    label: string;
    sortable?: boolean;
    align?: 'left' | 'right';
};

export type GridLoadArgs<F> = {
    page: number;
    perPage: number;
    sort: GridSort;
    // UnwrapNestedRefs = what reactive() hands back; for the flat string
    // records the grid takes it resolves to F itself at every concrete call.
    filters: UnwrapNestedRefs<F>;
};

// The load callback reports the filtered total on success; on failure the view
// has already surfaced the error (toast) — the grid just keeps its last state.
export type GridLoadOutcome = { ok: true; total: number } | { ok: false };

export type GridStateOptions<F extends Record<string, string>> = {
    defaultSort: GridSort;
    filters: F;
    load: (args: GridLoadArgs<F>) => Promise<GridLoadOutcome>;
    perPage?: number;
    syncUrl?: boolean;
    debounceMs?: number;
};

export type GridState<F extends Record<string, string>> = {
    page: Readonly<Ref<number>>;
    perPage: Readonly<Ref<number>>;
    sort: Readonly<Ref<GridSort>>;
    filters: UnwrapNestedRefs<F>;
    total: Readonly<Ref<number>>;
    isLoading: Readonly<Ref<boolean>>;
    hasActiveFilters: ComputedRef<boolean>;
    selectedCount: ComputedRef<number>;
    isAllFilteredSelected: ComputedRef<boolean>;
    init: () => Promise<void>;
    reload: () => Promise<void>;
    handleSort: (column: string) => void;
    handlePageChange: (page: number) => void;
    clearFilters: () => void;
    isSelected: (id: string) => boolean;
    toggleRow: (id: string) => void;
    togglePage: (ids: string[]) => void;
    selectAllFiltered: () => void;
    clearSelection: () => void;
    selectedIds: () => string[];
};

const readUrl = (
    page: Ref<number>,
    sort: Ref<GridSort>,
    filters: Record<string, string>,
    defaults: { sort: GridSort; keys: string[] },
): void => {
    const params = new URLSearchParams(window.location.search);
    const urlPage = Number.parseInt(params.get('page') ?? '', 10);

    if (Number.isFinite(urlPage) === true && urlPage > 1) {
        page.value = urlPage;
    }

    const sortBy = params.get('sort_by');
    const sortDir = params.get('sort_dir');

    if (sortBy !== null && sortBy !== '') {
        sort.value = {
            column: sortBy,
            direction: sortDir === 'DESC' ? 'DESC' : 'ASC',
        };
    } else {
        sort.value = { ...defaults.sort };
    }

    for (const key of defaults.keys) {
        const value = params.get(key);

        if (value !== null && value !== '') {
            filters[key] = value;
        }
    }
};

const writeUrl = (
    page: number,
    sort: GridSort,
    filters: Record<string, string>,
    defaultSort: GridSort,
): void => {
    const params = new URLSearchParams();

    if (page > 1) {
        params.set('page', String(page));
    }

    if (sort.column !== defaultSort.column || sort.direction !== defaultSort.direction) {
        params.set('sort_by', sort.column);
        params.set('sort_dir', sort.direction);
    }

    for (const [key, value] of Object.entries(filters)) {
        if (value !== '') {
            params.set(key, value);
        }
    }

    const qs = params.toString();
    const url = qs === '' ? window.location.pathname : `${window.location.pathname}?${qs}`;

    // replaceState (not router.replace): no history entry, no guard re-run —
    // the URL is a mirror of grid state, not a navigation.
    window.history.replaceState(window.history.state, '', url);
};

export const createGridState = <F extends Record<string, string>>(
    options: GridStateOptions<F>,
): GridState<F> => {
    const page = ref(1);
    const perPage = ref(options.perPage ?? 25);
    const sort = ref<GridSort>({ ...options.defaultSort });
    const filters = reactive({ ...options.filters });
    // The same object through a plain-record lens — F's concrete keys make
    // string indexing a type error, and the repo bans `as`.
    const filterRecord: Record<string, string> = filters;
    const total = ref(0);
    const isLoading = ref(false);

    // Selection is dual-mode: 'manual' tracks explicit ids; 'allFiltered'
    // means "every row the current filters match" WITHOUT enumerating ids —
    // bulk actions send the filter set instead of an id list.
    const selected = ref(new Set<string>());
    const allFiltered = ref(false);

    const debounce = createDebounce(options.debounceMs ?? 400);

    // True only while init() applies the URL into the filters — the filter
    // watcher must ignore those programmatic writes, or a filtered deep link
    // would reset the page to 1 and fire a second (duplicate) load.
    let urlSyncing = false;

    const reload = async (): Promise<void> => {
        if (options.syncUrl === true) {
            writeUrl(page.value, sort.value, filterRecord, options.defaultSort);
        }

        isLoading.value = true;

        const outcome = await options.load({
            page: page.value,
            perPage: perPage.value,
            sort: { ...sort.value },
            filters: { ...filters },
        });

        isLoading.value = false;

        if (outcome.ok === true) {
            total.value = outcome.total;

            // Clamp an out-of-range page and re-fetch: a bulk delete can empty
            // the last page, and a hand-edited ?page=999 deep link must degrade
            // gracefully instead of stranding the user on an empty page whose
            // pager has hidden itself.
            const maxPage = Math.max(1, Math.ceil(total.value / perPage.value));

            if (page.value > maxPage) {
                page.value = maxPage;
                await reload();
            }
        }
    };

    const clearSelection = (): void => {
        selected.value = new Set<string>();
        allFiltered.value = false;
    };

    watch(filters, (): void => {
        if (urlSyncing === true) {
            return;
        }

        debounce.run((): void => {
            page.value = 1;
            clearSelection();
            void reload();
        });
    });

    const handleSort = (column: string): void => {
        // Tri-state cycle: ASC → DESC → back to the default sort.
        if (sort.value.column !== column) {
            sort.value = { column, direction: 'ASC' };
        } else if (sort.value.direction === 'ASC') {
            sort.value = { column, direction: 'DESC' };
        } else {
            sort.value = { ...options.defaultSort };
        }

        page.value = 1;
        void reload();
    };

    const handlePageChange = (next: number): void => {
        page.value = next;
        void reload();
    };

    const clearFilters = (): void => {
        for (const key of Object.keys(filterRecord)) {
            filterRecord[key] = '';
        }
    };

    const init = async (): Promise<void> => {
        if (options.syncUrl === true) {
            urlSyncing = true;
            readUrl(page, sort, filterRecord, {
                sort: options.defaultSort,
                keys: Object.keys(options.filters),
            });
            // Let the filter watcher flush (and skip, thanks to urlSyncing)
            // before we clear the flag and issue the single initial load.
            await nextTick();
            urlSyncing = false;
        }

        await reload();
    };

    // In allFiltered mode a per-row toggle can't be expressed as "all except
    // this" (the ids aren't enumerated), so any toggle DROPS the escalation to
    // an empty selection — clear + reselect. This deliberately trades the
    // "all-except-a-few" convenience for safety: the previous behavior turned
    // "uncheck a row to spare it" into "select ONLY that row", so a bulk delete
    // removed exactly the row the user meant to keep.
    const toggleRow = (id: string): void => {
        if (allFiltered.value === true) {
            clearSelection();

            return;
        }

        const next = new Set(selected.value);

        if (next.has(id) === true) {
            next.delete(id);
        } else {
            next.add(id);
        }

        selected.value = next;
    };

    const togglePage = (ids: string[]): void => {
        // The header checkbox reads "all selected" in allFiltered mode, so a
        // click means deselect — clear, don't re-add the page.
        if (allFiltered.value === true) {
            clearSelection();

            return;
        }

        const next = new Set(selected.value);
        const allPresent = ids.every((id) => next.has(id) === true);

        for (const id of ids) {
            if (allPresent === true) {
                next.delete(id);
            } else {
                next.add(id);
            }
        }

        selected.value = next;
    };

    const selectAllFiltered = (): void => {
        selected.value = new Set<string>();
        allFiltered.value = true;
    };

    return {
        page: readonly(page),
        perPage: readonly(perPage),
        sort: readonly(sort),
        filters,
        total: readonly(total),
        isLoading: readonly(isLoading),
        hasActiveFilters: computed((): boolean =>
            Object.values(filterRecord).some((value) => value !== '')),
        selectedCount: computed((): number =>
            allFiltered.value === true ? total.value : selected.value.size),
        isAllFilteredSelected: computed((): boolean => allFiltered.value === true),
        init,
        reload,
        handleSort,
        handlePageChange,
        clearFilters,
        isSelected: (id: string): boolean =>
            allFiltered.value === true || selected.value.has(id) === true,
        toggleRow,
        togglePage,
        selectAllFiltered,
        clearSelection,
        selectedIds: (): string[] => [...selected.value],
    };
};
