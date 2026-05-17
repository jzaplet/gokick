<script setup lang="ts">
import { useToast } from './useToast';

const { toasts, remove } = useToast();

import Toast from './Toast.vue';
</script>

<template>
    <TransitionGroup
        name="fade-toast"
        tag="div"
        class="fixed end-0 top-0 z-[60] flex flex-col gap-3 max-w-[350px] w-full p-5"
        :class="{
            'p-0': toasts.length === 0,
            'pointer-events-none': toasts.length === 0,
        }"
    >
        <Toast
            v-for="toast in toasts"
            :key="toast.id"
            :type="toast.type"
            @close="remove(toast)"
        >
            <div>{{ toast.message }}</div>
        </Toast>
    </TransitionGroup>
</template>

<style scoped>
.fade-toast-enter-active,
.fade-toast-leave-active {
    transition: all 0.4s ease;
    transform: translateY(-20px);
}
.fade-toast-enter-from,
.fade-toast-leave-to {
    opacity: 0;
}
.fade-toast-enter-to,
.fade-toast-leave-from {
    opacity: 1;
    transform: translateY(0);
}
</style>
