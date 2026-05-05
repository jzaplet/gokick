import { describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import App from '@/App.vue';
import { routes } from '@/router/routes';

describe('App', () => {
    it('mounts without errors', async () => {
        const router = createRouter({
            history: createMemoryHistory(),
            routes,
        });

        await router.push('/');
        await router.isReady();

        const wrapper = mount(App, {
            global: {
                plugins: [router],
                stubs: ['RouterView'],
            },
        });

        expect(wrapper.exists()).toBe(true);
    });
});
