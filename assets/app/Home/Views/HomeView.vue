<script setup lang="ts">
import { useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import { Permission } from '@/app/Auth/enums/resources';
import Button from '@/app-ui/Buttons/Button.vue';

const router = useRouter();
const { success } = useToast();
const { user, isAuthenticated, logout, hasPermission } = useAuth();

const goToLogin = (): void => {
    void router.push({ name: 'login' });
};

const goToDashboard = (): void => {
    const name = hasPermission(Permission.AdminDashboardRead) === true
        ? 'admin-dashboard'
        : 'user-dashboard';

    void router.push({ name });
};

const handleLogout = async (): Promise<void> => {
    await logout();
    success('Signed out.');
};
</script>

<template>
    <div class="flex flex-col items-center justify-center min-h-screen p-10">
        <img
            src="@/img/go-vue-cqrs-ddd.png"
            alt="Go Vue CQRS DDD Logo"
            class="max-w-full max-h-[50vh] object-contain"
        >

        <div
            v-if="isAuthenticated === false"
            class="mt-8"
        >
            <Button
                variant="primary"
                size="lg"
                @click="goToLogin"
            >
                Sign in
            </Button>
        </div>

        <div
            v-else
            class="mt-8 flex flex-col items-center gap-3"
        >
            <p
                v-if="user !== null"
                class="text-sm text-gray-600"
            >
                Signed in as <strong class="text-gray-900">{{ user.nickname }}</strong>
            </p>

            <div class="flex flex-wrap items-center justify-center gap-3">
                <Button
                    variant="primary"
                    @click="goToDashboard"
                >
                    Dashboard
                </Button>

                <Button
                    variant="secondary"
                    @click="handleLogout"
                >
                    Sign out
                </Button>
            </div>
        </div>
    </div>
</template>
