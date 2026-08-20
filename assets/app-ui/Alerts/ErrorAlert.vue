<script setup lang="ts">
import ErrorIcon from '@/app-ui/Icons/ErrorIcon.vue';

// null and undefined both mean "nothing to show" — null so tm(errors.general)
// binds directly (it returns string | null).
defineProps<{
    message?: string | null;
}>();
</script>

<template>
    <div
        v-if="message !== undefined && message !== null && message !== ''"
        class="bg-red-50 border border-red-200 rounded-lg p-4"
    >
        <div class="flex">
            <div class="flex-shrink-0 text-red-800">
                <ErrorIcon class="w-5 h-5" />
            </div>
            <div class="ml-3">
                <p class="text-sm font-medium text-red-800">
                    {{ message }}
                </p>
            </div>
        </div>
    </div>
</template>
