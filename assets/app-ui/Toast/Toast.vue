<script setup lang="ts">
import type { ToastType } from './types/ToastType';
import SuccessIcon from '@/app-ui/Icons/SuccessIcon.vue';
import ErrorIcon from '@/app-ui/Icons/ErrorIcon.vue';
import WarningIcon from '@/app-ui/Icons/WarningIcon.vue';
import InfoIcon from '@/app-ui/Icons/InfoIcon.vue';
import CloseIcon from '@/app-ui/Icons/CloseIcon.vue';
import type { Component } from 'vue';

defineProps<{
    type: ToastType;
}>();

const emit = defineEmits<{
    close: [];
}>();

const colors = {
    success: 'text-green-600 bg-green-50',
    error: 'text-red-600 bg-red-50',
    warning: 'text-yellow-600 bg-yellow-50',
    info: 'text-gray-600 bg-blue-50',
};

const iconColors = {
    success: 'text-green-500 bg-green-100',
    error: 'text-red-500 bg-red-100',
    warning: 'text-yellow-500 bg-yellow-100',
    info: 'text-blue-500 bg-blue-100',
};

const icons: Record<ToastType, Component> = {
    success: SuccessIcon,
    error: ErrorIcon,
    warning: WarningIcon,
    info: InfoIcon,
};

</script>

<template>
    <div
        class="flex items-center p-4 rounded-lg shadow-sm"
        :class="colors[type]"
        role="alert"
    >
        <div
            class="inline-flex items-center justify-center flex-shrink-0 w-8 h-8 rounded-lg"
            :class="iconColors[type]"
        >
            <component :is="icons[type]" />
        </div>
        <div class="mx-3 text-sm font-normal">
            <slot />
        </div>
        <button
            type="button"
            class="inline-flex items-center justify-center
        ml-auto p-1.5 -my-1.5 h-8 w-8 rounded-lg
        hover:bg-gray-100 hover:text-gray-900
        focus:ring-2 focus:ring-gray-300
        transition-colors duration-200 cursor-pointer"
            :class="colors[type]"
            aria-label="Close"
            @click="emit('close')"
        >
            <CloseIcon />
        </button>
    </div>
</template>
