<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import ChevronDownIcon from '@/app-ui/Icons/ChevronDownIcon.vue';
import { useI18n } from '@/app-ui/I18n';

// Collapsible filter panel above a grid (aibobr parity): a MINIMAL toggle —
// plain text with a chevron, turning into a filled orange pill when filters
// are active so the narrowed list is impossible to miss. Remembers its
// open/closed state per grid in localStorage and opens itself when filters
// are active on mount (a deep link with filters must show WHY the list is
// narrowed). The inputs are the consumer's slot; the mini clear-all link
// lives inside the panel under them.
const { storageKey, label = null, hasActiveFilters } = defineProps<{
    storageKey: string;
    label?: string | null;
    hasActiveFilters: boolean;
}>();

const emit = defineEmits<{
    clear: [];
}>();

const { t } = useI18n();

// No prop default — the translated fallback resolves here so it stays
// reactive to locale changes.
const resolvedLabel = computed((): string => label ?? t('filters.title'));

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
            :aria-expanded="isOpen"
            :aria-label="hasActiveFilters === true ? t('filters.active_label', { label: resolvedLabel }) : undefined"
            :class="[
                'inline-flex items-center gap-1.5 cursor-pointer',
                'px-3 py-1.5 rounded-lg',
                'text-sm font-medium transition-colors',
                hasActiveFilters === true
                    ? 'bg-orange-500 text-white hover:bg-orange-600'
                    : 'text-gray-600 hover:text-gray-900 hover:bg-gray-100',
            ]"
            @click="toggle"
        >
            {{ resolvedLabel }}
            <ChevronDownIcon
                :class="[
                    'w-3.5 h-3.5 transition-transform duration-200',
                    isOpen === true ? 'rotate-180' : '',
                ]"
            />
        </button>

        <div
            v-show="isOpen === true"
            class="mt-3 bg-white border border-gray-200 rounded-xl p-4"
        >
            <slot />

            <div class="flex mt-3 ml-1">
                <button
                    type="button"
                    :disabled="hasActiveFilters === false"
                    :class="[
                        'text-xs underline underline-offset-2 transition-colors',
                        hasActiveFilters === true
                            ? 'text-red-400 hover:text-red-600 cursor-pointer'
                            : 'text-gray-300 cursor-default',
                    ]"
                    @click="emit('clear')"
                >
                    {{ t('filters.clear') }}
                </button>
            </div>
        </div>
    </div>
</template>
