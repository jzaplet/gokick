---
layout: 'page'
uri: '/guides/frontend-utils'
position: 2
slug: 'guides-frontend-utils'
parent: 'guides'
navTitle: 'Frontend Utils'
title: 'Frontend Utils'
description: 'Composables (useFetch, useAuth) a sdílené UI komponenty (Toast, Modals).'
---

# Frontend Utils


## useFetch

Generický HTTP klient s typovanou response. Vždy se píše celá URL.

```typescript
import { apiFetch, apiUpload, apiDownload } from '@/app-ui/Fetch/useFetch';

// GET
const result = await apiFetch<HealthResponse>('GET', '/health');

// POST s body
const result = await apiFetch<LoginResponse>('POST', '/api/v1/auth/login', {
    body: { nickname: 'admin', password: 'secret' },
});

// Discriminated union response
if (result.success === true) {
    result.data;   // HealthResponse
}
if (result.success === false) {
    result.data;   // { message: string } (default) nebo vlastní TError
}

// Custom error typ
const result = await apiFetch<UserList, ValidationError>('GET', '/api/v1/admin/users');

// Upload s progress
const result = await apiUpload<UploadResult>('/api/v1/files', formData, (stats) => {
    stats.percent;  // 0-100
    stats.loaded;   // bytes
    stats.total;    // bytes
});

// Download souboru (spustí browser download dialog)
const result = await apiDownload('/api/v1/exports/report.csv', 'report.csv');
// result: { success: true, status: 200, filename: 'report-2026-04.csv' }
// filename se parsuje z Content-Disposition headeru, fallback na druhý parametr
```

Všechny tři funkce automaticky přidávají `Authorization: Bearer` header když je nastaven token.


## useAuth

Composable pro autentizaci, role a permissions.

```typescript
import { useAuth } from '@/app-ui/Auth/useAuth';

const {
    user,              // Ref<AuthUser | null>
    isAuthenticated,   // Ref<boolean>
    login,             // (credentials) => Promise<ApiResponse>
    logout,            // () => Promise<void>
    refresh,           // () => Promise<boolean>
    hasRole,           // (role: string) => boolean
    isAdmin,           // () => boolean
    hasPermission,     // (permission: string) => boolean
    hasAllPermissions, // (permissions: string[]) => boolean
    hasAnyPermission,  // (permissions: string[]) => boolean
} = useAuth();
```

| Metoda | Popis |
|---|---|
| `login(credentials)` | POST login, uloží token, naplánuje auto-refresh |
| `logout()` | POST logout, vyčistí token + stav |
| `refresh()` | POST refresh, obnoví token nebo vyčistí stav |
| `hasRole(role)` | Kontrola role uživatele |
| `isAdmin()` | Shortcut pro `hasRole('admin')` |
| `hasPermission(p)` | Admin má vždy true, jinak hledá v `permissions[]` |
| `hasAllPermissions(ps)` | Všechny permissions musí být splněny |
| `hasAnyPermission(ps)` | Stačí jedna permission |

Auto-refresh běží 30s před expirací access tokenu.


## Toast

Globální notifikace. `ToastContainer` je v root `App.vue`, stačí volat funkce.

```typescript
import { useToast } from '@/app-ui/Toast/useToast';

const { success, error, info, warning, clear } = useToast();

success('Uloženo');
error('Něco se pokazilo');
info('Informace', 5000);       // vlastní duration (ms)
warning('Pozor', null);        // null = bez auto-dismiss
clear();                       // smaže všechny toasty
```


## Modals

Dva typy — obecný `Modal` a potvrzovací `ConfirmModal`.

```html
<Modal :show="isOpen" title="Detail" @close="isOpen = false">
    <p>Obsah modalu</p>
</Modal>

<ConfirmModal
    :show="isConfirmOpen"
    title="Smazat?"
    message="Tuto akci nelze vrátit."
    @confirm="handleDelete"
    @cancel="isConfirmOpen = false"
/>
```
