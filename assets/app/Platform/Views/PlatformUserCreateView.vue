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

    isFetching.value = false;

    if (result.success === false) {
        error('Failed to load tenants.');
        void router.push({ name: 'platform-users' });

        return;
    }

    tenantOptions.value = result.data.items.map((t) => ({ value: t.id, label: t.name }));
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
