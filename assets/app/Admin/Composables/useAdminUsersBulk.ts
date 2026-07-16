import type { GridState } from '@/app-ui/DataGrid/createGridState';
import type { UsersBulk } from '@/app-ui/BulkActions/createUsersBulk';
import { createUsersBulk } from '@/app-ui/BulkActions/createUsersBulk';
import type { BulkDeleteUsersRequest } from '@/app/Admin/types/BulkDeleteUsersRequest';
import type { BulkActiveUsersRequest } from '@/app/Admin/types/BulkActiveUsersRequest';

type AdminGridFilters = {
    nickname: string;
    email: string;
    role: string;
    active: string;
};

// The admin users grid's bulk half — a thin wrapper over the shared core that
// supplies the admin endpoints and request bodies (no tenant filter).
export const useAdminUsersBulk = (grid: GridState<AdminGridFilters>): UsersBulk =>
    createUsersBulk<AdminGridFilters, BulkDeleteUsersRequest, BulkActiveUsersRequest>(grid, {
        deleteUrl: '/api/v1/admin/users/bulk-delete',
        activeUrl: '/api/v1/admin/users/bulk-active',
        buildDelete: (): BulkDeleteUsersRequest => ({
            ids: grid.selectedIds(),
            all_filtered: grid.isAllFilteredSelected.value,
            nickname: grid.filters.nickname,
            email: grid.filters.email,
            role: grid.filters.role,
            active: grid.filters.active,
        }),
        buildActive: (setActive: boolean): BulkActiveUsersRequest => ({
            ids: grid.selectedIds(),
            all_filtered: grid.isAllFilteredSelected.value,
            nickname: grid.filters.nickname,
            email: grid.filters.email,
            role: grid.filters.role,
            active: grid.filters.active,
            set_active: setActive,
        }),
        buildActivate: (id: string): BulkActiveUsersRequest => ({
            ids: [id],
            all_filtered: false,
            nickname: '',
            email: '',
            role: '',
            active: '',
            set_active: true,
        }),
    });
