<script setup lang="ts">
import type { ChangePasswordErrors } from '@/app/Profile/types/ChangePasswordErrors';
import type { ChangePasswordFormData } from '@/app/Profile/types/ChangePasswordFormData';
import { reactive, ref } from 'vue';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Button from '@/app-ui/Buttons/Button.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import ErrorAlert from '@/app-ui/Alerts/ErrorAlert.vue';

const { success } = useToast();

const form: ChangePasswordFormData = reactive({
    old_password: '',
    new_password: '',
});

const errors = ref<ChangePasswordErrors>({});
const isLoading = ref(false);

const clearFieldError = (field: keyof ChangePasswordErrors): void => {
    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
    delete errors.value[field];
};

const resetForm = (): void => {
    form.old_password = '';
    form.new_password = '';
};

const handleSubmit = async (): Promise<void> => {
    isLoading.value = true;
    errors.value = {};

    const result = await authFetch<null, ChangePasswordErrors>(
        'PUT',
        '/api/v1/profile/password',
        { body: form },
    );

    isLoading.value = false;

    if (result.success === false) {
        errors.value = result.data;

        return;
    }

    success('Password changed.');
    resetForm();
};
</script>

<template>
    <div class="bg-white rounded-lg shadow-md p-6 space-y-4">
        <h2 class="text-lg font-semibold text-gray-900">
            Change password
        </h2>

        <form
            class="space-y-4"
            @submit.prevent="handleSubmit"
        >
            <Input
                v-model="form.old_password"
                name="old_password"
                type="password"
                label="Current password"
                :error="errors.old_password"
                required
                :disabled="isLoading"
                @update:model-value="() => clearFieldError('old_password')"
            />

            <Input
                v-model="form.new_password"
                name="new_password"
                type="password"
                label="New password"
                :error="errors.new_password"
                required
                :disabled="isLoading"
                @update:model-value="() => clearFieldError('new_password')"
            />

            <ErrorAlert :message="errors.general" />

            <Button
                type="submit"
                variant="primary"
                size="md"
                :loading="isLoading"
                :disabled="isLoading"
            >
                <span v-if="isLoading === false">Change password</span>
                <span v-else>Saving...</span>
            </Button>
        </form>
    </div>
</template>
