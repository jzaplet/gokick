<script setup lang="ts">
import ToastContainer from '@/app-ui/Toast/ToastContainer.vue';
</script>

<template>
    <div class="min-h-screen bg-gray-50">
        <RouterView />
        <ToastContainer />
    </div>
</template>
