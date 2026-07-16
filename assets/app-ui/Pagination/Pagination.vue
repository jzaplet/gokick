<script setup lang="ts">
import { computed } from 'vue';

// "Showing X–Y of N" + a window of up to five page buttons with prev/next.
// Pure presentation over (page, perPage, total) — the grid state owns the
// actual paging (ported from aibobr).
const { page, perPage, total } = defineProps<{
    page: number;
    perPage: number;
    total: number;
}>();

const emit = defineEmits<{
    'update:page': [page: number];
}>();

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
    <div class="flex flex-col sm:flex-row items-center justify-between gap-3">
        <p class="text-sm text-gray-500">
            Showing {{ rangeFrom }}–{{ rangeTo }} of {{ total }}
        </p>

        <nav
            v-if="totalPages > 1"
            class="flex items-center gap-1"
            aria-label="Pagination"
        >
            <button
                type="button"
                :disabled="page <= 1"
                :class="[
                    'px-3 py-1.5 rounded-md text-sm font-medium',
                    'text-gray-700 hover:bg-gray-100',
                    'disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer',
                ]"
                @click="goTo(page - 1)"
            >
                Prev
            </button>
            <button
                v-for="p in pageWindow"
                :key="p"
                type="button"
                :class="[
                    'px-3 py-1.5 rounded-md text-sm font-medium cursor-pointer',
                    p === page
                        ? 'bg-orange-50 text-orange-700'
                        : 'text-gray-700 hover:bg-gray-100',
                ]"
                @click="goTo(p)"
            >
                {{ p }}
            </button>
            <button
                type="button"
                :disabled="page >= totalPages"
                :class="[
                    'px-3 py-1.5 rounded-md text-sm font-medium',
                    'text-gray-700 hover:bg-gray-100',
                    'disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer',
                ]"
                @click="goTo(page + 1)"
            >
                Next
            </button>
        </nav>
    </div>
</template>
