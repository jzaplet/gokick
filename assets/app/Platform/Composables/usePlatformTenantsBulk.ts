import type { ComputedRef, Ref } from 'vue';
import { computed, ref, watch } from 'vue';
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

    // A vanishing selection must DISARM, not merely hide. ConfirmModal's visibility
    // is prop-driven (`:show="bulkConfirm !== null"`), so when a filter change
    // clears the selection out from under the open modal it just disappears —
    // emitting no @cancel, leaving pendingBulk set. The next row ticked would then
    // pop the delete confirm back up unprompted, over a selection nobody armed, one
    // muscle-memory click from a real delete. cancelPendingBulk only covers the
    // explicit Cancel, so this is the path that has no other reset.
    watch(grid.selectedCount, (count: number): void => {
        if (count === 0) {
            pendingBulk.value = null;
        }
    });

    const bulkConfirm = computed((): BulkConfirm | null => {
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
        // Read the armed action rather than assuming it. Deleting unconditionally
        // would work today (there is one bulk action) at the cost of making
        // handleBulkAction decorative — it could stop arming entirely and every
        // test here would still pass while the grid's Delete did nothing in the
        // browser. It also means a second bulk action added later runs DELETE
        // whichever one was clicked. Mirrors createUsersBulk's dispatch.
        const action = pendingBulk.value;

        pendingBulk.value = null;

        if (action !== 'delete') {
            return;
        }

        // The selection can still have cleared between arming and confirming — do
        // nothing rather than post an empty payload the backend rejects.
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

    // Per-row delete: its OWN endpoint (DELETE /platform/tenants/:id), not the bulk
    // one narrowed to a single id. It answers 204 or a 400 naming the reason, so
    // there is no affected count to interpret and no skip arithmetic here — a
    // tenant the rule spares surfaces as the backend's message, which is more
    // useful than "0 deleted" would be.
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
            // Every reason this endpoint refuses — still has users, still has runs,
            // already gone — arrives under `general`, because none of them maps to a
            // field on a grid. So the backend's own message is the honest toast.
            error(result.data.general ?? 'Failed to delete the tenant.');

            // Reload on failure too. Every reason this can fail means the grid is
            // showing something stale — the tenant gained a user, or another
            // superadmin already deleted it — so leaving the row on screen with its
            // Delete button live invites the same click and the same error forever.
            await grid.reload();

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
