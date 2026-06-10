import { describe, expect, it, beforeEach, afterEach } from 'vitest';
import { mount } from '@vue/test-utils';
import type { VueWrapper } from '@vue/test-utils';
import Dropdown from '@/app-ui/Dropdown/Dropdown.vue';

// Dropdown has no props/emits. The `trigger` slot is the clickable control;
// the default slot is the menu, rendered only while open. It closes on
// click-outside (document listener via useClickOutside) and on click inside
// the menu.
//
// The menu is wrapped in a <Transition>. In jsdom there is no real CSS
// transition, so the leave animation resolves synchronously and a single
// nextTick (await wrapper.vm.$nextTick / await flush) is enough to observe
// the post-toggle DOM. The component must be attached to the live document
// (attachTo) so the click-outside `document` listener actually fires for
// dispatched DOM events.

const TRIGGER = '<button class="trigger-btn">open</button>';
const MENU = '<div class="menu-item">menu</div>';

const mountDropdown = (): VueWrapper<InstanceType<typeof Dropdown>> => mount(Dropdown, {
    attachTo: document.body,
    slots: {
        trigger: TRIGGER,
        default: MENU,
    },
});

describe('Dropdown', () => {
    let wrapper: VueWrapper | null = null;

    beforeEach((): void => {
        document.body.innerHTML = '';
    });

    afterEach((): void => {
        if (wrapper !== null) {
            wrapper.unmount();
            wrapper = null;
        }
    });

    it('renders the trigger slot as the control and hides the menu initially', (): void => {
        wrapper = mountDropdown();

        // Trigger (control) is always rendered.
        expect(wrapper.find('.trigger-btn').exists()).toBe(true);
        // Menu (default slot) is not rendered until opened.
        expect(wrapper.find('.menu-item').exists()).toBe(false);
    });

    it('opens the menu when the trigger is clicked', async (): Promise<void> => {
        wrapper = mountDropdown();

        // The wrapping div around the trigger slot owns the @click toggle.
        await wrapper.find('.trigger-btn').trigger('click');

        expect(wrapper.find('.menu-item').exists()).toBe(true);
        expect(wrapper.find('.menu-item').text()).toBe('menu');
    });

    it('toggles closed when the trigger is clicked again', async (): Promise<void> => {
        wrapper = mountDropdown();

        await wrapper.find('.trigger-btn').trigger('click');
        expect(wrapper.find('.menu-item').exists()).toBe(true);

        await wrapper.find('.trigger-btn').trigger('click');
        expect(wrapper.find('.menu-item').exists()).toBe(false);
    });

    it('closes the menu when clicking inside the menu content', async (): Promise<void> => {
        wrapper = mountDropdown();

        await wrapper.find('.trigger-btn').trigger('click');
        expect(wrapper.find('.menu-item').exists()).toBe(true);

        // Click inside the menu — the menu container has an @click="close".
        await wrapper.find('.menu-item').trigger('click');

        expect(wrapper.find('.menu-item').exists()).toBe(false);
    });

    it('closes the menu on a click outside the component', async (): Promise<void> => {
        wrapper = mountDropdown();

        await wrapper.find('.trigger-btn').trigger('click');
        expect(wrapper.find('.menu-item').exists()).toBe(true);

        // An outside element dispatching a real DOM click event reaches the
        // document listener registered by useClickOutside.
        const outside = document.createElement('div');

        document.body.appendChild(outside);
        outside.dispatchEvent(new MouseEvent('click', { bubbles: true }));

        await wrapper.vm.$nextTick();

        expect(wrapper.find('.menu-item').exists()).toBe(false);
    });

    it('keeps the trigger control rendered regardless of open/closed state', async (): Promise<void> => {
        wrapper = mountDropdown();

        // Closed: trigger present, menu absent.
        expect(wrapper.find('.trigger-btn').exists()).toBe(true);

        // Open: trigger still present alongside the menu.
        await wrapper.find('.trigger-btn').trigger('click');
        expect(wrapper.find('.trigger-btn').exists()).toBe(true);
        expect(wrapper.find('.menu-item').exists()).toBe(true);
    });
});
