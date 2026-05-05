---
layout: 'page'
uri: '/guides/frontend-utils'
position: 3
slug: 'guides-frontend-utils'
parent: 'guides'
navTitle: 'Frontend Utils'
title: 'Frontend Utils'
description: 'Přehled všech frontend utilit — fetch, auth, toast, modals.'
---

# Frontend Utils

Rychlý přehled co je k dispozici a odkud to importovat.

| Utilita | Import | Kdy použít |
|---|---|---|
| [`apiFetch`](#apifetch) | `@/app-ui/Fetch` | Public endpoint (`/health`, bez JWT) |
| [`authFetch`](#authfetch) | `@/app-ui/Auth` | Protected API — auto-refresh na 401 |
| [`apiUpload`](#apiupload) | `@/app-ui/Fetch` | Upload souboru s progress |
| [`apiDownload`](#apidownload) | `@/app-ui/Fetch` | Download souboru (browser dialog) |
| [`useAuth`](#useauth) | `@/app-ui/Auth` | Session state, login/logout, permissions |
| [`useToast`](#usetoast) | `@/app-ui/Toast/useToast` | Notifikace |
| [`useClickOutside`](#useclickoutside) | `@/app-ui/ClickOutside/useClickOutside` | Detekce kliku mimo element |
| [`Dropdown`](#dropdown) | `@/app-ui/Dropdown/Dropdown.vue` | Click-outside dropdown menu (slot trigger + slot menu) |
| [`Modal`](#modal), [`ConfirmModal`](#confirmmodal) | `@/app-ui/Modals/*` | Dialogy |


## apiFetch

```typescript
import { apiFetch } from '@/app-ui/Fetch';

const result = await apiFetch<HealthResponse>('GET', '/health');
const result = await apiFetch<LoginResponse>('POST', '/api/v1/auth/login', {
    body: { nickname: 'admin', password: 'secret' },
});
const result = await apiFetch<UserList, ValidationError>('GET', '/api/v1/users');

if (result.success === true) { result.data; }
if (result.success === false) { result.data; /* { message: string } default */ }
```

Automaticky přidává `Authorization: Bearer` header pokud je nastaven token. **Neretrí na 401** — pro to použij `authFetch`.


## authFetch

```typescript
import { authFetch } from '@/app-ui/Auth';

const result = await authFetch<UserProfile>('GET', '/api/v1/profile');
```

Stejné API jako `apiFetch`, navíc při 401 automaticky zavolá `refresh()` a request zopakuje s novým tokenem. Paralelní 401 sdílí jedno volání `/auth/refresh`. Skip pro `/api/v1/auth/*` (login/refresh/logout se neretrají).


## apiUpload

```typescript
import { apiUpload } from '@/app-ui/Fetch';

const result = await apiUpload<UploadResult>('/api/v1/files', formData, (stats) => {
    stats.percent;  // 0-100
    stats.loaded;   // bytes
    stats.total;    // bytes
});
```


## apiDownload

```typescript
import { apiDownload } from '@/app-ui/Fetch';

const result = await apiDownload('/api/v1/exports/report.csv', 'report.csv');
// result: { success: true, status: 200, filename: 'report-2026-04.csv' }
```

Filename z `Content-Disposition`, fallback na druhý parametr.


## useAuth

```typescript
import { useAuth } from '@/app-ui/Auth';

const {
    user,              // Readonly<Ref<AuthUser | null>>
    isAuthenticated,   // Readonly<Ref<boolean>>
    login,             // (credentials) => Promise<ApiResponse>
    logout,            // () => Promise<void>
    refresh,           // () => Promise<boolean>
    hasRole,           // (role) => boolean
    isAdmin,           // () => boolean
    hasPermission,     // (permission) => boolean
    hasAllPermissions, // (permissions[]) => boolean
    hasAnyPermission,  // (permissions[]) => boolean
} = useAuth();
```

`AuthUser` má tvar `{ id, nickname, email, role, permissions[] }`. Auto-refresh běží 30s před expirací access tokenu. Při hard refreshi stránky `assets/app.ts:bootstrap()` zavolá `refresh()` ještě před mountem routeru -- session se obnoví ze cookie tiše.


## useToast

```typescript
import { useToast } from '@/app-ui/Toast/useToast';

const { success, error, info, warning, clear } = useToast();

success('Uloženo');
error('Něco se pokazilo');
info('Informace', 5000);       // vlastní duration (ms)
warning('Pozor', null);        // null = bez auto-dismiss
clear();                       // smaže všechny toasty
```


## useClickOutside

```typescript
import { ref } from 'vue';
import { useClickOutside } from '@/app-ui/ClickOutside/useClickOutside';

const containerRef = ref<HTMLElement | null>(null);
const close = (): void => { /* … */ };

useClickOutside(containerRef, close);
```

Composable navěsí listener na `document` při mountu a sundá ho při unmountu. Když klik dopadne **mimo** `containerRef`, zavolá se `close`. Používá se v `Dropdown.vue` pro auto-close.


## Dropdown

```html
<Dropdown>
    <template #trigger>
        <button type="button">Open</button>
    </template>

    <RouterLink to="/profile" class="block px-4 py-2 hover:bg-gray-100">
        Profile
    </RouterLink>
    <button type="button" class="block w-full text-left px-4 py-2 text-red-600 hover:bg-red-50" @click="logout">
        Sign out
    </button>
</Dropdown>
```

Slot `trigger` je ovládací prvek (button, ikona). Default slot je obsah menu. Klik na položku menu auto-zavře dropdown (parent má `@click="close"`). Klik mimo dropdown taky zavře (`useClickOutside`).


## Modal

```html
<Modal :show="isOpen" title="Detail" @close="isOpen = false">
    <p>Obsah modalu</p>
</Modal>
```


## ConfirmModal

```html
<ConfirmModal
    :show="isConfirmOpen"
    title="Smazat?"
    message="Tuto akci nelze vrátit."
    @confirm="handleDelete"
    @cancel="isConfirmOpen = false"
/>
```
