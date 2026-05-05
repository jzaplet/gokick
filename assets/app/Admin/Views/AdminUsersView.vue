<script setup lang="ts">
import type { AdminUser } from '@/app/Admin/types/AdminUser';
import { onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { authFetch } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Button from '@/app-ui/Buttons/Button.vue';
import PlusIcon from '@/app-ui/Icons/PlusIcon.vue';
import ConfirmModal from '@/app-ui/Modals/ConfirmModal.vue';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import UsersTable from '@/app/Admin/Components/UsersTable.vue';

const router = useRouter();
const { success, error } = useToast();

const users = ref<AdminUser[]>([]);
const isLoading = ref(true);
const userToDelete = ref<AdminUser | null>(null);

const fetchUsers = async (): Promise<void> => {
    isLoading.value = true;
    const result = await authFetch<AdminUser[]>('GET', '/api/v1/admin/users');

    isLoading.value = false;

    if (result.success === false) {
        error('Failed to load user list.');

        return;
    }

    users.value = result.data;
};

const goToCreate = (): void => {
    void router.push({ name: 'admin-users-new' });
};

const goToEdit = (user: AdminUser): void => {
    void router.push({ name: 'admin-users-edit', params: { id: user.id } });
};

const askDelete = (user: AdminUser): void => {
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
        `/api/v1/admin/users/${target.id}`,
    );

    if (result.success === false) {
        error(result.data.general ?? 'Delete failed.');

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
            <div
                :class="[
                    'flex flex-col gap-4',
                    'sm:flex-row sm:items-center sm:justify-between',
                ]"
            >
                <h1 class="text-3xl font-extrabold text-gray-900">
                    User management
                </h1>

                <Button
                    variant="primary"
                    @click="goToCreate"
                >
                    <PlusIcon class="w-4 h-4" />
                    Add user
                </Button>
            </div>

            <div
                v-if="isLoading === true"
                class="flex items-center justify-center py-12"
            >
                <Spinner />
            </div>

            <UsersTable
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
