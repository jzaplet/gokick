<script setup lang="ts">
import { onMounted } from 'vue'
import { addToast } from '@/app-ui/Toast/Toast'

onMounted(() => {
    console.log('HomeView mounted')
    addToast('info', 'Welcome to the Home Page!')
})
</script>

<template>
    <div class="flex items-center justify-center min-h-screen">
        <h1 class="text-4xl font-bold text-gray-900">
            Welcome to the Home Page!
        </h1>
    </div>
</template>
