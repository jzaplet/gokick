import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import Pagination from '@/app-ui/Pagination/Pagination.vue';

type PaginationWrapper = ReturnType<typeof mount<typeof Pagination>>;

const make = (page: number, perPage: number, total: number): PaginationWrapper =>
    mount(Pagination, { props: { page, perPage, total } });

const pageButtons = (wrapper: PaginationWrapper): string[] =>
    wrapper.findAll('nav button').map((b) => b.text());

describe('Pagination', () => {
    it('shows the range summary', () => {
        expect(make(2, 25, 60).text()).toContain('Showing 26–50 of 60');
        expect(make(1, 25, 0).text()).toContain('Showing 0–0 of 0');
    });

    it('hides the page buttons for a single page', () => {
        expect(make(1, 25, 10).find('nav').exists()).toBe(false);
    });

    it('windows to five pages centered on the current one', () => {
        expect(pageButtons(make(7, 10, 200))).toEqual(['Prev', '5', '6', '7', '8', '9', 'Next']);
        expect(pageButtons(make(1, 10, 200))).toEqual(['Prev', '1', '2', '3', '4', '5', 'Next']);
        expect(pageButtons(make(20, 10, 200))).toEqual(['Prev', '16', '17', '18', '19', '20', 'Next']);
    });

    it('emits update:page for valid targets only', async (): Promise<void> => {
        const wrapper = make(2, 10, 50);
        const buttons = wrapper.findAll('nav button');

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
