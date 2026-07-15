import { reactive, ref } from 'vue';
import type { ToastType } from './types/ToastType';
import type { Toast } from './types/Toast';

const TOAST_STORAGE_KEY = 'persistent-toasts';

const storage
    = typeof localStorage !== 'undefined'
        ? localStorage
        : {
                getItem: (): null => null,
                setItem: (): void => { /* noop */ },
                removeItem: (): void => { /* noop */ },
            };

let idCounter = 0;

const isAsleep = ref(false);

const sleepingToasts = reactive<Toast[]>([]);

export const toasts = reactive<Toast[]>([]);

export const asleep = (): void => {
    isAsleep.value = true;
};

export const awake = (): void => {
    isAsleep.value = false;

    for (const toast of sleepingToasts.splice(0, sleepingToasts.length)) {
        addToast(toast.type, toast.message, toast.duration);
    }
};

const persistToasts = (): void => {
    storage.setItem(
        TOAST_STORAGE_KEY,
        JSON.stringify({
            toasts: [...toasts],
            sleepingToasts: [...sleepingToasts],
        }),
    );
};

export const removeToast = (toast: Toast): void => {
    const index = toasts.indexOf(toast);

    if (index !== -1) {
        toasts.splice(index, 1);
        persistToasts();
    }
};

export const addToast = (
    type: ToastType,
    message: string,
    duration: number | null = 5000,
): void => {
    const toast: Toast = {
        id: idCounter++,
        type,
        message,
        duration,
    };

    if (isAsleep.value) {
        sleepingToasts.push(toast);
        persistToasts();

        return;
    }

    toasts.push(toast);
    persistToasts();

    if (toast.duration !== null) {
        setTimeout(() => {
            removeToast(toast);
        }, toast.duration);
    }
};

export const success = (
    message: string,
    duration: number | null = 3000,
): void => {
    addToast('success', message, duration);
};

export const error = (
    message: string,
    duration: number | null = 3000,
): void => {
    addToast('error', message, duration);
};

export const info = (message: string, duration: number | null = 3000): void => {
    addToast('info', message, duration);
};

export const warning = (
    message: string,
    duration: number | null = 3000,
): void => {
    addToast('warning', message, duration);
};

export const clearToasts = (): void => {
    toasts.splice(0, toasts.length);
    sleepingToasts.splice(0, sleepingToasts.length);
    storage.removeItem(TOAST_STORAGE_KEY);
};

const loadToasts = (): void => {
    const storedToasts = storage.getItem(TOAST_STORAGE_KEY);

    if (storedToasts === null) {
        return;
    }

    // loadToasts() runs at module-evaluation time (App.vue → ToastContainer →
    // useToast import), so an unguarded throw here bricks the whole app with a
    // white screen at bootstrap. localStorage survives across deploys, so one
    // corrupt or schema-changed 'persistent-toasts' value would poison every
    // future load until cleared by hand. Recover instead: drop the bad key and
    // start with empty state — a lost toast history is never worth a dead app.
    // Long-term hardening (versioning/validating the persisted shape) is tracked
    // separately with the BE↔FE parity follow-up. (F-076)
    try {
        const parsed: {
            toasts: Toast[];
            sleepingToasts: Toast[];
        } = JSON.parse(storedToasts) as {
            toasts: Toast[];
            sleepingToasts: Toast[];
        };

        const { toasts: active, sleepingToasts: sleeping } = parsed;

        for (const toast of active) {
            addToast(toast.type, toast.message, toast.duration);
        }

        for (const toast of sleeping) {
            sleepingToasts.push(toast);
        }

        awake();
    } catch {
        storage.removeItem(TOAST_STORAGE_KEY);
    }
};

loadToasts();

export const useToast = (): {
    toasts: typeof toasts;
    success: typeof success;
    error: typeof error;
    info: typeof info;
    warning: typeof warning;
    clear: typeof clearToasts;
    remove: typeof removeToast;
    asleep: typeof asleep;
    awake: typeof awake;
} => {
    return {
        toasts,
        success,
        error,
        info,
        warning,
        clear: clearToasts,
        remove: removeToast,
        asleep,
        awake,
    };
};
