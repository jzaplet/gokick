import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import App from '@/App.vue';

describe('App', () => {
    it('mounts without errors', () => {
        const wrapper = mount(App, {
            global: {
                stubs: ['RouterView'],
            },
        });

        expect(wrapper.exists()).toBe(true);
    });
});
