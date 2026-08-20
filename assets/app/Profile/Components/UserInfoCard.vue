<script setup lang="ts">
import type { DeepReadonly } from 'vue';
import type { AuthUser } from '@/app-ui/Auth';
import { roleLabel } from '@/app/Auth/enums/roleLabels';
import { useI18n } from '@/app-ui/I18n';

defineProps<{
    user: DeepReadonly<AuthUser>;
}>();

const { t } = useI18n();
</script>

<template>
    <div class="bg-white rounded-lg shadow-md p-6 space-y-3">
        <h2 class="text-lg font-semibold text-gray-900">
            {{ t('profile.account_info') }}
        </h2>
        <dl class="text-sm text-gray-700 space-y-1">
            <div class="flex gap-2">
                <dt class="font-medium">
                    {{ t('auth.nickname') }}:
                </dt>
                <dd>{{ user.nickname }}</dd>
            </div>
            <div class="flex gap-2">
                <dt class="font-medium">
                    {{ t('common.role') }}:
                </dt>
                <dd>{{ roleLabel(user.role) }}</dd>
            </div>
        </dl>
    </div>
</template>
