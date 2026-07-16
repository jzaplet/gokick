import type { Ref } from 'vue';
import { describe, expect, it, vi, beforeEach } from 'vitest';

// useSidebar keeps module-level singleton state (the shell renders once), so
// each test gets a fresh module via resetModules + dynamic import.
const setViewportWidth = (width: number): void => {
    Object.defineProperty(window, 'innerWidth', {
        value: width,
        configurable: true,
        writable: true,
    });
};

type SidebarModule = {
    useSidebar: () => {
        isSidebarOpen: Readonly<Ref<boolean>>;
        toggleSidebar: () => void;
        closeSidebar: () => void;
        closeOnMobileNav: () => void;
    };
};

const freshSidebar = async (): Promise<SidebarModule> => {
    vi.resetModules();

    return import('@/app/Layout/useSidebar');
};

describe('useSidebar', () => {
    beforeEach((): void => {
        localStorage.clear();
    });

    it('defaults to open on desktop', async (): Promise<void> => {
        setViewportWidth(1200);
        const { useSidebar } = await freshSidebar();

        expect(useSidebar().isSidebarOpen.value).toBe(true);
    });

    it('respects a stored closed preference on desktop', async (): Promise<void> => {
        setViewportWidth(1200);
        localStorage.setItem('gk_sidebar_open', '0');
        const { useSidebar } = await freshSidebar();

        expect(useSidebar().isSidebarOpen.value).toBe(false);
    });

    it('starts closed on mobile regardless of the stored preference', async (): Promise<void> => {
        setViewportWidth(500);
        localStorage.setItem('gk_sidebar_open', '1');
        const { useSidebar } = await freshSidebar();

        expect(useSidebar().isSidebarOpen.value).toBe(false);
    });

    it('toggle persists the preference on desktop', async (): Promise<void> => {
        setViewportWidth(1200);
        const { useSidebar } = await freshSidebar();
        const sidebar = useSidebar();

        sidebar.toggleSidebar();

        expect(sidebar.isSidebarOpen.value).toBe(false);
        expect(localStorage.getItem('gk_sidebar_open')).toBe('0');

        sidebar.toggleSidebar();

        expect(localStorage.getItem('gk_sidebar_open')).toBe('1');
    });

    it('toggle on mobile never persists — the overlay is transient', async (): Promise<void> => {
        setViewportWidth(500);
        const { useSidebar } = await freshSidebar();
        const sidebar = useSidebar();

        sidebar.toggleSidebar();

        expect(sidebar.isSidebarOpen.value).toBe(true);
        expect(localStorage.getItem('gk_sidebar_open')).toBeNull();
    });

    it('closes when the viewport shrinks below the breakpoint', async (): Promise<void> => {
        setViewportWidth(1200);
        const { useSidebar } = await freshSidebar();
        const sidebar = useSidebar();

        expect(sidebar.isSidebarOpen.value).toBe(true);

        setViewportWidth(500);
        window.dispatchEvent(new Event('resize'));

        expect(sidebar.isSidebarOpen.value).toBe(false);
    });

    it('restores the stored preference when growing back to desktop', async (): Promise<void> => {
        setViewportWidth(500);
        const { useSidebar } = await freshSidebar();
        const sidebar = useSidebar();

        expect(sidebar.isSidebarOpen.value).toBe(false);

        setViewportWidth(1200);
        window.dispatchEvent(new Event('resize'));

        expect(sidebar.isSidebarOpen.value).toBe(true);
    });

    it('closeOnMobileNav closes only below the breakpoint', async (): Promise<void> => {
        setViewportWidth(1200);
        const { useSidebar } = await freshSidebar();
        const sidebar = useSidebar();

        sidebar.closeOnMobileNav();

        expect(sidebar.isSidebarOpen.value).toBe(true);

        setViewportWidth(500);
        window.dispatchEvent(new Event('resize'));
        sidebar.toggleSidebar();
        sidebar.closeOnMobileNav();

        expect(sidebar.isSidebarOpen.value).toBe(false);
    });
});
