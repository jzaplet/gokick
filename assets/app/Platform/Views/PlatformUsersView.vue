<script setup lang="ts">
import type { PlatformUser } from '@/app/Platform/types/PlatformUser';
import { isPlatformUser } from '@/app/Platform/types/PlatformUser';
import { arrayOf, isTransport } from '@/app-ui/Fetch';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import ConfirmModal from '@/app-ui/Modals/ConfirmModal.vue';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import PlatformUsersTable from '@/app/Platform/Components/PlatformUsersTable.vue';

const router = useRouter();
const { success, error } = useToast();

const users = ref<PlatformUser[]>([]);
const isLoading = ref(true);
const userToDelete = ref<PlatformUser | null>(null);

const fetchUsers = async (): Promise<void> => {
    isLoading.value = true;
    const result = await authFetch<PlatformUser[]>('GET', '/api/v1/platform/users', {
        validate: arrayOf(isPlatformUser),
    });

    isLoading.value = false;

    if (result.success === false) {
        error('Failed to load user list.');

        return;
    }

    users.value = result.data;
};

const goToEdit = (user: PlatformUser): void => {
    void router.push({ name: 'platform-users-edit', params: { id: user.id } });
};

const askDelete = (user: PlatformUser): void => {
    userToDelete.value = user;
};

const cancelDelete = (): void => {
    userToDelete.value = null;
};

const confirmDelete = async (): Promise<void> => {
    if (userToDelete.value === null) {
        return;
    }

    const target = userToDelete.value;

    userToDelete.value = null;

    const result = await authFetch<null, { general?: string }>(
        'DELETE',
        `/api/v1/platform/users/${target.id}`,
    );

    if (result.success === false) {
        error(isTransport(result) ? result.message : result.data.general ?? 'Delete failed.');

        return;
    }

    success(`User ${target.nickname} deleted.`);
    await fetchUsers();
};

onMounted(async (): Promise<void> => {
    await fetchUsers();
});
</script>

<template>
    <div class="py-12 px-4 sm:px-6 lg:px-8">
        <div class="max-w-5xl mx-auto space-y-6">
            <h1 class="text-3xl font-extrabold text-gray-900">
                Users
            </h1>

            <div
                v-if="isLoading === true"
                class="flex items-center justify-center py-12"
            >
                <Spinner />
            </div>

            <PlatformUsersTable
                v-else
                :users="users"
                @edit="goToEdit"
                @delete="askDelete"
            />

            <ConfirmModal
                :show="userToDelete !== null"
                title="Delete user"
                :message="userToDelete === null
                    ? ''
                    : `Really delete user ${userToDelete.nickname}? This action is irreversible.`"
                confirm-text="Delete"
                cancel-text="Cancel"
                @confirm="confirmDelete"
                @cancel="cancelDelete"
            />
        </div>
    </div>
</template>
