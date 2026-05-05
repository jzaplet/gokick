<script setup lang="ts">
import type { AuthUser } from '@/app-ui/Auth';

defineProps<{
    user: AuthUser;
}>();
</script>

<template>
    <div class="bg-white rounded-lg shadow-md p-6 space-y-3">
        <h2 class="text-lg font-semibold text-gray-900">
            Account information
        </h2>
        <dl class="text-sm text-gray-700 space-y-1">
            <div class="flex gap-2">
                <dt class="font-medium">
                    Nickname:
                </dt>
                <dd>{{ user.nickname }}</dd>
            </div>
            <div class="flex gap-2">
                <dt class="font-medium">
                    Role:
                </dt>
                <dd>{{ user.role }}</dd>
            </div>
        </dl>
    </div>
</template>
