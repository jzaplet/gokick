<script setup lang="ts">
import type { AdminUser } from '@/app/Admin/types/AdminUser';
import type { UserFormData } from '@/app/Admin/types/UserFormData';
import type { UserFormErrors } from '@/app/Admin/types/UserFormErrors';
import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import UserForm from '@/app/Admin/Components/UserForm.vue';

const router = useRouter();
const route = useRoute();
const { success, error } = useToast();

const userId = String(route.params['id']);
const initial = ref<UserFormData | null>(null);
const errors = ref<UserFormErrors>({});
const isLoading = ref(false);
const isFetching = ref(true);

const clearFieldError = (field: keyof UserFormErrors): void => {
    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
    delete errors.value[field];
};

const handleSubmit = async (data: UserFormData): Promise<void> => {
    isLoading.value = true;
    errors.value = {};

    const result = await authFetch<null, UserFormErrors>(
        'PUT',
        `/api/v1/admin/users/${userId}`,
        { body: data },
    );

    isLoading.value = false;

    if (result.success === false) {
        errors.value = result.data;

        return;
    }

    success(`Uživatel ${data.nickname} uložen.`);
    void router.push({ name: 'admin-users' });
};

const handleCancel = (): void => {
    void router.push({ name: 'admin-users' });
};

onMounted(async (): Promise<void> => {
    const result = await authFetch<AdminUser[]>('GET', '/api/v1/admin/users');

    isFetching.value = false;

    if (result.success === false) {
        error('Nepodařilo se načíst uživatele.');
        void router.push({ name: 'admin-users' });

        return;
    }

    const target = result.data.find((u) => u.id === userId);

    if (target === undefined) {
        error('Uživatel nenalezen.');
        void router.push({ name: 'admin-users' });

        return;
    }

    initial.value = {
        nickname: target.nickname,
        password: '',
        email: target.email,
        role: target.role,
    };
});
</script>

<template>
    <div class="min-h-screen bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
                Editace uživatele
            </h1>

            <div
                v-if="isFetching === true"
                class="flex items-center justify-center py-12"
            >
                <Spinner />
            </div>

            <UserForm
                v-else-if="initial !== null"
                mode="edit"
                submit-label="Uložit"
                :initial="initial"
                :is-loading="isLoading"
                :errors="errors"
                @submit="handleSubmit"
                @cancel="handleCancel"
                @clear-error="clearFieldError"
            />
        </div>
    </div>
</template>
