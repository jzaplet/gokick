<script setup lang="ts">
import type { UserFormData } from '@/app/Admin/types/UserFormData';
import type { UserFormErrors } from '@/app/Admin/types/UserFormErrors';
import { ref } from 'vue';
import { useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import UserForm from '@/app/Admin/Components/UserForm.vue';

const router = useRouter();
const { success } = useToast();

const errors = ref<UserFormErrors>({});
const isLoading = ref(false);

const clearFieldError = (field: keyof UserFormErrors): void => {
    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
    delete errors.value[field];
};

const handleSubmit = async (data: UserFormData): Promise<void> => {
    isLoading.value = true;
    errors.value = {};

    const result = await authFetch<null, UserFormErrors>(
        'POST',
        '/api/v1/admin/users',
        { body: data },
    );

    isLoading.value = false;

    if (result.success === false) {
        errors.value = result.data;

        return;
    }

    success(`User ${data.nickname} created.`);
    void router.push({ name: 'admin-users' });
};

const handleCancel = (): void => {
    void router.push({ name: 'admin-users' });
};
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
                New user
            </h1>

            <UserForm
                mode="create"
                submit-label="Create"
                :is-loading="isLoading"
                :errors="errors"
                @submit="handleSubmit"
                @cancel="handleCancel"
                @clear-error="clearFieldError"
            />
        </div>
    </div>
</template>
