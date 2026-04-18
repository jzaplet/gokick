<script setup lang="ts">
import { useAuth } from '@/app-ui/Auth';

const { user } = useAuth();
</script>

<template>
    <div class="flex flex-col items-center justify-center min-h-screen p-10 gap-4">
        <h1 class="text-3xl font-bold text-gray-900">
            Profil
        </h1>
        <div
            v-if="user !== null"
            class="text-gray-700"
        >
            <p>Nickname: {{ user.nickname }}</p>
            <p>Role: {{ user.role }}</p>
        </div>
    </div>
</template>
