import type { ComputedRef, Ref } from 'vue';
import { computed, ref } from 'vue';
import type { GridState } from '@/app-ui/DataGrid/createGridState';
import type { BulkAction } from '@/app-ui/BulkActions/BulkActionBar.vue';
import type { BulkResult } from '@/app-ui/BulkActions/BulkResult';
import { isBulkResult } from '@/app-ui/BulkActions/BulkResult';
import type { PlatformBulkDeleteTenantsRequest }
    from '@/app/Platform/types/PlatformBulkDeleteTenantsRequest';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';

// The bulk half of the tenants grid. Deliberately NOT createUsersBulk: that core
// models a users grid (deactivate + delete + per-row activate), and tenants have
// exactly one bulk action. More to the point, the two disagree on what a partial
// result MEANS — see reportDeleted.

type TenantGridFilters = {
    name: string;
    plan: string;
};

type BulkConfirm = {
    title: string;
    message: string;
    confirmText: string;
};

// What the per-row delete needs off a row: the id to send, the name to name in
// the toast.
type DeleteTarget = {
    id: string;
    name: string;
};

export type TenantsBulk = {
    bulkActions: BulkAction[];
    bulkConfirm: ComputedRef<BulkConfirm | null>;
    handleBulkAction: (key: string) => void;
    runPendingBulk: () => Promise<void>;
    cancelPendingBulk: () => void;
    tenantToDelete: Ref<DeleteTarget | null>;
    askDelete: (tenant: DeleteTarget) => void;
    cancelDelete: () => void;
    runDelete: () => Promise<void>;
};

export const usePlatformTenantsBulk = (grid: GridState<TenantGridFilters>): TenantsBulk => {
    const { success, info, error } = useToast();

    const bulkActions: BulkAction[] = [{ key: 'delete', label: 'Delete' }];

    const pendingBulk = ref<'delete' | null>(null);
    const tenantToDelete = ref<DeleteTarget | null>(null);

    const bulkConfirm = computed((): BulkConfirm | null => {
        // Selection vanished under the open modal (a debounced filter change
        // cleared it): close rather than confirm an empty operation.
        if (grid.selectedCount.value === 0 || pendingBulk.value === null) {
            return null;
        }

        const count = String(grid.selectedCount.value);

        return {
            title: 'Delete selected tenants',
            message: `Really delete ${count} selected tenant(s)? Tenants that still have `
                + 'users are skipped. This action is irreversible.',
            confirmText: 'Delete',
        };
    });

    // Skipping is the DESIGNED outcome here, not an anomaly — the grid lets a
    // superadmin select freely and the server deletes only the empty ones. So a
    // partial result gets named rather than glossed: "3 deleted" after selecting
    // five would otherwise read as complete success.
    //
    // `expected` is null in all-filtered mode: nobody enumerated the selection, so
    // the skipped count is not knowable and is not guessed at.
    const reportDeleted = (affected: number, expected: number | null): void => {
        if (affected === 0) {
            info('No tenants were deleted — the selected tenants still have users.');

            return;
        }

        const skipped = expected === null ? 0 : expected - affected;

        if (skipped > 0) {
            success(`${String(affected)} tenant(s) deleted, `
                + `${String(skipped)} skipped (they still have users).`);

            return;
        }

        success(`${String(affected)} tenant(s) deleted.`);
    };

    const postDelete = async (
        body: PlatformBulkDeleteTenantsRequest,
        expected: number | null,
    ): Promise<void> => {
        const result = await authFetch<BulkResult, { general?: string },
            PlatformBulkDeleteTenantsRequest>(
            'POST',
            '/api/v1/platform/tenants/bulk-delete',
            { body, validate: isBulkResult },
        );

        if (result.success === false) {
            error(result.data.general ?? 'Bulk delete failed.');

            return;
        }

        reportDeleted(result.data.affected, expected);
        grid.clearSelection();
        await grid.reload();
    };

    const handleBulkAction = (key: string): void => {
        if (key === 'delete') {
            pendingBulk.value = 'delete';
        }
    };

    const runPendingBulk = async (): Promise<void> => {
        pendingBulk.value = null;

        // The selection may have cleared under the modal (debounced filter
        // change) between arming and confirming — do nothing rather than post an
        // empty payload the backend rejects.
        if (grid.selectedCount.value === 0) {
            return;
        }

        const allFiltered = grid.isAllFilteredSelected.value;
        const ids = grid.selectedIds();

        await postDelete(
            {
                ids,
                all_filtered: allFiltered,
                name: grid.filters.name,
                plan: grid.filters.plan,
            },
            allFiltered ? null : ids.length,
        );
    };

    const cancelPendingBulk = (): void => {
        pendingBulk.value = null;
    };

    // Per-row delete: same endpoint narrowed to one id, so the emptiness rule and
    // the audit trail have exactly one implementation on the server. Expected is 1
    // — a skip here means the grid's count was stale, which the toast should say
    // out loud rather than silently report success.
    const askDelete = (tenant: DeleteTarget): void => {
        tenantToDelete.value = tenant;
    };

    const cancelDelete = (): void => {
        tenantToDelete.value = null;
    };

    const runDelete = async (): Promise<void> => {
        const target = tenantToDelete.value;

        tenantToDelete.value = null;

        if (target === null) {
            return;
        }

        const result = await authFetch<null, { general?: string }>(
            'DELETE',
            `/api/v1/platform/tenants/${target.id}`,
        );

        if (result.success === false) {
            error(result.data.general ?? 'Failed to delete the tenant.');

            return;
        }

        success(`Tenant ${target.name} deleted.`);
        // Drop the id BEFORE reloading: a deleted row must not ride the next bulk
        // payload as a ghost id.
        grid.deselect(target.id);
        await grid.reload();
    };

    return {
        bulkActions,
        bulkConfirm,
        handleBulkAction,
        runPendingBulk,
        cancelPendingBulk,
        tenantToDelete,
        askDelete,
        cancelDelete,
        runDelete,
    };
};
