<script setup lang="ts">
import { reactive, ref } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useAuth } from '@/app-ui/Auth';
import { useToast } from '@/app-ui/Toast/useToast';
import Button from '@/app-ui/Buttons/Button.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import Spinner from '@/app-ui/Loading/Spinner.vue';
import ErrorIcon from '@/app-ui/Icons/ErrorIcon.vue';

type LoginForm = {
    nickname: string;
    password: string;
};

// Všechny chyby jako plain string — prázdný string znamená "bez chyby".
// Vyhýbáme se `undefined` kvůli exactOptionalPropertyTypes a `as` castům.
type LoginErrors = {
    general: string;
    nickname: string;
    password: string;
};

const router = useRouter();
const route = useRoute();
const { login } = useAuth();
const { success } = useToast();

const form: LoginForm = reactive({
    nickname: '',
    password: '',
});

const errors: LoginErrors = reactive({
    general: '',
    nickname: '',
    password: '',
});

const isLoading = ref(false);

const clearFieldError = (field: keyof LoginErrors): void => {
    errors[field] = '';
};

const handleSubmit = async (): Promise<void> => {
    isLoading.value = true;
    errors.general = '';
    errors.nickname = '';
    errors.password = '';

    const result = await login(form);

    if (result.success === false) {
        isLoading.value = false;
        errors.general = result.data.message;

        return;
    }

    success(`Vítej zpátky, ${result.data.user.nickname}.`);

    const redirectQuery = route.query['redirect'];
    const target = typeof redirectQuery === 'string' ? redirectQuery : '/';

    await router.push(target);
};
</script>

<template>
    <div class="min-h-screen bg-gray-50 flex items-center justify-center px-4 sm:px-6 lg:px-8">
        <div class="max-w-md w-full space-y-8">
            <div class="text-center">
                <h2 class="text-3xl font-extrabold text-gray-900">
                    Přihlášení
                </h2>
                <p class="mt-2 text-sm text-gray-600">
                    Zadejte své přihlašovací údaje
                </p>
            </div>

            <form
                class="mt-8 space-y-6"
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

                <div
                    v-if="errors.general !== ''"
                    class="bg-red-50 border border-red-200 rounded-lg p-4"
                >
                    <div class="flex">
                        <div class="flex-shrink-0 text-red-800">
                            <ErrorIcon class="w-5 h-5" />
                        </div>
                        <div class="ml-3">
                            <p class="text-sm font-medium text-red-800">
                                {{ errors.general }}
                            </p>
                        </div>
                    </div>
                </div>

                <Button
                    type="submit"
                    variant="primary"
                    size="lg"
                    :loading="isLoading"
                    :disabled="isLoading"
                >
                    <span v-if="isLoading === false">Přihlásit se</span>
                    <span
                        v-else
                        class="flex items-center"
                    >
                        <Spinner
                            size="sm"
                            color="white"
                            class="mr-2"
                        />
                        Přihlašuji...
                    </span>
                </Button>
            </form>
        </div>
    </div>
</template>
