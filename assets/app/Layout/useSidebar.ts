import type { Ref } from 'vue';
import { readonly, ref } from 'vue';

// Sidebar open/closed state shared by AppLayout (content margin), AppSidebar
// (width + backdrop) and AppHeader (toggle button). Module-level singleton on
// purpose — the shell renders once and every consumer must see the same state
// (same pattern as the Toast store).
//
// Behavior (ported from the aibobr admin shell): desktop (≥768px) defaults to
// open and remembers the user's choice in localStorage; mobile is always
// closed on load and on shrinking past the breakpoint — there the sidebar is a
// transient off-canvas overlay, so its state is never persisted.

const STORAGE_KEY = 'gk_sidebar_open';
const DESKTOP_MIN_WIDTH = 768;

const isOpen = ref(false);

let initialized = false;

const isDesktop = (): boolean => window.innerWidth >= DESKTOP_MIN_WIDTH;

const storedPreference = (): boolean => localStorage.getItem(STORAGE_KEY) !== '0';

const applyViewport = (): void => {
    isOpen.value = isDesktop() === true ? storedPreference() : false;
};

const toggleSidebar = (): void => {
    isOpen.value = isOpen.value === false;

    if (isDesktop() === true) {
        localStorage.setItem(STORAGE_KEY, isOpen.value === true ? '1' : '0');
    }
};

const closeSidebar = (): void => {
    isOpen.value = false;
};

// closeOnMobileNav collapses the overlay after a nav click on mobile — an SPA
// route change keeps the page alive, so unlike an MPA the overlay would stay
// open over the new view.
const closeOnMobileNav = (): void => {
    if (isDesktop() === false) {
        closeSidebar();
    }
};

type SidebarApi = {
    isSidebarOpen: Readonly<Ref<boolean>>;
    toggleSidebar: () => void;
    closeSidebar: () => void;
    closeOnMobileNav: () => void;
};

export const useSidebar = (): SidebarApi => {
    if (initialized === false) {
        initialized = true;
        applyViewport();
        window.addEventListener('resize', applyViewport);
    }

    return {
        isSidebarOpen: readonly(isOpen),
        toggleSidebar,
        closeSidebar,
        closeOnMobileNav,
    };
};
