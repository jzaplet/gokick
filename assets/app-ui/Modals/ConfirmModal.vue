<script setup lang="ts">
import Button from '@/app-ui/Buttons/Button.vue';
import WarningIcon from '@/app-ui/Icons/WarningIcon.vue';

const {
    show,
    title,
    message,
    confirmText = 'Confirm',
    cancelText = 'Cancel',
} = defineProps<{
    show: boolean;
    title: string;
    message: string;
    confirmText?: string;
    cancelText?: string;
}>();

const emit = defineEmits<{
    confirm: [];
    cancel: [];
}>();

// Visibility is fully controlled by the `show` prop — the parent flips it. No
// local isVisible mirror (it only risked drifting from the prop).
const handleConfirm = (): void => {
    emit('confirm');
};

const handleCancel = (): void => {
    emit('cancel');
};
</script>

<template>
    <Teleport to="body">
        <Transition name="modal">
            <div
                v-if="show"
                class="fixed inset-0 z-50 overflow-y-auto"
                aria-labelledby="modal-title"
                role="dialog"
                aria-modal="true"
            >
                <div
                    class="flex items-center justify-center min-h-screen pt-4 px-4 pb-20 text-center sm:p-0"
                >
                    <!-- Background overlay -->
                    <Transition name="fade">
                        <div
                            v-if="show"
                            class="fixed inset-0 bg-gray-900/50 transition-opacity"
                            @click="handleCancel"
                        />
                    </Transition>

                    <!-- Modal panel -->
                    <Transition name="scale">
                        <div
                            v-if="show"
                            class="relative inline-block align-bottom
                bg-white rounded-lg shadow-xl text-left
                overflow-hidden transform transition-all
                sm:my-8 sm:align-middle sm:max-w-lg
                sm:w-full z-10"
                        >
                            <div
                                class="px-4 pt-5 pb-4 bg-white sm:p-6 sm:pb-4"
                            >
                                <div class="sm:flex sm:items-start">
                                    <div
                                        class="flex flex-shrink-0 items-center
                      justify-center mx-auto sm:mx-0
                      h-12 w-12 sm:h-10 sm:w-10
                      rounded-full bg-red-100"
                                    >
                                        <WarningIcon class="h-6 w-6 text-red-600" />
                                    </div>
                                    <div
                                        class="mt-3 sm:mt-0 sm:ml-4 text-center sm:text-left"
                                    >
                                        <h3
                                            id="modal-title"
                                            class="text-lg leading-6 font-medium text-gray-900"
                                        >
                                            {{ title }}
                                        </h3>
                                        <div class="mt-2">
                                            <p class="text-sm text-gray-500">
                                                {{ message }}
                                            </p>
                                        </div>
                                    </div>
                                </div>
                            </div>
                            <div
                                :class="[
                                    'px-4 py-3 sm:px-6 bg-gray-50',
                                    'flex flex-col-reverse sm:flex-row sm:justify-end gap-2',
                                ]"
                            >
                                <Button
                                    variant="secondary"
                                    @click="handleCancel"
                                >
                                    {{ cancelText }}
                                </Button>
                                <Button
                                    variant="danger"
                                    @click="handleConfirm"
                                >
                                    {{ confirmText }}
                                </Button>
                            </div>
                        </div>
                    </Transition>
                </div>
            </div>
        </Transition>
    </Teleport>
</template>

<style scoped>
.modal-enter-active,
.modal-leave-active {
    transition: opacity 200ms ease;
}

.modal-enter-from,
.modal-leave-to {
    opacity: 0;
}

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
