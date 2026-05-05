import type { Ref } from 'vue';
import { onMounted, onUnmounted } from 'vue';

// Calls `onClickOutside` whenever a click lands outside the referenced element.
// Listener is attached on mount and detached on unmount.
export const useClickOutside = (
    elementRef: Ref<HTMLElement | null>,
    onClickOutside: () => void,
): void => {
    const handleClick = (event: MouseEvent): void => {
        if (
            elementRef.value !== null
            && elementRef.value.contains(event.target as Node) === false
        ) {
            onClickOutside();
        }
    };

    onMounted(() => {
        document.addEventListener('click', handleClick);
    });

    onUnmounted(() => {
        document.removeEventListener('click', handleClick);
    });
};
