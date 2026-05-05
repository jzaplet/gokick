<script setup lang="ts">
import { useRoute } from 'vue-router';
import AppLayout from '@/app/Layout/AppLayout.vue';
import ToastContainer from '@/app-ui/Toast/ToastContainer.vue';

const route = useRoute();
</script>

<template>
    <AppLayout v-if="route.meta.requiresAuth === true">
        <RouterView />
    </AppLayout>
    <RouterView v-else />
    <ToastContainer />
</template>
