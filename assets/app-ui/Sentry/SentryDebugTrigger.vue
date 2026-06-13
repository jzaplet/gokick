<script setup lang="ts">
import { sentryDebugEnabled } from '@/app-ui/Sentry/runtimeConfig';

// Rendered only when the server reports APP_SENTRY_DEBUG=true (via the injected
// meta tag). Lets you verify frontend Sentry end-to-end on a live deploy. Never
// enabled in production.
const enabled = sentryDebugEnabled();

const crash = (): void => {
    // Throw outside the click handler's call stack so it is a genuinely
    // uncaught error → window.onerror → Sentry (bypasses any handler guard).
    setTimeout(() => {
        throw new Error('sentry debug: deliberate frontend test error (APP_SENTRY_DEBUG)');
    }, 0);
};
</script>

<template>
    <button
        v-if="enabled === true"
        type="button"
        :class="[
            'fixed bottom-4 right-4 z-50',
            'px-3 py-2',
            'text-sm font-medium text-white',
            'bg-red-600 rounded-lg shadow-lg',
            'hover:bg-red-700 transition-colors cursor-pointer',
        ]"
        @click="crash"
    >
        💥 Trigger FE Sentry error
    </button>
</template>
