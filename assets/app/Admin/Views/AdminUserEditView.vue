<script setup lang="ts">
import type { AdminUser } from '@/app/Admin/types/AdminUser';
import { isAdminUser } from '@/app/Admin/types/AdminUser';
import type { UserFormData } from '@/app/Admin/types/UserFormData';
import type { UserFormErrors } from '@/app/Admin/types/UserFormErrors';
import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { authFetch, useAuth } from '@/app-ui/Auth';
import { getLocale, localizePath, useI18n } from '@/app-ui/I18n';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import UserForm from '@/app/Admin/Components/UserForm.vue';

const router = useRouter();
const route = useRoute();
const { success, error } = useToast();
const { user: currentUser } = useAuth();
const { t } = useI18n();

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

    const result = await authFetch<null, UserFormErrors, UserFormData>(
        'PUT',
        `/api/v1/admin/users/${userId}`,
        { body: data },
    );

    isLoading.value = false;

    if (result.success === false) {
        errors.value = result.data;

        return;
    }

    const isSelf = currentUser.value !== null && currentUser.value.id === userId;
    const roleChanged = initial.value !== null && initial.value.role !== data.role;

    if (isSelf === true && roleChanged === true) {
        // Force a full page reload so bootstrap re-runs `refresh()` and the
        // SPA picks up the new role/permissions from a fresh access token.
        // A simple router.push would keep the stale JWT in memory. The target
        // is rebased onto the current locale — a bare '/admin/users' would
        // drop a /cs prefix and flip the language across the reload.
        window.location.assign(localizePath('/admin/users', getLocale()));

        return;
    }

    success(t('users.saved', { nickname: data.nickname }));
    void router.push({ name: 'admin-users' });
};

const handleCancel = (): void => {
    void router.push({ name: 'admin-users' });
};

onMounted(async (): Promise<void> => {
    const result = await authFetch<AdminUser>('GET', `/api/v1/admin/users/${userId}`, {
        validate: isAdminUser,
    });

    isFetching.value = false;

    if (result.success === false) {
        // A missing / cross-tenant / superadmin id comes back as a 400 from the
        // read-one endpoint — the same redirect as any load failure.
        error(t('users.load_one_failed'));
        void router.push({ name: 'admin-users' });

        return;
    }

    initial.value = {
        nickname: result.data.nickname,
        password: '',
        email: result.data.email,
        role: result.data.role,
    };
});
</script>

<template>
    <div>
        <div class="max-w-xl mx-auto space-y-6">
            <h1 class="text-2xl font-bold text-gray-900">
                {{ t('users.edit_title') }}
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
                :submit-label="t('common.save')"
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
