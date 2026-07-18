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

// The arrow is the rotated square: the one span that is neither the bubble nor
// inside it. Found by shape rather than by a test hook, so the test says what the
// arrow IS instead of trusting a marker that could outlive the element.
const arrowOf = (wrapper: TooltipWrapper): ReturnType<TooltipWrapper['find']> =>
    wrapper.find('span.rotate-45');

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

    // The arrow points at the TRIGGER, at any trigger width. That is only true
    // because it is positioned against the WRAPPER (which is exactly the trigger's
    // box) rather than against the bubble — so it must not live inside the bubble.
    // Nested, its left/right would resolve against the bubble's box, and a
    // right-aligned bubble is offset from its trigger: the arrow then needs a
    // hard-coded nudge back, which can only be correct for one trigger width. It
    // used to be right-3 — right for the single 24px icon button that motivated
    // align, and wrong for every wider trigger.
    it('positions the arrow against the wrapper, not inside the bubble', () => {
        const wrapper = make('why this is off', { align: 'right' });

        expect(wrapper.find('[role="tooltip"] span').exists()).toBe(false);
    });

    it.each([
        ['center' as const],
        ['right' as const],
    ])('centres the arrow on the trigger when align=%s', (align) => {
        const arrow = arrowOf(make('why this is off', { align }));

        expect(arrow.classes()).toContain('left-1/2');
        expect(arrow.classes()).toContain('-translate-x-1/2');
        // No alignment-specific nudge may survive: it would be a trigger width
        // baked into a shared component.
        expect(arrow.classes()).not.toContain('right-3');
        expect(arrow.classes()).not.toContain('right-0');
    });

    // The arrow sits astride the bubble's near edge, on the same side the bubble
    // is. As a child it rode the bubble's frame and inherited this; as a sibling it
    // resolves against the trigger, so it has to close the gap from its own side.
    it.each([
        ['top' as const, 'bottom-full', 'mb-1'],
        ['bottom' as const, 'top-full', 'mt-1'],
    ])('anchors the arrow to the %s edge of the trigger', (position, edge, gap) => {
        const arrow = arrowOf(make('hi', { position }));

        expect(arrow.classes()).toContain(edge);
        expect(arrow.classes()).toContain(gap);
    });

    // The arrow is no longer inside the bubble, so nothing hides it for free — it
    // needs its own v-if and its own transition, or an empty tooltip leaves a
    // floating diamond and a hovered one animates in two pieces.
    it('renders NO arrow for empty text', () => {
        expect(arrowOf(make('')).exists()).toBe(false);
    });

    it('reveals the arrow on hover exactly like the bubble', () => {
        const arrow = arrowOf(make('hi'));

        expect(arrow.classes()).toContain('opacity-0');
        expect(arrow.classes()).toContain('group-hover:opacity-100');
        expect(arrow.classes()).toContain('pointer-events-none');
    });
});
