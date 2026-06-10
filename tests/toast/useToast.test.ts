import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import {
    success,
    error,
    info,
    warning,
    addToast,
    clearToasts,
    toasts,
} from '@/app-ui/Toast/useToast';

// Ledger claim guide-forms-fe-38 (docs/guides/frontend-utils.md:107-116):
// "Toast functions accept a second argument that is the duration in
// milliseconds, and passing null disables auto-dismiss."
//
// The `toasts` reactive array and the "persistent-toasts" localStorage key
// are module-level singletons, so we reset both before every test to avoid
// cross-test bleed (the module also runs loadToasts() once at import time).
describe('useToast duration semantics (guide-forms-fe-38)', () => {
    beforeEach((): void => {
        vi.useFakeTimers();
        clearToasts();
        localStorage.clear();
    });

    afterEach((): void => {
        vi.useRealTimers();
    });

    it('auto-dismisses a toast after the given duration (ms) elapses', (): void => {
        success('hello', 1000);

        expect(toasts).toHaveLength(1);
        expect(toasts[0]?.message).toBe('hello');

        // Just before the deadline the toast is still present.
        vi.advanceTimersByTime(999);
        expect(toasts).toHaveLength(1);

        // Crossing the deadline removes it.
        vi.advanceTimersByTime(1);
        expect(toasts).toHaveLength(0);
    });

    it('keeps a toast forever when duration is null (no auto-dismiss)', (): void => {
        success('persistent', null);

        expect(toasts).toHaveLength(1);

        // Advance far past any plausible timeout; null must schedule nothing.
        vi.advanceTimersByTime(10 * 60 * 1000);

        expect(toasts).toHaveLength(1);
        expect(toasts[0]?.message).toBe('persistent');
        expect(toasts[0]?.duration).toBeNull();
    });

    it('honours distinct per-toast durations independently', (): void => {
        info('short', 500);
        warning('long', 2000);

        expect(toasts).toHaveLength(2);

        // After 500ms only the short one is gone.
        vi.advanceTimersByTime(500);
        expect(toasts.map((t) => t.message)).toEqual(['long']);

        // After a further 1500ms (2000ms total) the long one is gone too.
        vi.advanceTimersByTime(1500);
        expect(toasts).toHaveLength(0);
    });

    it('defaults the convenience helpers to a 3000ms duration', (): void => {
        success('default-success');
        error('default-error');

        expect(toasts.map((t) => t.duration)).toEqual([3000, 3000]);

        // Both survive up to but not through 3000ms.
        vi.advanceTimersByTime(2999);
        expect(toasts).toHaveLength(2);

        vi.advanceTimersByTime(1);
        expect(toasts).toHaveLength(0);
    });

    it('addToast carries the toast type and message onto the queued toast', (): void => {
        addToast('error', 'boom', null);

        expect(toasts).toHaveLength(1);
        expect(toasts[0]?.type).toBe('error');
        expect(toasts[0]?.message).toBe('boom');
    });

    it('does not schedule a duplicate removal: a null toast is unaffected by later timers', (): void => {
        // A null (sticky) toast plus a timed toast: only the timed one expires.
        warning('sticky', null);
        info('temp', 1000);

        expect(toasts).toHaveLength(2);

        vi.advanceTimersByTime(1000);

        expect(toasts.map((t) => t.message)).toEqual(['sticky']);
    });
});
