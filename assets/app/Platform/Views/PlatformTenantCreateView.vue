<script setup lang="ts">
import type { PlatformTenantFormData } from '@/app/Platform/types/PlatformTenantFormData';
import type { PlatformTenantFormErrors } from '@/app/Platform/types/PlatformTenantFormErrors';
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useI18n } from '@/app-ui/I18n';
import { useToast } from '@/app-ui/Toast/useToast';
import PlatformTenantForm from '@/app/Platform/Components/PlatformTenantForm.vue';

const router = useRouter();
const { success } = useToast();
const { t } = useI18n();

const errors = ref<PlatformTenantFormErrors>({});
const isLoading = ref(false);

const clearFieldError = (field: keyof PlatformTenantFormErrors): void => {
    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
    delete errors.value[field];
};

const handleSubmit = async (data: PlatformTenantFormData): Promise<void> => {
    isLoading.value = true;
    errors.value = {};

    const result = await authFetch<null, PlatformTenantFormErrors, PlatformTenantFormData>(
        'POST',
        '/api/v1/platform/tenants',
        { body: data },
    );

    isLoading.value = false;

    if (result.success === false) {
        errors.value = result.data;

        return;
    }

    success(t('tenants.created', { name: data.name }));
    void router.push({ name: 'platform-tenants' });
};

const handleCancel = (): void => {
    void router.push({ name: 'platform-tenants' });
};
</script>

<template>
    <div>
        <div class="max-w-xl mx-auto space-y-6">
            <h1 class="text-2xl font-bold text-gray-900">
                {{ t('tenants.new_title') }}
            </h1>

            <PlatformTenantForm
                :submit-label="t('common.create')"
                :is-loading="isLoading"
                :errors="errors"
                @submit="handleSubmit"
                @cancel="handleCancel"
                @clear-error="clearFieldError"
            />
        </div>
    </div>
</template>
