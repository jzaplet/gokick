import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import Tooltip from '@/app-ui/Tooltip/Tooltip.vue';

type TooltipWrapper = ReturnType<typeof mount<typeof Tooltip>>;

const make = (
    text: string,
    options: { position?: 'top' | 'bottom'; maxWidth?: number } = {},
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
});
