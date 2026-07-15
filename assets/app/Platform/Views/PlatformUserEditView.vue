<script setup lang="ts">
import type { PlatformUser } from '@/app/Platform/types/PlatformUser';
import { isPlatformUser } from '@/app/Platform/types/PlatformUser';
import type { PlatformUserFormData } from '@/app/Platform/types/PlatformUserFormData';
import type { PlatformUserFormErrors } from '@/app/Platform/types/PlatformUserFormErrors';
import { onMounted, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import PlatformUserForm from '@/app/Platform/Components/PlatformUserForm.vue';

const router = useRouter();
const route = useRoute();
const { success, error } = useToast();

const userId = String(route.params['id']);
const initial = ref<PlatformUserFormData | null>(null);
const errors = ref<PlatformUserFormErrors>({});
const isLoading = ref(false);
const isFetching = ref(true);

const clearFieldError = (field: keyof PlatformUserFormErrors): void => {
    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
    delete errors.value[field];
};

const handleSubmit = async (data: PlatformUserFormData): Promise<void> => {
    isLoading.value = true;
    errors.value = {};

    const result = await authFetch<null, PlatformUserFormErrors, PlatformUserFormData>(
        'PUT',
        `/api/v1/platform/users/${userId}`,
        { body: data },
    );

    isLoading.value = false;

    if (result.success === false) {
        errors.value = result.data;

        return;
    }

    success(`User ${data.nickname} saved.`);
    void router.push({ name: 'platform-users' });
};

const handleCancel = (): void => {
    void router.push({ name: 'platform-users' });
};

onMounted(async (): Promise<void> => {
    const result = await authFetch<PlatformUser>('GET', `/api/v1/platform/users/${userId}`, {
        validate: isPlatformUser,
    });

    isFetching.value = false;

    if (result.success === false) {
        // A missing id comes back as a 400 from the read-one endpoint — the same
        // redirect as any load failure.
        error('Failed to load user.');
        void router.push({ name: 'platform-users' });

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
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
                Edit user
            </h1>

            <div
                v-if="isFetching === true"
                class="flex items-center justify-center py-12"
            >
                <Spinner />
            </div>

            <PlatformUserForm
                v-else-if="initial !== null"
                mode="edit"
                submit-label="Save"
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
