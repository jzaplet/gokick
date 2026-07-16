import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import BulkActionBar from '@/app-ui/BulkActions/BulkActionBar.vue';

type BarWrapper = ReturnType<typeof mount<typeof BulkActionBar>>;

const make = (count: number, total: number, isAllFiltered = false): BarWrapper =>
    mount(BulkActionBar, {
        props: {
            count,
            total,
            isAllFiltered,
            actions: [
                { key: 'activate', label: 'Activate' },
                { key: 'delete', label: 'Delete' },
            ],
        },
    });

const openDropdown = async (wrapper: BarWrapper): Promise<void> => {
    // The orange count pill is the first button in the bar.
    await wrapper.find('button').trigger('click');
};

describe('BulkActionBar', () => {
    it('renders nothing without a selection', () => {
        expect(make(0, 50).find('div').exists()).toBe(false);
    });

    it('shows the summary and the escalation to all filtered', async (): Promise<void> => {
        const wrapper = make(3, 50);

        expect(wrapper.text()).toContain('Selected: 3 / 50');

        const escalate = wrapper.findAll('button').find((b) => b.text().includes('Select all (50)'));

        expect(escalate).toBeDefined();

        await escalate?.trigger('click');

        expect(wrapper.emitted('selectAllFiltered')).toHaveLength(1);
    });

    it('shows the all-filtered summary WITHOUT the escalation link', () => {
        const wrapper = make(50, 50, true);

        expect(wrapper.text()).toContain('Selected: all (50)');
        expect(wrapper.findAll('button').some((b) => b.text().includes('Select all ('))).toBe(false);
    });

    it('opens the dropdown and emits the action key', async (): Promise<void> => {
        const wrapper = make(2, 50);

        // Actions live in the dropdown — hidden until the pill is clicked.
        expect(wrapper.findAll('button').some((b) => b.text().includes('Delete'))).toBe(false);

        await openDropdown(wrapper);

        const item = wrapper.findAll('button').find((b) => b.text().includes('Delete (2x)'));

        expect(item).toBeDefined();

        await item?.trigger('click');

        expect(wrapper.emitted('action')).toEqual([['delete']]);
    });

    it('offers only deselect when there are no actions (tenants grid)', async (): Promise<void> => {
        const wrapper = mount(BulkActionBar, {
            props: { count: 2, total: 5, isAllFiltered: false, actions: [] },
        });

        await openDropdown(wrapper);

        const items = wrapper.findAll('button').filter((b) => b.text() !== '');

        // Pill, dropdown "Clear selection", escalation link, inline clear.
        expect(items.some((b) => b.text() === 'Clear selection')).toBe(true);
        expect(items.some((b) => b.text().includes('(2x)'))).toBe(false);
    });

    it('emits clear from the dropdown item and the inline link', async (): Promise<void> => {
        const wrapper = make(2, 50);

        await openDropdown(wrapper);

        const clears = wrapper.findAll('button').filter((b) => b.text() === 'Clear selection');

        // One inside the dropdown, one inline at the end of the bar.
        expect(clears).toHaveLength(2);

        await clears[0]?.trigger('click');
        await clears[1]?.trigger('click');

        expect(wrapper.emitted('clear')).toHaveLength(2);
    });
});
