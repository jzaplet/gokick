<script setup lang="ts">
import type { UserFormData } from '@/app/Admin/types/UserFormData';
import type { UserFormErrors } from '@/app/Admin/types/UserFormErrors';
import { reactive, ref } from 'vue';
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

const form: UserFormData = reactive({
    nickname: initial.nickname ?? '',
    password: '',
    email: initial.email ?? '',
    role: initial.role ?? 'user',
});

const roleOptions = ref([
    { value: 'user', label: 'User' },
    { value: 'admin', label: 'Admin' },
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
            label="Nickname"
            :error="errors.nickname"
            required
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'nickname')"
        />

        <Input
            v-model="form.password"
            name="password"
            type="password"
            :label="mode === 'create' ? 'Password' : 'Password (leave empty to keep current)'"
            :error="errors.password"
            :required="mode === 'create'"
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'password')"
        />

        <Input
            v-model="form.email"
            name="email"
            type="email"
            label="Email (optional)"
            :error="errors.email"
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'email')"
        />

        <Select
            v-model="form.role"
            name="role"
            label="Role"
            :options="roleOptions"
            :error="errors.role"
            required
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'role')"
        />

        <ErrorAlert :message="errors.general" />

        <div class="flex items-center justify-end gap-3 pt-2">
            <Button
                type="button"
                variant="secondary"
                :disabled="isLoading"
                @click="emit('cancel')"
            >
                Cancel
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
