import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { createDebounce } from '@/app-ui/Debounce/createDebounce';

describe('createDebounce', () => {
    beforeEach((): void => {
        vi.useFakeTimers();
    });

    afterEach((): void => {
        vi.useRealTimers();
    });

    it('runs once after the delay, restarting on every call', () => {
        const debounce = createDebounce(400);
        const fn = vi.fn();

        debounce.run(fn);
        vi.advanceTimersByTime(300);
        debounce.run(fn);
        vi.advanceTimersByTime(300);

        expect(fn).not.toHaveBeenCalled();
        expect(debounce.isPending()).toBe(true);

        vi.advanceTimersByTime(100);

        expect(fn).toHaveBeenCalledTimes(1);
        expect(debounce.isPending()).toBe(false);
    });

    it('cancel drops the pending run', () => {
        const debounce = createDebounce(400);
        const fn = vi.fn();

        debounce.run(fn);
        debounce.cancel();
        vi.advanceTimersByTime(1000);

        expect(fn).not.toHaveBeenCalled();
    });
});
