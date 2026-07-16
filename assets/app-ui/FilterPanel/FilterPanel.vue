<script setup lang="ts">
import { onMounted, ref } from 'vue';
import Button from '@/app-ui/Buttons/Button.vue';

// Collapsible filter panel above a grid (ported from aibobr): remembers its
// open/closed state per grid in localStorage, opens itself when filters are
// active on mount (a deep link with filters must show WHY the list is
// narrowed), and offers one clear-all button. The inputs are the consumer's
// slot — the panel owns only the chrome.
const { storageKey, label = 'Filters', hasActiveFilters } = defineProps<{
    storageKey: string;
    label?: string;
    hasActiveFilters: boolean;
}>();

const emit = defineEmits<{
    clear: [];
}>();

const isOpen = ref(false);

const storage = (): string => `filter_panel_${storageKey}`;

onMounted((): void => {
    isOpen.value = localStorage.getItem(storage()) === '1' || hasActiveFilters === true;
});

const toggle = (): void => {
    isOpen.value = isOpen.value === false;
    localStorage.setItem(storage(), isOpen.value === true ? '1' : '0');
};
</script>

<template>
    <div class="bg-white rounded-lg shadow-md">
        <div class="flex items-center justify-between px-4 sm:px-6 py-3">
            <button
                type="button"
                :class="[
                    'inline-flex items-center gap-2 cursor-pointer',
                    'text-sm font-medium text-gray-700 hover:text-gray-900',
                ]"
                @click="toggle"
            >
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    aria-hidden="true"
                    :class="[
                        'w-4 h-4 transition-transform',
                        isOpen === true ? 'rotate-180' : '',
                    ]"
                >
                    <path
                        stroke="currentColor"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="m6 9 6 6 6-6"
                    />
                </svg>
                {{ label }}
                <span
                    v-if="hasActiveFilters === true"
                    class="inline-flex w-2 h-2 rounded-full bg-orange-500"
                    aria-label="Filters active"
                />
            </button>

            <Button
                v-if="hasActiveFilters === true"
                variant="ghost"
                size="sm"
                @click="emit('clear')"
            >
                Clear filters
            </Button>
        </div>

        <div
            v-show="isOpen === true"
            class="px-4 sm:px-6 pb-4 border-t border-gray-100 pt-4"
        >
            <slot />
        </div>
    </div>
</template>
