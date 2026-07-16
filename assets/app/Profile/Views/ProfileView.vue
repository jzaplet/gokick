<script setup lang="ts">
import { useAuth } from '@/app-ui/Auth';
import UserInfoCard from '@/app/Profile/Components/UserInfoCard.vue';
import ChangePasswordForm from '@/app/Profile/Components/ChangePasswordForm.vue';

const { user } = useAuth();
</script>

<template>
    <div>
        <div class="max-w-2xl mx-auto space-y-8">
            <h1 class="text-2xl font-bold text-gray-900">
                Profile
            </h1>

            <UserInfoCard
                v-if="user !== null"
                :user="user"
            />
            <ChangePasswordForm />
        </div>
    </div>
</template>
