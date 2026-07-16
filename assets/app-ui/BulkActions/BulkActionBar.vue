<script setup lang="ts">
import { ref } from 'vue';
import ChevronDownIcon from '@/app-ui/Icons/ChevronDownIcon.vue';
import { useClickOutside } from '@/app-ui/ClickOutside/useClickOutside';

export type BulkAction = {
    key: string;
    label: string;
};

// The action bar over a grid selection (aibobr parity). Appears once anything
// is selected; the selected count is an orange pill opening a dropdown of
// actions ("Delete (3x)"). The escalation link switches to "all N filtered" —
// the grid then sends the FILTER SET to the backend instead of enumerating
// ids.
const { count, total, isAllFiltered, actions } = defineProps<{
    count: number;
    total: number;
    isAllFiltered: boolean;
    actions: BulkAction[];
}>();

const emit = defineEmits<{
    action: [key: string];
    selectAllFiltered: [];
    clear: [];
}>();

const dropdownRef = ref<HTMLElement | null>(null);
const isOpen = ref(false);

const toggle = (): void => {
    isOpen.value = isOpen.value === false;
};

const close = (): void => {
    isOpen.value = false;
};

const handleAction = (key: string): void => {
    close();
    emit('action', key);
};

const clearSelection = (): void => {
    close();
    emit('clear');
};

useClickOutside(dropdownRef, close);
</script>

<template>
    <div
        v-if="count > 0"
        :class="[
            'flex items-center gap-3',
            'px-4 py-2',
            'bg-orange-50 border border-orange-200 rounded-lg',
        ]"
    >
        <div
            ref="dropdownRef"
            class="relative"
        >
            <button
                type="button"
                :class="[
                    'inline-flex items-center gap-1.5',
                    'px-3 py-1.5',
                    'text-sm font-medium text-white',
                    'bg-orange-500 rounded-lg',
                    'hover:bg-orange-600 transition-colors cursor-pointer',
                ]"
                @click="toggle"
            >
                {{ count }}
                <ChevronDownIcon class="w-3.5 h-3.5" />
            </button>

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
                        'absolute left-0 mt-2 w-56 py-1 z-50',
                        'bg-white rounded-lg shadow-lg border border-gray-200',
                    ]"
                >
                    <button
                        v-for="action in actions"
                        :key="action.key"
                        type="button"
                        :class="[
                            'w-full text-left px-4 py-2',
                            'text-sm text-gray-700',
                            'hover:bg-gray-50 cursor-pointer',
                        ]"
                        @click="handleAction(action.key)"
                    >
                        {{ action.label }} ({{ count }}x)
                    </button>

                    <div class="border-t border-gray-200 my-1" />

                    <button
                        type="button"
                        :class="[
                            'w-full text-left px-4 py-2',
                            'text-sm text-gray-500',
                            'hover:bg-gray-50 cursor-pointer',
                        ]"
                        @click="clearSelection"
                    >
                        Clear selection
                    </button>
                </div>
            </Transition>
        </div>

        <span class="text-sm text-gray-600">
            {{ isAllFiltered === true
                ? `Selected: all (${total})`
                : `Selected: ${count} / ${total}` }}
        </span>

        <button
            v-if="isAllFiltered === false"
            type="button"
            class="text-sm text-orange-600 underline hover:text-orange-800 cursor-pointer"
            @click="emit('selectAllFiltered')"
        >
            Select all ({{ total }})
        </button>

        <button
            type="button"
            class="text-sm text-gray-500 underline hover:text-gray-700 cursor-pointer"
            @click="emit('clear')"
        >
            Clear selection
        </button>
    </div>
</template>
