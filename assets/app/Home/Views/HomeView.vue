<script setup lang="ts">
import type { HealthResponse } from '@/app/Home/types/HealthResponse';
import { onMounted } from 'vue';
import { useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { apiFetch } from '@/app-ui/Fetch';
import { useToast } from '@/app-ui/Toast/useToast';
import Button from '@/app-ui/Buttons/Button.vue';

const router = useRouter();
const { success, error } = useToast();
const { user, isAuthenticated, logout, hasPermission } = useAuth();

const goToLogin = (): void => {
    void router.push({ name: 'login' });
};

const goToProfile = (): void => {
    void router.push({ name: 'profile' });
};

const goToDashboard = (): void => {
    const name = hasPermission('admin:dashboard:read') === true
        ? 'admin-dashboard'
        : 'user-dashboard';

    void router.push({ name });
};

const goToAdminUsers = (): void => {
    void router.push({ name: 'admin-users' });
};

const handleLogout = async (): Promise<void> => {
    await logout();
    success('Odhlášení proběhlo.');
};

onMounted(async (): Promise<void> => {
    const result = await apiFetch<HealthResponse>('GET', '/health');

    if (result.success === true) {
        success(`API Test status: ${result.data.status}`);
    } else {
        error(`API Test Error ${String(result.status)}`);
    }
});
</script>

<template>
    <div class="flex flex-col items-center justify-center min-h-screen p-10">
        <h1 class="text-4xl font-bold text-gray-900 text-center">
            This is your brand new app!
        </h1>
        <img
            src="@/img/go-vue-cqrs-ddd.png"
            alt="Go Vue CQRS DDD Logo"
            class="mt-8 max-w-full max-h-[50vh] object-contain"
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
                Přihlásit se
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
                Přihlášen jako <strong class="text-gray-900">{{ user.nickname }}</strong>
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
                    @click="goToProfile"
                >
                    Můj profil
                </Button>

                <Button
                    v-if="hasPermission('admin:users:read') === true"
                    variant="secondary"
                    @click="goToAdminUsers"
                >
                    Správa uživatelů
                </Button>

                <Button
                    variant="ghost"
                    @click="handleLogout"
                >
                    Odhlásit se
                </Button>
            </div>
        </div>
    </div>
</template>
