<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Button from '@/app-ui/Buttons/Button.vue';

const router = useRouter();
const { success } = useToast();
const { user, hasPermission, logout } = useAuth();

const dashboardRoute = computed<string>(() => {
    return hasPermission('admin:dashboard:read') === true
        ? 'admin-dashboard'
        : 'user-dashboard';
});

const goHome = (): void => {
    void router.push({ name: 'home' });
};

const handleLogout = async (): Promise<void> => {
    await logout();
    success('Signed out.');
    void router.push({ name: 'home' });
};
</script>

<template>
    <header class="bg-white border-b border-gray-200">
        <div
            :class="[
                'max-w-6xl mx-auto',
                'px-4 sm:px-6 lg:px-8',
                'h-14 flex items-center justify-between gap-4',
            ]"
        >
            <button
                type="button"
                class="font-bold text-gray-900 whitespace-nowrap cursor-pointer"
                @click="goHome"
            >
                GoKick
            </button>

            <nav class="hidden sm:flex items-center gap-1">
                <RouterLink
                    :to="{ name: dashboardRoute }"
                    :class="[
                        'px-3 py-1.5 rounded-md text-sm font-medium',
                        'text-gray-700 hover:text-gray-900 hover:bg-gray-100',
                    ]"
                    active-class="!text-orange-700 !bg-orange-50"
                >
                    Dashboard
                </RouterLink>
                <RouterLink
                    :to="{ name: 'profile' }"
                    :class="[
                        'px-3 py-1.5 rounded-md text-sm font-medium',
                        'text-gray-700 hover:text-gray-900 hover:bg-gray-100',
                    ]"
                    active-class="!text-orange-700 !bg-orange-50"
                >
                    Profile
                </RouterLink>
                <RouterLink
                    v-if="hasPermission('admin:users:read') === true"
                    :to="{ name: 'admin-users' }"
                    :class="[
                        'px-3 py-1.5 rounded-md text-sm font-medium',
                        'text-gray-700 hover:text-gray-900 hover:bg-gray-100',
                    ]"
                    active-class="!text-orange-700 !bg-orange-50"
                >
                    Users
                </RouterLink>
            </nav>

            <div class="flex items-center gap-3">
                <span
                    v-if="user !== null"
                    class="hidden sm:inline text-sm text-gray-600"
                >
                    {{ user.nickname }}
                </span>
                <Button
                    variant="ghost"
                    size="sm"
                    @click="handleLogout"
                >
                    Sign out
                </Button>
            </div>
        </div>

        <nav
            :class="[
                'sm:hidden',
                'border-t border-gray-100',
                'flex items-center justify-center gap-1 py-2',
            ]"
        >
            <RouterLink
                :to="{ name: dashboardRoute }"
                :class="[
                    'px-3 py-1.5 rounded-md text-sm font-medium',
                    'text-gray-700 hover:text-gray-900 hover:bg-gray-100',
                ]"
                active-class="!text-orange-700 !bg-orange-50"
            >
                Dashboard
            </RouterLink>
            <RouterLink
                :to="{ name: 'profile' }"
                :class="[
                    'px-3 py-1.5 rounded-md text-sm font-medium',
                    'text-gray-700 hover:text-gray-900 hover:bg-gray-100',
                ]"
                active-class="!text-orange-700 !bg-orange-50"
            >
                Profile
            </RouterLink>
            <RouterLink
                v-if="hasPermission('admin:users:read') === true"
                :to="{ name: 'admin-users' }"
                :class="[
                    'px-3 py-1.5 rounded-md text-sm font-medium',
                    'text-gray-700 hover:text-gray-900 hover:bg-gray-100',
                ]"
                active-class="!text-orange-700 !bg-orange-50"
            >
                Users
            </RouterLink>
        </nav>
    </header>
</template>
