<script setup lang="ts">
import { computed } from 'vue';
import { useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { useI18n } from '@/app-ui/I18n';
import { Permission } from '@/app/Auth/enums/resources';
import CloseIcon from '@/app-ui/Icons/CloseIcon.vue';
import { useSidebar } from '@/app/Layout/useSidebar';

const router = useRouter();
const { isSuperAdmin, hasPermission } = useAuth();
const { isSidebarOpen, closeSidebar, closeOnMobileNav } = useSidebar();
const { t } = useI18n();

// Nav is role-aware, not permission-aware: a superadmin holds every permission,
// so gating admin links on hasPermission would still show them. A superadmin
// sees only the platform plane; admins see admin tools; users see their dashboard.
const navLinks = computed<{ name: string; label: string }[]>(() => {
    if (isSuperAdmin() === true) {
        return [
            { name: 'platform-dashboard', label: t('common.dashboard') },
            { name: 'platform-tenants', label: t('common.tenants') },
            { name: 'platform-users', label: t('common.users') },
        ];
    }

    const dashboard = hasPermission(Permission.AdminDashboardRead) === true
        ? 'admin-dashboard'
        : 'user-dashboard';
    const links = [{ name: dashboard, label: t('common.dashboard') }];

    if (hasPermission(Permission.AdminUsersRead) === true) {
        links.push({ name: 'admin-users', label: t('common.users') });
    }

    return links;
});

const sectionLabel = computed<string>(() => isSuperAdmin() === true ? t('nav.platform') : t('nav.application'));

const goHome = (): void => {
    void router.push({ name: 'home' });
};
</script>

<template>
    <!-- Mobile backdrop: the sidebar is an off-canvas overlay below md -->
    <div
        v-if="isSidebarOpen === true"
        class="fixed inset-0 z-20 bg-black/30 md:hidden"
        aria-hidden="true"
        @click="closeSidebar"
    />

    <aside
        :class="[
            'fixed inset-y-0 left-0 z-30',
            'flex flex-col',
            'bg-white border-r border-gray-200',
            'transition-all duration-200 overflow-hidden',
            isSidebarOpen === true ? 'w-64' : 'w-0',
        ]"
    >
        <div
            :class="[
                'flex items-center justify-between shrink-0',
                'h-16 px-6',
                'border-b border-gray-200',
            ]"
        >
            <button
                type="button"
                class="font-bold text-gray-900 whitespace-nowrap cursor-pointer"
                @click="goHome"
            >
                GoKick
            </button>
            <button
                type="button"
                class="md:hidden text-gray-500 hover:text-gray-700 cursor-pointer"
                :aria-label="t('nav.close_menu')"
                @click="closeSidebar"
            >
                <CloseIcon class="w-4 h-4" />
            </button>
        </div>

        <div class="flex-1 overflow-y-auto">
            <p
                :class="[
                    'px-6 pt-6 pb-2',
                    'text-xs font-semibold text-gray-400 uppercase tracking-wider',
                    'whitespace-nowrap',
                ]"
            >
                {{ sectionLabel }}
            </p>
            <nav class="px-3 space-y-1">
                <RouterLink
                    v-for="link in navLinks"
                    :key="link.name"
                    :to="{ name: link.name }"
                    :class="[
                        'flex items-center',
                        'px-3 py-2 rounded-md',
                        'text-sm font-medium text-gray-700',
                        'hover:bg-gray-100 hover:text-gray-900',
                        'transition-colors whitespace-nowrap',
                    ]"
                    active-class="!text-orange-700 !bg-orange-50"
                    @click="closeOnMobileNav"
                >
                    {{ link.label }}
                </RouterLink>
            </nav>
        </div>
    </aside>
</template>
