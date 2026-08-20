<script setup lang="ts">
import { useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { useI18n } from '@/app-ui/I18n';
import LangSwitcher from '@/app-ui/I18n/LangSwitcher.vue';
import { useToast } from '@/app-ui/Toast/useToast';
import { homeForRole } from '@/router/homeForRole';
import Button from '@/app-ui/Buttons/Button.vue';

const router = useRouter();
const { success } = useToast();
const { t } = useI18n();
const { user, isAuthenticated, logout } = useAuth();

const goToLogin = (): void => {
    void router.push({ name: 'login' });
};

const goToDashboard = (): void => {
    if (user.value === null) {
        return;
    }

    void router.push(homeForRole(user.value.role));
};

const handleLogout = async (): Promise<void> => {
    await logout();
    success(t('auth.signed_out'));
};
</script>

<template>
    <div class="relative flex flex-col items-center justify-center min-h-screen p-10">
        <div class="absolute top-4 right-4">
            <LangSwitcher />
        </div>

        <img
            src="@/img/go-vue-cqrs-ddd.png"
            :alt="t('home.logo_alt')"
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
                {{ t('common.sign_in') }}
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
                {{ t('home.signed_in_as') }} <strong class="text-gray-900">{{ user.nickname }}</strong>
            </p>

            <div class="flex flex-wrap items-center justify-center gap-3">
                <Button
                    variant="primary"
                    @click="goToDashboard"
                >
                    {{ t('common.dashboard') }}
                </Button>

                <Button
                    variant="secondary"
                    @click="handleLogout"
                >
                    {{ t('common.sign_out') }}
                </Button>
            </div>
        </div>
    </div>
</template>
