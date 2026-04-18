<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Button from '@/app-ui/Buttons/Button.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import ErrorAlert from '@/app-ui/Alerts/ErrorAlert.vue';

type LoginFormData = {
    nickname: string;
    password: string;
};

// general = non-field errors (auth, rate-limit, …)
// nickname / password = ValidationError with matching Field
type LoginErrors = {
    general?: string;
    nickname?: string;
    password?: string;
};

const router = useRouter();
const route = useRoute();
const { login } = useAuth();
const { success } = useToast();

const form: LoginFormData = reactive({
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

    success(`Vítej zpátky, ${result.data.user.nickname}.`);

    const redirectQuery = route.query['redirect'];
    const target = typeof redirectQuery === 'string' ? redirectQuery : '/';

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
                label="Přezdívka"
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
                label="Heslo"
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
            :loading="isLoading"
            :disabled="isLoading"
        >
            <span v-if="isLoading === false">Přihlásit se</span>
            <span v-else>Přihlašuji...</span>
        </Button>
    </form>
</template>
