// Generic debounce factory (ported from aibobr): run() postpones fn by the
// delay and restarts the timer on every call; cancel() drops a pending run.
// A factory (not a composable) on purpose — it holds no Vue reactivity and is
// usable anywhere, e.g. inside createGridState's filter watcher.
export type Debounce = {
    run: (fn: () => void) => void;
    cancel: () => void;
    isPending: () => boolean;
};

export const createDebounce = (delayMs = 400): Debounce => {
    let timer: ReturnType<typeof setTimeout> | null = null;

    const cancel = (): void => {
        if (timer !== null) {
            clearTimeout(timer);
            timer = null;
        }
    };

    const run = (fn: () => void): void => {
        cancel();
        timer = setTimeout((): void => {
            timer = null;
            fn();
        }, delayMs);
    };

    return {
        run,
        cancel,
        isPending: (): boolean => timer !== null,
    };
};
