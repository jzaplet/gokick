<script setup lang="ts">
import { ref } from 'vue';
import { useClickOutside } from '@/app-ui/ClickOutside/useClickOutside';

const containerRef = ref<HTMLElement | null>(null);
const isOpen = ref(false);

const toggle = (): void => {
    isOpen.value = isOpen.value === false;
};

const close = (): void => {
    isOpen.value = false;
};

useClickOutside(containerRef, close);
</script>

<template>
    <div
        ref="containerRef"
        class="relative"
    >
        <div @click="toggle">
            <slot name="trigger" />
        </div>

        <Transition
            enter-active-class="transition ease-out duration-100"
            enter-from-class="transform opacity-0 scale-95"
            enter-to-class="transform opacity-100 scale-100"
            leave-active-class="transition ease-in duration-75"
            leave-from-class="transform opacity-100 scale-100"
            leave-to-class="transform opacity-0 scale-95"
        >
            <div
                v-if="isOpen === true"
                :class="[
                    'absolute right-0 mt-2 w-48 z-50',
                    'bg-white rounded-lg shadow-lg border border-gray-200 py-1',
                ]"
                @click="close"
            >
                <slot />
            </div>
        </Transition>
    </div>
</template>
