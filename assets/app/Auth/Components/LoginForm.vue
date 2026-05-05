<script setup lang="ts">
import type { LoginErrors } from '@/app/Auth/types/LoginErrors';
import type { LoginRequest } from '@/app-ui/Auth';
import { reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Button from '@/app-ui/Buttons/Button.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import ErrorAlert from '@/app-ui/Alerts/ErrorAlert.vue';

const router = useRouter();
const route = useRoute();
const { login } = useAuth();
const { success } = useToast();

const form: LoginRequest = reactive({
    nickname: '',
    password: '',
});

const errors = ref<LoginErrors>({});
const isLoading = ref(false);

const clearFieldError = (field: keyof LoginErrors): void => {
    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
    delete errors.value[field];
};

const handleSubmit = async (): Promise<void> => {
    isLoading.value = true;
    errors.value = {};

    const result = await login<LoginErrors>(form);

    if (result.success === false) {
        isLoading.value = false;
        errors.value = result.data;

        return;
    }

    success(`Welcome back, ${result.data.user.nickname}.`);

    const redirectQuery = route.query['redirect'];
    const defaultByRole = result.data.user.role === 'admin'
        ? '/admin/dashboard'
        : '/user/dashboard';
    const target = typeof redirectQuery === 'string' ? redirectQuery : defaultByRole;

    await router.push(target);
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
                label="Nickname"
                placeholder="admin"
                :error="errors.nickname"
                required
                :disabled="isLoading"
                @update:model-value="() => clearFieldError('nickname')"
            />

            <Input
                v-model="form.password"
                name="password"
                type="password"
                label="Password"
                :error="errors.password"
                required
                :disabled="isLoading"
                @update:model-value="() => clearFieldError('password')"
            />
        </div>

        <ErrorAlert :message="errors.general" />

        <Button
            type="submit"
            variant="primary"
            size="lg"
            class="w-full"
            :loading="isLoading"
            :disabled="isLoading"
        >
            <span v-if="isLoading === false">Sign in</span>
            <span v-else>Signing in...</span>
        </Button>
    </form>
</template>
