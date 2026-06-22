<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import { Permission } from '@/app/Auth/enums/resources';
import Dropdown from '@/app-ui/Dropdown/Dropdown.vue';
import UserIcon from '@/app-ui/Icons/UserIcon.vue';

const router = useRouter();
const { success } = useToast();
const { user, isSuperAdmin, hasPermission, logout } = useAuth();

// Nav is role-aware, not permission-aware: a superadmin holds every permission,
// so gating admin links on hasPermission would still show them. A superadmin
// sees only the platform plane; admins see admin tools; users see their dashboard.
const navLinks = computed<{ name: string; label: string }[]>(() => {
    if (isSuperAdmin() === true) {
        return [
            { name: 'platform-dashboard', label: 'Dashboard' },
            { name: 'platform-tenants', label: 'Tenants' },
            { name: 'platform-users', label: 'Users' },
        ];
    }

    const dashboard = hasPermission(Permission.AdminDashboardRead) === true
        ? 'admin-dashboard'
        : 'user-dashboard';
    const links = [{ name: dashboard, label: 'Dashboard' }];

    if (hasPermission(Permission.AdminUsersRead) === true) {
        links.push({ name: 'admin-users', label: 'Users' });
    }

    return links;
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
                    v-for="link in navLinks"
                    :key="link.name"
                    :to="{ name: link.name }"
                    :class="[
                        'px-3 py-1.5 rounded-md text-sm font-medium',
                        'text-gray-700 hover:text-gray-900 hover:bg-gray-100',
                    ]"
                    active-class="!text-orange-700 !bg-orange-50"
                >
                    {{ link.label }}
                </RouterLink>
            </nav>

            <Dropdown v-if="user !== null">
                <template #trigger>
                    <button
                        type="button"
                        :class="[
                            'flex items-center justify-center cursor-pointer',
                            'w-9 h-9 rounded-full',
                            'border border-gray-300 text-gray-600',
                            'hover:bg-gray-50 hover:border-gray-400 transition-colors',
                        ]"
                        aria-label="Account menu"
                    >
                        <UserIcon class="w-5 h-5" />
                    </button>
                </template>

                <div class="px-4 py-3">
                    <p class="text-sm font-semibold text-gray-900 truncate">
                        {{ user.nickname }}
                    </p>
                    <p
                        v-if="user.email !== ''"
                        class="text-sm text-gray-500 truncate"
                    >
                        {{ user.email }}
                    </p>
                </div>

                <div class="border-t border-gray-100" />

                <RouterLink
                    :to="{ name: 'profile' }"
                    class="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100"
                >
                    Profile settings
                </RouterLink>
                <button
                    type="button"
                    :class="[
                        'block w-full text-left cursor-pointer',
                        'px-4 py-2 text-sm text-red-600 hover:bg-red-50',
                    ]"
                    @click="handleLogout"
                >
                    Sign out
                </button>
            </Dropdown>
        </div>

        <nav
            :class="[
                'sm:hidden',
                'border-t border-gray-100',
                'flex items-center justify-center gap-1 py-2',
            ]"
        >
            <RouterLink
                v-for="link in navLinks"
                :key="link.name"
                :to="{ name: link.name }"
                :class="[
                    'px-3 py-1.5 rounded-md text-sm font-medium',
                    'text-gray-700 hover:text-gray-900 hover:bg-gray-100',
                ]"
                active-class="!text-orange-700 !bg-orange-50"
            >
                {{ link.label }}
            </RouterLink>
        </nav>
    </header>
</template>
