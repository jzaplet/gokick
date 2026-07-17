<script setup lang="ts">
import type { PlatformUserCreateData } from '@/app/Platform/types/PlatformUserCreateData';
import type { PlatformUserFormErrors } from '@/app/Platform/types/PlatformUserFormErrors';
import type { PlatformTenantListResponse } from '@/app/Platform/types/PlatformTenantListResponse';
import { isPlatformTenantListResponse } from '@/app/Platform/types/PlatformTenantListResponse';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import PlatformUserForm from '@/app/Platform/Components/PlatformUserForm.vue';
import Spinner from '@/app-ui/Loading/Spinner.vue';

const router = useRouter();
const { success, error } = useToast();

const errors = ref<PlatformUserFormErrors>({});
const isLoading = ref(false);
const isFetching = ref(true);
const tenantOptions = ref<{ value: string; label: string }[]>([]);
const initial = ref<Partial<PlatformUserCreateData>>({});

const clearFieldError = (field: keyof PlatformUserFormErrors): void => {
    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
    delete errors.value[field];
};

// The picker reads the same paged endpoint the grid does, asking for its maximum
// page (tenant.ListPerPageMax = 100). Beyond that a select is the wrong control
// anyway — a searchable picker would be, and it would want its own endpoint.
onMounted(async (): Promise<void> => {
    const result = await authFetch<PlatformTenantListResponse>(
        'GET',
        '/api/v1/platform/tenants?page=1&per_page=100&sort_by=name&sort_dir=ASC',
        { validate: isPlatformTenantListResponse },
    );

    if (result.success === false) {
        // Stay on the loading state through the redirect. Flipping isFetching here
        // would paint the form — with a required tenant picker holding zero options
        // — for however long the navigation takes, and a submit landing in that gap
        // earns a 400 on a field with nothing to pick.
        error('Failed to load tenants.');
        void router.push({ name: 'platform-users' });

        return;
    }

    isFetching.value = false;
    tenantOptions.value = result.data.items.map((t) => ({ value: t.id, label: t.name }));

    // The list is one page deep, so past the cap some tenants simply are not in it
    // — and the form requires a tenant with no way to type one. Say so rather than
    // presenting a truncated list as if it were the whole set: an operator who
    // cannot find the tenant they just created would otherwise have no idea why.
    if (result.data.total > result.data.items.length) {
        error(`Showing the first ${String(result.data.items.length)} tenants of `
            + `${String(result.data.total)}. To add a user to a tenant that is not `
            + `listed, use: app create-user --tenant-id <id>`);
    }

    // Preselect only when there is nothing to choose. A single-tenant install has
    // exactly one tenant, so a blank required picker there is a question with one
    // answer — and the backend would (rightly) reject the blank.
    //
    // With several tenants it stays blank ON PURPOSE. Preselecting the first (or
    // the default tenant) would put a REAL tenant one mis-click away from owning a
    // new user, silently and irreversibly — an edit cannot move them out. Better a
    // 400 on the field than a user quietly born in the wrong company.
    const only = tenantOptions.value[0];

    if (tenantOptions.value.length === 1 && only !== undefined) {
        initial.value = { tenant_id: only.value };
    }
});

const handleSubmit = async (data: PlatformUserCreateData): Promise<void> => {
    isLoading.value = true;
    errors.value = {};

    const result = await authFetch<null, PlatformUserFormErrors, PlatformUserCreateData>(
        'POST',
        '/api/v1/platform/users',
        { body: data },
    );

    isLoading.value = false;

    if (result.success === false) {
        errors.value = result.data;

        return;
    }

    success(`User ${data.nickname} created.`);
    void router.push({ name: 'platform-users' });
};

const handleCancel = (): void => {
    void router.push({ name: 'platform-users' });
};
</script>

<template>
    <div>
        <div class="max-w-xl mx-auto space-y-6">
            <h1 class="text-2xl font-bold text-gray-900">
                New user
            </h1>

            <div
                v-if="isFetching === true"
                class="flex justify-center py-12"
            >
                <Spinner />
            </div>

            <PlatformUserForm
                v-else
                mode="create"
                submit-label="Create"
                :initial="initial"
                :is-loading="isLoading"
                :errors="errors"
                :tenants="tenantOptions"
                @submit="handleSubmit"
                @cancel="handleCancel"
                @clear-error="clearFieldError"
            />
        </div>
    </div>
</template>
