import { describe, expect, it, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { defineComponent, h, ref } from 'vue';
import type { Component, Ref, VNode } from 'vue';
import { useClickOutside } from '@/app-ui/ClickOutside/useClickOutside';

type HostOptions = {
    // When false the ref is never bound to a rendered node — exercises the
    // null-guard branch in the composable.
    bindRef?: boolean;
    // Lets a test pre-bind the ref to its own (possibly detached) node so the
    // node survives unmount, isolating the listener-removal assertion.
    externalRef?: Ref<HTMLElement | null>;
};

// Single host-component factory shared by every test. Each call closes over its
// own `onOutside` callback and options, but there is exactly ONE `defineComponent`
// definition in this file, satisfying vue/one-component-per-file — these are
// throwaway test fixtures, not application components.
const makeHost = (onOutside: () => void, options: HostOptions = {}): Component => {
    const { bindRef = true, externalRef } = options;

    return defineComponent({
        setup(): () => VNode {
            const elementRef: Ref<HTMLElement | null> = externalRef ?? ref<HTMLElement | null>(null);

            useClickOutside(elementRef, onOutside);

            return (): VNode => {
                if (bindRef === false) {
                    return h('div', 'no ref bound');
                }

                return h('div', { ref: elementRef, id: 'inside-root' }, [
                    h('span', { id: 'inside-child' }, 'inner'),
                ]);
            };
        },
    });
};

describe('useClickOutside', () => {
    beforeEach((): void => {
        vi.restoreAllMocks();
    });

    it('fires the callback on a click outside the element (listener attached on mount)', (): void => {
        const onOutside = vi.fn();
        const wrapper = mount(makeHost(onOutside), { attachTo: document.body });

        // target === document, which is outside the ref → callback fires.
        // This also proves the document listener was attached on mount.
        document.dispatchEvent(new MouseEvent('click', { bubbles: true }));

        expect(onOutside).toHaveBeenCalledTimes(1);

        wrapper.unmount();
    });

    it('does NOT fire when the click lands inside the element', (): void => {
        const onOutside = vi.fn();
        const wrapper = mount(makeHost(onOutside), { attachTo: document.body });

        // Real bubbling click on a descendant node → target is inside the ref.
        const child = wrapper.get('#inside-child').element;

        child.dispatchEvent(new MouseEvent('click', { bubbles: true }));

        expect(onOutside).not.toHaveBeenCalled();

        wrapper.unmount();
    });

    it('does NOT fire when the click lands on the ref element itself', (): void => {
        const onOutside = vi.fn();
        const wrapper = mount(makeHost(onOutside), { attachTo: document.body });

        // Node.contains(node) is true for the node itself → counts as inside.
        const root = wrapper.get('#inside-root').element;

        root.dispatchEvent(new MouseEvent('click', { bubbles: true }));

        expect(onOutside).not.toHaveBeenCalled();

        wrapper.unmount();
    });

    it('removes the document listener on unmount (no callback after unmount)', (): void => {
        const onOutside = vi.fn();

        // A plain ref to a detached node — Vue never nulls it (it is not a template
        // ref bound to a rendered node), so it survives unmount. That isolates the
        // assertion to listener removal: if onUnmounted's removeEventListener is gone,
        // el.contains(document) === false on the next click and the callback fires.
        // bindRef:false keeps the host from overwriting the externally-supplied ref.
        const el = document.createElement('div');
        const elementRef = ref<HTMLElement | null>(el);
        const wrapper = mount(makeHost(onOutside, { bindRef: false, externalRef: elementRef }));

        wrapper.unmount();

        document.dispatchEvent(new MouseEvent('click', { bubbles: true }));

        expect(onOutside).not.toHaveBeenCalled();
    });

    it('does NOT fire while the ref is still null (guard branch)', (): void => {
        const onOutside = vi.fn();
        const wrapper = mount(makeHost(onOutside, { bindRef: false }), { attachTo: document.body });

        document.dispatchEvent(new MouseEvent('click', { bubbles: true }));

        expect(onOutside).not.toHaveBeenCalled();

        wrapper.unmount();
    });
});
