<script setup lang="ts">
import type { PlatformTenantFormData } from '@/app/Platform/types/PlatformTenantFormData';
import type { PlatformTenantFormErrors } from '@/app/Platform/types/PlatformTenantFormErrors';
import { reactive } from 'vue';
import { tm, useI18n } from '@/app-ui/I18n';
import Button from '@/app-ui/Buttons/Button.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import ErrorAlert from '@/app-ui/Alerts/ErrorAlert.vue';

// The tenant form. Create-only for now, so there is no `mode` prop — the platform
// plane offers no tenant edit, and a mode with one value would be decoration.
//
// Name is the only field: NewTenant stamps plan='free' (the sole tier gokick
// ships) and the id. A plan picker belongs here once a paid tier exists — see the
// planOptions note in PlatformTenantsView.
//
// No client-side validation: the backend's tenant.NewName is authoritative and
// reports back through the `errors` prop.
const { submitLabel, isLoading, errors } = defineProps<{
    submitLabel: string;
    isLoading: boolean;
    errors: PlatformTenantFormErrors;
}>();

const emit = defineEmits<{
    submit: [data: PlatformTenantFormData];
    cancel: [];
    clearError: [field: keyof PlatformTenantFormErrors];
}>();

const { t } = useI18n();

const form: PlatformTenantFormData = reactive({
    name: '',
});

const handleSubmit = (): void => {
    emit('submit', { ...form });
};
</script>

<template>
    <form
        class="bg-white rounded-lg shadow-md p-6 space-y-4"
        @submit.prevent="handleSubmit"
    >
        <Input
            v-model="form.name"
            name="name"
            type="text"
            :label="t('tenants.name')"
            placeholder="Acme Corp"
            :error="tm(errors.name)"
            required
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'name')"
        />

        <ErrorAlert :message="tm(errors.general)" />

        <div class="flex items-center justify-end gap-3 pt-2">
            <Button
                type="button"
                variant="secondary"
                :disabled="isLoading"
                @click="emit('cancel')"
            >
                {{ t('common.cancel') }}
            </Button>
            <Button
                type="submit"
                variant="primary"
                :loading="isLoading"
                :disabled="isLoading"
            >
                {{ submitLabel }}
            </Button>
        </div>
    </form>
</template>
