import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import Pagination from '@/app-ui/Pagination/Pagination.vue';

type PaginationWrapper = ReturnType<typeof mount<typeof Pagination>>;

const make = (page: number, perPage: number, total: number): PaginationWrapper =>
    mount(Pagination, { props: { page, perPage, total } });

// Page-number buttons only — prev/next are chevron icons without text.
const pageButtons = (wrapper: PaginationWrapper): string[] =>
    wrapper.findAll('nav button')
        .map((b) => b.text())
        .filter((t) => t !== '');

describe('Pagination', () => {
    it('shows the range summary and hides itself for an empty result', () => {
        expect(make(2, 25, 60).text()).toContain('26–50 / 60');
        expect(make(1, 25, 0).find('div').exists()).toBe(false);
    });

    it('hides the page buttons for a single page', () => {
        expect(make(1, 25, 10).find('nav').exists()).toBe(false);
    });

    it('windows to five pages centered on the current one', () => {
        expect(pageButtons(make(7, 10, 200))).toEqual(['5', '6', '7', '8', '9']);
        expect(pageButtons(make(1, 10, 200))).toEqual(['1', '2', '3', '4', '5']);
        expect(pageButtons(make(20, 10, 200))).toEqual(['16', '17', '18', '19', '20']);
    });

    it('emits update:page for valid targets only', async (): Promise<void> => {
        const wrapper = make(2, 10, 50);
        const buttons = wrapper.findAll('nav button');

        // First button is the prev chevron.
        await buttons[0]?.trigger('click');

        expect(wrapper.emitted('update:page')).toEqual([[1]]);

        // Clicking the CURRENT page must not emit.
        const current = buttons.find((b) => b.text() === '2');

        await current?.trigger('click');

        expect(wrapper.emitted('update:page')).toEqual([[1]]);
    });

    it('disables prev on the first and next on the last page', () => {
        const first = make(1, 10, 30).findAll('nav button');
        const last = make(3, 10, 30).findAll('nav button');

        expect(first[0]?.attributes('disabled')).toBeDefined();
        expect(last[last.length - 1]?.attributes('disabled')).toBeDefined();
    });
});
