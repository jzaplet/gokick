<script setup lang="ts">
import type { PlatformUserCreateData } from '@/app/Platform/types/PlatformUserCreateData';
import type { PlatformUserFormErrors } from '@/app/Platform/types/PlatformUserFormErrors';
import { Role } from '@/app/Auth/enums/roles';
import { reactive, ref } from 'vue';
import Button from '@/app-ui/Buttons/Button.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import Select from '@/app-ui/Inputs/Select.vue';
import ErrorAlert from '@/app-ui/Alerts/ErrorAlert.vue';

// Platform's own user form — a deliberate independent copy of Admin's UserForm
// (F-084, direction B: keep the domains decoupled, no shared abstraction and no
// app/Platform -> app/Admin import). The two evolve independently from here, and
// the tenant picker below is the first place they have actually diverged.
//
// The form emits PlatformUserCreateData (the superset). Only the create view
// sends tenant_id on the wire: an edit must never move a user between tenants, so
// PUT has no tenant_id in its contract at all and the edit view drops the field
// explicitly rather than relying on the backend to ignore it.
const {
    initial = {},
    mode,
    submitLabel,
    isLoading,
    errors,
    tenants = [],
} = defineProps<{
    initial?: Partial<PlatformUserCreateData>;
    mode: 'create' | 'edit';
    submitLabel: string;
    isLoading: boolean;
    errors: PlatformUserFormErrors;
    // Selectable tenants — create mode only. The view fetches them; the form
    // stays dumb.
    tenants?: { value: string; label: string }[];
}>();

const emit = defineEmits<{
    submit: [data: PlatformUserCreateData];
    cancel: [];
    clearError: [field: keyof PlatformUserFormErrors];
}>();

const form: PlatformUserCreateData = reactive({
    nickname: initial.nickname ?? '',
    password: '',
    email: initial.email ?? '',
    role: initial.role ?? Role.User,
    tenant_id: initial.tenant_id ?? '',
});

const roleOptions = ref([
    { value: Role.User, label: 'User' },
    { value: Role.Admin, label: 'Admin' },
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

        <!-- Create only: a superadmin picks the owning tenant. An edit cannot
             move a user between tenants, so the field has nothing to offer there. -->
        <Select
            v-if="mode === 'create'"
            v-model="form.tenant_id"
            name="tenant_id"
            label="Tenant"
            :options="tenants"
            :error="errors.tenant_id"
            required
            :disabled="isLoading"
            @update:model-value="() => emit('clearError', 'tenant_id')"
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
