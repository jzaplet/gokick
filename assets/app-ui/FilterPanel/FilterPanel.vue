<script setup lang="ts">
import { onMounted, ref } from 'vue';

// Collapsible filter panel above a grid (ported from aibobr): a MINIMAL
// toggle — plain text with a chevron, turning into a filled orange pill when
// filters are active so the narrowed list is impossible to miss. Remembers
// its open/closed state per grid in localStorage and opens itself when
// filters are active on mount (a deep link with filters must show WHY the
// list is narrowed). The inputs are the consumer's slot; clear-all lives
// inside the panel under them.
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
    <div>
        <button
            type="button"
            :class="[
                'inline-flex items-center gap-2 cursor-pointer',
                'px-3 py-1.5 rounded-md',
                'text-sm font-medium transition-colors',
                hasActiveFilters === true
                    ? 'bg-orange-500 text-white hover:bg-orange-600'
                    : '-ml-3 text-gray-700 hover:text-gray-900',
            ]"
            @click="toggle"
        >
            {{ label }}
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
        </button>

        <div
            v-show="isOpen === true"
            class="mt-3 bg-white rounded-lg shadow-md px-4 sm:px-6 py-4"
        >
            <slot />
            <button
                type="button"
                :class="[
                    'mt-3 text-sm underline cursor-pointer',
                    hasActiveFilters === true
                        ? 'text-orange-700 hover:text-orange-900'
                        : 'text-gray-400 cursor-not-allowed',
                ]"
                :disabled="hasActiveFilters === false"
                @click="emit('clear')"
            >
                Clear filters
            </button>
        </div>
    </div>
</template>
