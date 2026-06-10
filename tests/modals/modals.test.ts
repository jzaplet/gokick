import { describe, expect, it, afterEach } from 'vitest';
import { mount } from '@vue/test-utils';
import type { VueWrapper } from '@vue/test-utils';
import { nextTick } from 'vue';
import Modal from '@/app-ui/Modals/Modal.vue';
import ConfirmModal from '@/app-ui/Modals/ConfirmModal.vue';

// Both modals Teleport their content to document.body, so assertions and
// click targets are looked up on the real body — NOT on the VTU wrapper
// (wrapper.find() does not reach teleported nodes). These are pure component
// tests: no network, no auth state, so the usual fetch/clearAuth harness
// boilerplate is intentionally omitted.
//
// Teleported DOM is not auto-cleaned between tests; the afterEach unmount is
// what keeps the suite deterministic (otherwise a later querySelector('button')
// could match a leftover modal from a previous test and pass for the wrong reason).

const findButtonByText = (text: string): HTMLButtonElement => {
    const match = [...document.body.querySelectorAll('button')].find(
        (btn) => btn.textContent.includes(text) === true,
    );

    if (match === undefined) {
        throw new Error(`button with text "${text}" not found in document.body`);
    }

    return match;
};

// guide-forms-fe-41: Modal accepts :show and title props and emits a close event.
describe('Modal', () => {
    let wrapper: VueWrapper | null = null;

    afterEach((): void => {
        wrapper?.unmount();
        wrapper = null;
    });

    it('renders the title in the teleported dialog when show is true', (): void => {
        wrapper = mount(Modal, {
            props: { show: true, title: 'Delete user?' },
            slots: { default: 'Body content here' },
        });

        // Sanity: the teleport actually landed in the body. If this is empty,
        // every assertion below would be vacuously testing nothing.
        expect(document.body.textContent).toContain('Delete user?');
        expect(document.body.textContent).toContain('Body content here');
    });

    it('does not render content when show is false', (): void => {
        wrapper = mount(Modal, {
            props: { show: false, title: 'Hidden title' },
            slots: { default: 'Hidden content' },
        });

        expect(document.body.textContent).not.toContain('Hidden title');
        expect(document.body.textContent).not.toContain('Hidden content');
    });

    it('emits "close" when the close button is clicked', async (): Promise<void> => {
        wrapper = mount(Modal, {
            props: { show: true, title: 'Closable' },
        });

        // Modal's only <button> is the close affordance (header X).
        findButtonByText('').click();
        await nextTick();

        expect(wrapper.emitted('close')).toHaveLength(1);
    });

    it('emits "close" when the backdrop overlay is clicked', async (): Promise<void> => {
        wrapper = mount(Modal, {
            props: { show: true, title: 'Backdrop close' },
        });

        const overlay = document.body.querySelector<HTMLDivElement>('div.bg-gray-900\\/50');

        expect(overlay).not.toBeNull();
        overlay?.click();
        await nextTick();

        expect(wrapper.emitted('close')).toHaveLength(1);
    });
});

// guide-forms-fe-42: ConfirmModal accepts :show, title, and message props and
// emits confirm and cancel events.
describe('ConfirmModal', () => {
    let wrapper: VueWrapper | null = null;

    afterEach((): void => {
        wrapper?.unmount();
        wrapper = null;
    });

    it('renders title and message in the teleported dialog when show is true', (): void => {
        wrapper = mount(ConfirmModal, {
            props: {
                show: true,
                title: 'Confirm deletion',
                message: 'This action cannot be undone.',
            },
        });

        expect(document.body.textContent).toContain('Confirm deletion');
        expect(document.body.textContent).toContain('This action cannot be undone.');
    });

    it('renders custom confirm/cancel button labels', (): void => {
        wrapper = mount(ConfirmModal, {
            props: {
                show: true,
                title: 'Delete?',
                message: 'Really delete?',
                confirmText: 'Yes, delete',
                cancelText: 'No, keep it',
            },
        });

        expect(findButtonByText('Yes, delete')).toBeTruthy();
        expect(findButtonByText('No, keep it')).toBeTruthy();
    });

    it('emits "confirm" when the confirm button is clicked', async (): Promise<void> => {
        wrapper = mount(ConfirmModal, {
            props: { show: true, title: 'Delete?', message: 'Sure?' },
        });

        // Select by text — the confirm control carries the default "Confirm" label.
        findButtonByText('Confirm').click();
        await nextTick();

        expect(wrapper.emitted('confirm')).toHaveLength(1);
        expect(wrapper.emitted('cancel')).toBeUndefined();
    });

    it('emits "cancel" when the cancel button is clicked', async (): Promise<void> => {
        // Fresh mount: ConfirmModal flips its internal isVisible to false after a
        // click, removing the buttons — so confirm and cancel cannot share a mount.
        wrapper = mount(ConfirmModal, {
            props: { show: true, title: 'Delete?', message: 'Sure?' },
        });

        findButtonByText('Cancel').click();
        await nextTick();

        expect(wrapper.emitted('cancel')).toHaveLength(1);
        expect(wrapper.emitted('confirm')).toBeUndefined();
    });

    it('emits "cancel" when the backdrop overlay is clicked', async (): Promise<void> => {
        wrapper = mount(ConfirmModal, {
            props: { show: true, title: 'Delete?', message: 'Sure?' },
        });

        const overlay = document.body.querySelector<HTMLDivElement>('div.bg-gray-900\\/50');

        expect(overlay).not.toBeNull();
        overlay?.click();
        await nextTick();

        expect(wrapper.emitted('cancel')).toHaveLength(1);
    });
});
