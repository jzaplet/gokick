<script setup lang="ts">
import { watch, onUnmounted } from 'vue';
import CloseIcon from '@/app-ui/Icons/CloseIcon.vue';

const props = defineProps<{
    show: boolean;
    title: string;
}>();

const emit = defineEmits<{
    close: [];
}>();

watch(
    () => props.show,
    (visible) => {
        document.body.style.overflow = visible ? 'hidden' : '';
    },
);

onUnmounted(() => {
    document.body.style.overflow = '';
});
</script>

<template>
    <Teleport to="body">
        <Transition
            name="fade"
            appear
        >
            <div
                v-if="show"
                class="fixed inset-0 z-50 bg-gray-900/50"
                @click="emit('close')"
            />
        </Transition>

        <Transition
            name="scale"
            appear
        >
            <div
                v-if="show"
                class="fixed inset-0 z-50 overflow-y-auto pointer-events-none"
                role="dialog"
                aria-modal="true"
            >
                <div
                    class="flex items-center justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:p-0"
                >
                    <div
                        class="pointer-events-auto relative inline-block
              align-bottom bg-white rounded-lg shadow-xl
              text-left overflow-hidden sm:my-8
              sm:align-middle sm:max-w-lg sm:w-full"
                    >
                        <div class="px-4 pt-5 pb-4 sm:p-6">
                            <div
                                class="flex items-center justify-between mb-4"
                            >
                                <h3 class="text-lg font-medium text-gray-900">
                                    {{ title }}
                                </h3>
                                <button
                                    type="button"
                                    class="text-gray-400 hover:text-gray-600 transition-colors cursor-pointer"
                                    @click="emit('close')"
                                >
                                    <CloseIcon class="w-5 h-5" />
                                </button>
                            </div>
                            <slot />
                        </div>
                    </div>
                </div>
            </div>
        </Transition>
    </Teleport>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
    transition: opacity 200ms ease;
}

.fade-enter-from,
.fade-leave-to {
    opacity: 0;
}

.scale-enter-active,
.scale-leave-active {
    transition: all 200ms ease;
}

.scale-enter-from,
.scale-leave-to {
    opacity: 0;
    transform: scale(0.95);
}
</style>
