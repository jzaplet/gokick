import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { scheduleRefresh, clearAuth } from '@/app-ui/Auth/state';

// guide-forms-fe-35 / guide-auth-perm-41 (FE timing half):
// Auto-refresh of the access token fires 30 seconds BEFORE the access token
// expires. scheduleRefresh(expiresInMs, fn) calls fn() at
// max(expiresInMs - 30_000, 1_000) ms. Token rotation itself is backend
// behaviour (covered in Go) — on the FE we only assert the timing + that a
// single pending timer is kept.
describe('scheduleRefresh', () => {
    beforeEach((): void => {
        vi.useFakeTimers();
    });

    afterEach((): void => {
        // clearAuth() clears the pending refresh timer; restore real timers.
        clearAuth();
        vi.useRealTimers();
        vi.restoreAllMocks();
    });

    it('fires the refresh fn exactly 30s before a 900s token expires', (): void => {
        const fn = vi.fn();

        // 900_000 ms expiry → fires at 900_000 - 30_000 = 870_000 ms.
        scheduleRefresh(900_000, fn);

        vi.advanceTimersByTime(869_999);
        expect(fn).not.toHaveBeenCalled();

        vi.advanceTimersByTime(1);
        expect(fn).toHaveBeenCalledTimes(1);
    });

    it('floors the delay at 1s when expiry is already within the 30s window', (): void => {
        const fn = vi.fn();

        // expiresInMs - 30_000 would be negative; Math.max clamps to 1_000.
        scheduleRefresh(5_000, fn);

        vi.advanceTimersByTime(999);
        expect(fn).not.toHaveBeenCalled();

        vi.advanceTimersByTime(1);
        expect(fn).toHaveBeenCalledTimes(1);
    });

    it('keeps only a single pending timer (a new schedule cancels the previous)', (): void => {
        const stale = vi.fn();
        const fresh = vi.fn();

        scheduleRefresh(900_000, stale);
        // Re-scheduling must clear the first timer so the stale fn never runs.
        scheduleRefresh(900_000, fresh);

        vi.advanceTimersByTime(870_000);

        expect(stale).not.toHaveBeenCalled();
        expect(fresh).toHaveBeenCalledTimes(1);
    });

    it('clearAuth cancels the pending refresh timer', (): void => {
        const fn = vi.fn();

        scheduleRefresh(900_000, fn);
        clearAuth();

        vi.advanceTimersByTime(900_000);

        expect(fn).not.toHaveBeenCalled();
    });
});
