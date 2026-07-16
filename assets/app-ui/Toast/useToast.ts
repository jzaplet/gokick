import { reactive, ref } from 'vue';
import { arrayOf, isNumber, isRecord, isString, nullable } from '@/app-ui/Fetch/guards';
import type { ToastType } from './types/ToastType';
import type { Toast } from './types/Toast';

const TOAST_STORAGE_KEY = 'persistent-toasts';

type StoredToasts = {
    toasts: Toast[];
    sleepingToasts: Toast[];
};

// localStorage is an untrusted boundary for the same reason the wire is: the
// value outlives the code that wrote it (it survives deploys), so a payload
// from an older schema — or a hand-edited one — is a real input, not a
// hypothetical. Hence the same discipline and the same primitives as the wire
// guards, rather than asserting the shape and hoping.
const isToastType = (v: unknown): v is ToastType =>
    v === 'success' || v === 'error' || v === 'info' || v === 'warning';

const isToast = (v: unknown): v is Toast =>
    isRecord(v)
    && isNumber(v['id'])
    && isToastType(v['type'])
    && isString(v['message'])
    && nullable(isNumber)(v['duration']);

const isStoredToasts = (v: unknown): v is StoredToasts =>
    isRecord(v)
    && arrayOf(isToast)(v['toasts'])
    && arrayOf(isToast)(v['sleepingToasts']);

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
    // white screen at bootstrap. Recover instead: drop the bad key and start
    // with empty state — a lost toast history is never worth a dead app. The
    // catch covers malformed JSON; isStoredToasts covers well-formed JSON of
    // the wrong shape, which an assertion used to wave through — a stale
    // schema then surfaced as `toast.type` being undefined deep in the render,
    // far from its cause. (F-076; explicit schema VERSIONING is still open.)
    try {
        const parsed: unknown = JSON.parse(storedToasts);

        if (isStoredToasts(parsed) === false) {
            storage.removeItem(TOAST_STORAGE_KEY);

            return;
        }

        for (const toast of parsed.toasts) {
            addToast(toast.type, toast.message, toast.duration);
        }

        for (const toast of parsed.sleepingToasts) {
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
