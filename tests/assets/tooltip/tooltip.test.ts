import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import Tooltip from '@/app-ui/Tooltip/Tooltip.vue';

type TooltipWrapper = ReturnType<typeof mount<typeof Tooltip>>;

const make = (
    text: string,
    options: {
        position?: 'top' | 'bottom';
        align?: 'center' | 'right';
        maxWidth?: number;
    } = {},
): TooltipWrapper =>
    mount(Tooltip, {
        props: { text, ...options },
        slots: { default: '<button>trigger</button>' },
    });

describe('Tooltip', () => {
    it('renders the trigger slot with a hover-revealed bubble', () => {
        const wrapper = make('Activate user');
        const bubble = wrapper.find('[role="tooltip"]');

        expect(wrapper.find('button').text()).toBe('trigger');
        expect(bubble.text()).toContain('Activate user');
        // CSS-only reveal: hidden by default, shown on group hover.
        expect(bubble.classes()).toContain('opacity-0');
        expect(bubble.classes()).toContain('group-hover:opacity-100');
    });

    it('renders NO bubble for empty text', () => {
        expect(make('').find('[role="tooltip"]').exists()).toBe(false);
    });

    it('positions above by default and below on demand', () => {
        expect(make('hi').find('[role="tooltip"]').classes()).toContain('bottom-full');
        expect(make('hi', { position: 'bottom' }).find('[role="tooltip"]').classes()).toContain('top-full');
    });

    it('switches to a wrapped, width-capped bubble with maxWidth', () => {
        const bubble = make('a longer explanation', { maxWidth: 200 }).find('[role="tooltip"]');

        expect(bubble.classes()).toContain('whitespace-normal');
        expect(bubble.attributes('style')).toContain('max-width: 200px');
    });

    // A centred bubble on a trigger at the right edge of its container hangs half
    // its width past the viewport and gets clipped (measured: a 200px bubble on
    // the tenants grid's actions column ran to 1308px in a 1280px viewport).
    // align='right' anchors the bubble's right edge to the trigger instead.
    it('anchors the bubble to the right edge when align=right', () => {
        const bubble = make('why this is off', { align: 'right' }).find('[role="tooltip"]');

        expect(bubble.classes()).toContain('right-0');
        // The centring pair must be GONE, not merely overridden — both would
        // apply and Tailwind's output order, not intent, would pick the winner.
        expect(bubble.classes()).not.toContain('left-1/2');
        expect(bubble.classes()).not.toContain('-translate-x-1/2');
    });

    it('keeps the default bubble centred', () => {
        const bubble = make('hi').find('[role="tooltip"]');

        expect(bubble.classes()).toContain('left-1/2');
        expect(bubble.classes()).not.toContain('right-0');
    });

    // The arrow points at the TRIGGER. A right-aligned bubble is offset from its
    // trigger, so an arrow centred on the bubble would point at empty space.
    it('moves the arrow over the trigger when align=right', () => {
        const arrow = make('why this is off', { align: 'right' })
            .find('[role="tooltip"] span');

        expect(arrow.classes()).toContain('right-3');
        expect(arrow.classes()).not.toContain('left-1/2');
    });
});
