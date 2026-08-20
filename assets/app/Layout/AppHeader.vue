<script setup lang="ts">
import { useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { useI18n } from '@/app-ui/I18n';
import LangSwitcher from '@/app-ui/I18n/LangSwitcher.vue';
import { useToast } from '@/app-ui/Toast/useToast';
import Dropdown from '@/app-ui/Dropdown/Dropdown.vue';
import UserIcon from '@/app-ui/Icons/UserIcon.vue';
import MenuIcon from '@/app-ui/Icons/MenuIcon.vue';
import ChevronLeftIcon from '@/app-ui/Icons/ChevronLeftIcon.vue';
import { useSidebar } from '@/app/Layout/useSidebar';

const router = useRouter();
const { success } = useToast();
const { user, logout } = useAuth();
const { isSidebarOpen, toggleSidebar } = useSidebar();
const { t } = useI18n();

const handleLogout = async (): Promise<void> => {
    await logout();
    success(t('auth.signed_out'));
    void router.push({ name: 'home' });
};
</script>

<template>
    <header
        :class="[
            'sticky top-0 z-20',
            'flex items-center justify-between',
            'h-16 px-4 sm:px-6',
            'bg-white border-b border-gray-200',
        ]"
    >
        <button
            type="button"
            :class="[
                'flex items-center justify-center cursor-pointer',
                'w-9 h-9 rounded-md',
                'text-gray-500 hover:text-gray-700 hover:bg-gray-100 transition-colors',
            ]"
            :aria-label="isSidebarOpen === true ? t('nav.collapse_menu') : t('nav.open_menu')"
            @click="toggleSidebar"
        >
            <ChevronLeftIcon
                v-if="isSidebarOpen === true"
                class="w-5 h-5"
            />
            <MenuIcon
                v-else
                class="w-5 h-5"
            />
        </button>

        <div class="flex items-center gap-3">
            <LangSwitcher />

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
                        :aria-label="t('nav.account_menu')"
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
                    {{ t('nav.profile_settings') }}
                </RouterLink>
                <button
                    type="button"
                    :class="[
                        'block w-full text-left cursor-pointer',
                        'px-4 py-2 text-sm text-red-600 hover:bg-red-50',
                    ]"
                    @click="handleLogout"
                >
                    {{ t('common.sign_out') }}
                </button>
            </Dropdown>
        </div>
    </header>
</template>
