import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import DataGrid from '@/app-ui/DataGrid/DataGrid.vue';

type GridWrapper = ReturnType<typeof mount<typeof DataGrid>>;

const make = (selectable: boolean, isLoading = false): GridWrapper =>
    mount(DataGrid, {
        props: {
            columns: [
                { key: 'nickname', label: 'Nickname', sortable: true },
                { key: 'email', label: 'Email' },
            ],
            sort: { column: 'nickname', direction: 'ASC' },
            isLoading,
            selectable,
        },
        slots: {
            rows: '<tr><td>alice</td><td>a@x.dev</td></tr>',
        },
    });

describe('DataGrid', () => {
    it('emits sort only for sortable columns', async (): Promise<void> => {
        const wrapper = make(false);
        const headers = wrapper.findAll('th');

        await headers[0]?.trigger('click');
        await headers[1]?.trigger('click');

        expect(wrapper.emitted('sort')).toEqual([['nickname']]);
    });

    it('renders the header checkbox only when selectable and emits togglePage', async (): Promise<void> => {
        const plain = make(false);

        expect(plain.find('input[type="checkbox"]').exists()).toBe(false);

        const selectable = make(true);
        const box = selectable.find('input[type="checkbox"]');

        expect(box.exists()).toBe(true);

        await box.trigger('change');

        expect(selectable.emitted('togglePage')).toHaveLength(1);
    });

    it('replaces rows with the loading row while loading', () => {
        const wrapper = make(true, true);

        expect(wrapper.text()).not.toContain('alice');
        expect(wrapper.findAll('tbody tr')).toHaveLength(1);
        expect(wrapper.find('tbody td').attributes('colspan')).toBe('3');
    });
});
