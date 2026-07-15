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
                { key: 'delete', label: 'Delete', variant: 'danger' },
            ],
        },
    });

describe('BulkActionBar', () => {
    it('renders nothing without a selection', () => {
        expect(make(0, 50).find('div').exists()).toBe(false);
    });

    it('shows the manual count and the escalation to all filtered', async (): Promise<void> => {
        const wrapper = make(3, 50);

        expect(wrapper.text()).toContain('3 selected');

        const escalate = wrapper.findAll('button').find((b) => b.text().includes('Select all 50'));

        expect(escalate).toBeDefined();

        await escalate?.trigger('click');

        expect(wrapper.emitted('selectAllFiltered')).toHaveLength(1);
    });

    it('shows the all-filtered summary WITHOUT the escalation link', () => {
        const wrapper = make(50, 50, true);

        expect(wrapper.text()).toContain('All 50 filtered selected');
        expect(wrapper.findAll('button').some((b) => b.text().includes('Select all'))).toBe(false);
    });

    it('emits the action key and clear', async (): Promise<void> => {
        const wrapper = make(2, 50);
        const buttons = wrapper.findAll('button');

        await buttons.find((b) => b.text() === 'Delete')?.trigger('click');

        expect(wrapper.emitted('action')).toEqual([['delete']]);

        await buttons.find((b) => b.text() === 'Clear')?.trigger('click');

        expect(wrapper.emitted('clear')).toHaveLength(1);
    });
});
