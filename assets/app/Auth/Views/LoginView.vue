<script setup lang="ts">
import LoginForm from '@/app/Auth/Components/LoginForm.vue';
</script>

<template>
    <div class="min-h-screen bg-gray-50 flex items-center justify-center px-4 sm:px-6 lg:px-8">
        <div class="max-w-md w-full space-y-8">
            <div class="text-center">
                <h2 class="text-3xl font-extrabold text-gray-900">
                    Sign in
                </h2>
                <p class="mt-2 text-sm text-gray-600">
                    Enter your credentials
                </p>
            </div>

            <LoginForm />

            <div class="text-center">
                <RouterLink
                    :to="{ name: 'home' }"
                    class="text-sm text-gray-500 hover:text-gray-700"
                >
                    ← Back to home
                </RouterLink>
            </div>
        </div>
    </div>
</template>
