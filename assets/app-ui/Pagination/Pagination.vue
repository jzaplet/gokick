<script setup lang="ts">
import { computed } from 'vue';
import ChevronLeftIcon from '@/app-ui/Icons/ChevronLeftIcon.vue';
import { useI18n } from '@/app-ui/I18n';

// "X–Y / N" + a window of up to five page buttons with prev/next chevrons.
// Pure presentation over (page, perPage, total) — the grid state owns the
// actual paging. Sits INSIDE the grid card, attached with a top border
// (aibobr parity); renders nothing for an empty result.
const { page, perPage, total } = defineProps<{
    page: number;
    perPage: number;
    total: number;
}>();

const emit = defineEmits<{
    'update:page': [page: number];
}>();

const { t } = useI18n();

const totalPages = computed((): number => Math.max(1, Math.ceil(total / perPage)));

const rangeFrom = computed((): number => total === 0 ? 0 : (page - 1) * perPage + 1);

const rangeTo = computed((): number => Math.min(page * perPage, total));

// A sliding window of up to 5 page numbers centered on the current page.
const pageWindow = computed((): number[] => {
    const windowSize = 5;
    let start = Math.max(1, page - 2);
    const end = Math.min(totalPages.value, start + windowSize - 1);

    start = Math.max(1, end - windowSize + 1);

    const pages: number[] = [];

    for (let p = start; p <= end; p += 1) {
        pages.push(p);
    }

    return pages;
});

const goTo = (target: number): void => {
    if (target >= 1 && target <= totalPages.value && target !== page) {
        emit('update:page', target);
    }
};
</script>

<template>
    <div
        v-if="total > 0"
        class="flex items-center justify-between border-t border-gray-200 px-4 py-3"
    >
        <p class="text-sm text-gray-500">
            {{ rangeFrom }}–{{ rangeTo }} / {{ total }}
        </p>

        <nav
            v-if="totalPages > 1"
            class="flex gap-1"
            :aria-label="t('pagination.label')"
        >
            <button
                type="button"
                :disabled="page <= 1"
                :aria-label="t('pagination.previous')"
                :class="[
                    'p-1.5 rounded border border-gray-300',
                    'hover:bg-gray-50 cursor-pointer',
                    'disabled:opacity-40 disabled:cursor-not-allowed',
                ]"
                @click="goTo(page - 1)"
            >
                <ChevronLeftIcon class="w-4 h-4" />
            </button>
            <button
                v-for="p in pageWindow"
                :key="p"
                type="button"
                :class="[
                    'px-3 py-1.5 text-sm rounded border cursor-pointer',
                    p === page
                        ? 'bg-orange-500 text-white border-orange-500'
                        : 'border-gray-300 hover:bg-gray-50',
                ]"
                @click="goTo(p)"
            >
                {{ p }}
            </button>
            <button
                type="button"
                :disabled="page >= totalPages"
                :aria-label="t('pagination.next')"
                :class="[
                    'p-1.5 rounded border border-gray-300',
                    'hover:bg-gray-50 cursor-pointer',
                    'disabled:opacity-40 disabled:cursor-not-allowed',
                ]"
                @click="goTo(page + 1)"
            >
                <ChevronLeftIcon class="w-4 h-4 rotate-180" />
            </button>
        </nav>
    </div>
</template>
