<script setup lang="ts">
import type { UserFormData } from '@/app/Admin/types/UserFormData';
import type { UserFormErrors } from '@/app/Admin/types/UserFormErrors';
import { Role } from '@/app/Auth/enums/roles';
import { roleLabel } from '@/app/Auth/enums/roleLabels';
import { computed, reactive } from 'vue';
import { tm, useI18n } from '@/app-ui/I18n';
import Button from '@/app-ui/Buttons/Button.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import Select from '@/app-ui/Inputs/Select.vue';
import ErrorAlert from '@/app-ui/Alerts/ErrorAlert.vue';

const {
    initial = {},
    mode,
    submitLabel,
    isLoading,
    errors,
} = defineProps<{
    initial?: Partial<UserFormData>;
    mode: 'create' | 'edit';
    submitLabel: string;
    isLoading: boolean;
    errors: UserFormErrors;
}>();

const emit = defineEmits<{
    submit: [data: UserFormData];
    cancel: [];
    clearError: [field: keyof UserFormErrors];
}>();

const { t } = useI18n();

const form: UserFormData = reactive({
    nickname: initial.nickname ?? '',
    password: '',
    email: initial.email ?? '',
    role: initial.role ?? Role.User,
});

const roleOptions = computed(() => [
    { value: Role.User, label: roleLabel(Role.User) },
    { value: Role.Admin, label: roleLabel(Role.Admin) },
]);

const handleSubmit = (): void => {
    emit('submit', { ...form });
};
</script>

<template>
    <form
        class="bg-white rounded-lg shadow-md p-6 space-y-4"
        @submit.prevent="handleSubmit"
    >
        <Input
            v-model="form.nickname"
            name="nickname"
            type="text"
            :label="t('auth.nickname')"
            :error="tm(errors.nickname)"
            required
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'nickname')"
        />

        <Input
            v-model="form.password"
            name="password"
            type="password"
            :label="mode === 'create' ? t('auth.password') : t('users.password_keep')"
            :error="tm(errors.password)"
            :required="mode === 'create'"
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'password')"
        />

        <Input
            v-model="form.email"
            name="email"
            type="email"
            :label="t('users.email_optional')"
            :error="tm(errors.email)"
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'email')"
        />

        <Select
            v-model="form.role"
            name="role"
            :label="t('common.role')"
            :options="roleOptions"
            :error="tm(errors.role)"
            required
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'role')"
        />

        <ErrorAlert :message="tm(errors.general)" />

        <div class="flex items-center justify-end gap-3 pt-2">
            <Button
                type="button"
                variant="secondary"
                :disabled="isLoading"
                @click="emit('cancel')"
            >
                {{ t('common.cancel') }}
            </Button>
            <Button
                type="submit"
                variant="primary"
                :loading="isLoading"
                :disabled="isLoading"
            >
                {{ submitLabel }}
            </Button>
        </div>
    </form>
</template>
