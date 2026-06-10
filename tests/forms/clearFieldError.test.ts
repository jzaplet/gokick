import { describe, expect, it, vi, beforeEach } from 'vitest';
import { flushPromises, mount } from '@vue/test-utils';
import type { VueWrapper } from '@vue/test-utils';
import { createMemoryHistory, createRouter } from 'vue-router';
import type { Router } from 'vue-router';
import LoginForm from '@/app/Auth/Components/LoginForm.vue';
import Input from '@/app-ui/Inputs/Input.vue';
import ErrorAlert from '@/app-ui/Alerts/ErrorAlert.vue';
import { clearAuth } from '@/app-ui/Auth';
import { setAccessToken } from '@/app-ui/Fetch/accessToken';

type InputWrapper = VueWrapper<InstanceType<typeof Input>>;

// guide-forms-fe-13: clearFieldError deletes a single key from errors.value,
// removing only that field's error.
//
// clearFieldError is NOT a shared composable — it is defined inline inside the
// form component and wired to each Input's @update:model-value. We therefore
// exercise it through the real component: drive errors.value via a failed
// login (mocked fetch → 400 with field errors), then emit update:modelValue
// on one Input and assert ONLY that field's error is gone while the others
// (a sibling field error + the general error) remain. Assertions read the
// `error` / `message` props the form passes down, which mirror errors.value
// reactively — no reach into private setup state.

const Blank = { template: '<div />' };

const makeTestRouter = (): Router => {
    const router = createRouter({
        history: createMemoryHistory(),
        routes: [
            { path: '/', name: 'home', component: Blank },
            { path: '/login', name: 'login', component: Blank },
            { path: '/user/dashboard', name: 'user-dashboard', component: Blank },
            { path: '/admin/dashboard', name: 'admin-dashboard', component: Blank },
        ],
    });

    return router;
};

const mountForm = async (): Promise<ReturnType<typeof mount>> => {
    const router = makeTestRouter();

    await router.push('/login');
    await router.isReady();

    const wrapper = mount(LoginForm, {
        global: {
            plugins: [router],
            // Button pulls a Spinner/icon tree we don't care about here.
            stubs: { Button: true },
        },
    });

    await flushPromises();

    return wrapper;
};

// Populate errors.value with three keys (nickname + password as field errors,
// general as a non-field error) by submitting the form against a mocked 400.
const seedThreeErrors = async (wrapper: ReturnType<typeof mount>): Promise<void> => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
        new Response(
            JSON.stringify({
                nickname: 'Nickname is required.',
                password: 'Password is too short.',
                general: 'Invalid credentials.',
            }),
            { status: 400 },
        ),
    );

    await wrapper.find('form').trigger('submit.prevent');
    await flushPromises();
};

const nicknameInput = (wrapper: ReturnType<typeof mount>): InputWrapper | undefined =>
    wrapper.findAllComponents(Input).find((c): boolean => c.props('name') === 'nickname');

const passwordInput = (wrapper: ReturnType<typeof mount>): InputWrapper | undefined =>
    wrapper.findAllComponents(Input).find((c): boolean => c.props('name') === 'password');

// clearFieldError is wired to each Input's @update:model-value, so the test
// drives it by emitting that event on the chosen Input. @vue/test-utils types an
// SFC wrapper's $emit as `any` under type-aware lint, so the one unavoidable cast
// is contained here behind a precise signature instead of disabled at every call.
const emitUpdate = (input: InputWrapper | undefined, value: string): void => {
    if (input === undefined) {
        throw new Error('expected the Input component to be mounted');
    }

    // eslint-disable-next-line @typescript-eslint/no-unsafe-member-access -- SFC wrapper $emit is typed `any`
    const emit = input.vm.$emit as (event: 'update:modelValue', value: string) => void;

    emit('update:modelValue', value);
};

describe('clearFieldError (LoginForm inline)', () => {
    beforeEach((): void => {
        clearAuth();
        setAccessToken(null);
        vi.restoreAllMocks();
    });

    it('seeds three errors then removes only the edited field, leaving the rest', async (): Promise<void> => {
        const wrapper = await mountForm();

        await seedThreeErrors(wrapper);

        // Precondition: all three errors are present after the failed login.
        expect(nicknameInput(wrapper)?.props('error')).toBe('Nickname is required.');
        expect(passwordInput(wrapper)?.props('error')).toBe('Password is too short.');
        expect(wrapper.getComponent(ErrorAlert).props('message')).toBe('Invalid credentials.');

        // Editing the nickname field emits update:modelValue → clearFieldError('nickname').
        emitUpdate(nicknameInput(wrapper), 'newnick');
        await flushPromises();

        // Only the nickname key is deleted.
        expect(nicknameInput(wrapper)?.props('error')).toBeUndefined();
        // The other field error and the general error are untouched.
        expect(passwordInput(wrapper)?.props('error')).toBe('Password is too short.');
        expect(wrapper.getComponent(ErrorAlert).props('message')).toBe('Invalid credentials.');
    });

    it('clears each field independently (editing password removes only password)', async (): Promise<void> => {
        const wrapper = await mountForm();

        await seedThreeErrors(wrapper);

        emitUpdate(passwordInput(wrapper), 'newpass');
        await flushPromises();

        expect(passwordInput(wrapper)?.props('error')).toBeUndefined();
        // Sibling field + general survive — clearFieldError touches a single key.
        expect(nicknameInput(wrapper)?.props('error')).toBe('Nickname is required.');
        expect(wrapper.getComponent(ErrorAlert).props('message')).toBe('Invalid credentials.');
    });

    it('clearing a field whose key is absent is a no-op for the others', async (): Promise<void> => {
        const wrapper = await mountForm();

        // Only a general error this time — no nickname key present.
        vi.spyOn(globalThis, 'fetch').mockResolvedValue(
            new Response(JSON.stringify({ general: 'Invalid credentials.' }), { status: 400 }),
        );
        await wrapper.find('form').trigger('submit.prevent');
        await flushPromises();

        expect(nicknameInput(wrapper)?.props('error')).toBeUndefined();
        expect(wrapper.getComponent(ErrorAlert).props('message')).toBe('Invalid credentials.');

        // Editing nickname deletes a key that was never set — general must survive.
        emitUpdate(nicknameInput(wrapper), 'x');
        await flushPromises();

        expect(nicknameInput(wrapper)?.props('error')).toBeUndefined();
        expect(wrapper.getComponent(ErrorAlert).props('message')).toBe('Invalid credentials.');
    });
});
