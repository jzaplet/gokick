import { describe, expect, it, beforeEach, vi } from 'vitest';

const KEY = 'persistent-toasts';

// loadToasts() runs at MODULE-EVALUATION time, so the persisted value has to be
// in place before the import — hence resetModules + a dynamic import per case
// rather than a plain top-level one.
const loadWith = async (raw: string): Promise<{ toasts: { message: string }[] }> => {
    localStorage.setItem(KEY, raw);
    vi.resetModules();

    return import('@/app-ui/Toast/useToast');
};

const validToast = { id: 1, type: 'success', message: 'kept', duration: null };

describe('persisted toast state is validated, not asserted', () => {
    beforeEach((): void => {
        localStorage.clear();
    });

    it('restores a well-formed payload', async (): Promise<void> => {
        const mod = await loadWith(JSON.stringify({ toasts: [validToast], sleepingToasts: [] }));

        expect(mod.toasts.map((t) => t.message)).toEqual(['kept']);
    });

    // The cast this replaced waved these through: JSON.parse succeeds, so the
    // catch never fired, and the bad shape surfaced later as `toast.type` being
    // undefined deep in the render — far from its cause. localStorage outlives
    // a deploy, so a payload from an older schema is a real input, not a
    // hypothetical.
    it.each([
        ['a stale schema (renamed key)', JSON.stringify({ toasts: [validToast], sleeping: [] })],
        ['a wrong field type', JSON.stringify({ toasts: [{ ...validToast, id: '1' }], sleepingToasts: [] })],
        ['an unknown toast type', JSON.stringify({ toasts: [{ ...validToast, type: 'boom' }], sleepingToasts: [] })],
        ['a missing field', JSON.stringify({ toasts: [{ id: 1, type: 'info' }], sleepingToasts: [] })],
        ['an array where an object belongs', JSON.stringify([validToast])],
        ['a bare primitive', '42'],
        ['malformed JSON', '{not json'],
    ])('drops %s and starts empty', async (_label, raw): Promise<void> => {
        const mod = await loadWith(raw);

        expect(mod.toasts).toEqual([]);
        // The poisoned key must be cleared too — otherwise it re-fails on every
        // future load until someone clears it by hand.
        expect(localStorage.getItem(KEY)).toBeNull();
    });

    it('never throws at import time — a dead app is worse than a lost toast', async (): Promise<void> => {
        await expect(loadWith('{"toasts": "not-an-array"}')).resolves.toBeDefined();
    });
});
