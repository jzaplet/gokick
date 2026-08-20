<script setup lang="ts">
import type { LoginErrors } from '@/app/Auth/types/LoginErrors';
import type { LoginRequest } from '@/app-ui/Auth';
import { reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { homeForRole } from '@/router/homeForRole';
import { useAuth } from '@/app-ui/Auth';
import { getLocale, localizePath, tm, useI18n } from '@/app-ui/I18n';
import { useToast } from '@/app-ui/Toast/useToast';
import Button from '@/app-ui/Buttons/Button.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import ErrorAlert from '@/app-ui/Alerts/ErrorAlert.vue';

const router = useRouter();
const route = useRoute();
const { login } = useAuth();
const { success } = useToast();
const { t } = useI18n();

const form: LoginRequest = reactive({
    nickname: '',
    password: '',
});

const errors = ref<LoginErrors>({});
const isLoading = ref(false);

const handleSubmit = async (): Promise<void> => {
    isLoading.value = true;
    errors.value = {};

    const result = await login<LoginErrors>(form);

    if (result.success === false) {
        isLoading.value = false;
        errors.value = result.data;

        return;
    }

    success(t('auth.welcome_back', { nickname: result.data.user.nickname }));

    const redirectQuery = route.query['redirect'];
    const defaultByRole = homeForRole(result.data.user.role);

    // Only accept a same-origin absolute path: reject protocol-relative
    // (`//evil.com`) and any non-string so a crafted ?redirect= can't bounce the
    // user off-site (open redirect). The narrowing lives in the ternary so no cast.
    const safeRedirect = (query: typeof redirectQuery): string | null =>
        typeof query === 'string'
        && query.startsWith('/') === true
        && query.startsWith('//') === false
            ? query
            : null;
    const target = safeRedirect(redirectQuery) ?? defaultByRole;

    // Rebase onto the CURRENT locale: a stored ?redirect (captured before an
    // explicit language switch on the login page) still carries the old
    // prefix, and pushing it as-is would re-apply the old language.
    await router.push(localizePath(target, getLocale()));
};
</script>

<template>
    <form
        class="space-y-6"
        @submit.prevent="handleSubmit"
    >
        <div class="space-y-4">
            <Input
                v-model="form.nickname"
                name="nickname"
                type="text"
                :label="t('auth.nickname')"
                :placeholder="t('auth.nickname_placeholder')"
                required
                :disabled="isLoading"
            />

            <Input
                v-model="form.password"
                name="password"
                type="password"
                :label="t('auth.password')"
                required
                :disabled="isLoading"
            />
        </div>

        <ErrorAlert :message="tm(errors.general)" />

        <Button
            type="submit"
            variant="primary"
            size="lg"
            class="w-full"
            :loading="isLoading"
            :disabled="isLoading"
        >
            <span v-if="isLoading === false">{{ t('common.sign_in') }}</span>
            <span v-else>{{ t('auth.signing_in') }}</span>
        </Button>
    </form>
</template>
