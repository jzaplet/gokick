---
layout: 'page'
uri: '/guides/forms'
position: 4
slug: 'guides-forms'
parent: 'guides'
navTitle: 'Forms & Validation'
title: 'Forms & Validation'
description: 'Jak napsat formulář, který mluví s API a renderuje chyby z backendu. Validace žije v doméně, frontend ji jen propisuje.'
---

# Forms & Validation

Validace je **server-side**. Frontend data nevaliduje, jen pošle a zobrazí, co backend vrátí. Chyby chodí jako `{ field: message }` — klíč přesně odpovídá poli ve formuláři.


## Princip v pěti krocích

1. Formulář pošle JSON přes `authFetch`.
2. Command/query handler vytvoří Value Objecty. První chybná VO vrací `*shared.ValidationError{Field, Message}`.
3. `response.Error()` zapíše JSON `{ "<field>": "<message>" }` se správným HTTP statusem (400 / 401 / 403).
4. Frontend: `errors.value = result.data`. Klíče z backendu **1:1** sedí na typ errorů.
5. `<Input :error="errors.field" />` a `<ErrorAlert :message="errors.general" />` renderují.


## Backend: validace v doméně

Value Object je místo, kde pravidla žijí. Ne v handleru, ne ve formuláři.

```go
// domain/user/nickname.go
func NewNickname(s string) (Nickname, error) {
    if s == "" {
        return "", &shared.ValidationError{Field: "nickname", Message: "nickname is required"}
    }
    if len(s) > 50 {
        return "", &shared.ValidationError{Field: "nickname", Message: "nickname must be at most 50 characters"}
    }
    return Nickname(s), nil
}
```

Handler volá VO za sebou a vrací první chybu, kterou potká:

```go
func (h *CreateUserHandler) Handle(ctx context.Context, cmd CreateUserCommand) error {
    nickname, err := user.NewNickname(cmd.Nickname)
    if err != nil { return err }  // → 400 { "nickname": "…" }

    role, err := user.NewRole(cmd.Role)
    if err != nil { return err }  // → 400 { "role": "…" }

    if existing != nil {
        return &shared.ValidationError{
            Field:   "nickname",
            Message: "user with this nickname already exists",
        }
    }
    // ... hash, save, collect event
}
```

Detaily error typů viz [Errors & Events](/framework/domain/errors-events), mapování na HTTP viz [HTTP Server](/framework/presentation/http-server).


## Response shape

Jedna chyba, jeden klíč:

```json
{ "nickname": "nickname is required" }    // ValidationError s polem
{ "general": "invalid credentials" }      // AuthError / PermissionError / bez pole
```

Víc klíčů najednou nechodí — handler vrací první chybu, na kterou narazí.


## Frontend: pass-through

### 1. Typ chyb

Každý formulář si napíše, jaké klíče očekává. Všechny optional.

```typescript
type ChangePasswordErrors = {
    general?: string;
    old_password?: string;
    new_password?: string;
};
```

### 2. State

```typescript
const errors = ref<ChangePasswordErrors>({});
```

Prázdný objekt = bez chyb. Klíč existuje = pole má chybu. `delete` klíče = chyba zmizela.

### 3. Submit

```typescript
const handleSubmit = async (): Promise<void> => {
    errors.value = {};
    isLoading.value = true;

    const result = await authFetch<null, ChangePasswordErrors>(
        'PUT',
        '/api/v1/profile/password',
        { body: form },
    );

    isLoading.value = false;

    if (result.success === false) {
        errors.value = result.data;   // ⬅ backend klíče = klíče typu
        return;
    }

    success('Password changed.');
    resetForm();
};
```

Klíčová řádka: `errors.value = result.data`. Žádné mapování, žádné `if` nad kódem chyby. Backend určí klíč, typ ho vynutí, render ho zobrazí.

### 4. Render

```html
<Input
    v-model="form.old_password"
    :error="errors.old_password"
    name="old_password"
    type="password"
    label="Current password"
    required
    :disabled="isLoading"
    @update:model-value="() => clearFieldError('old_password')"
/>

<ErrorAlert :message="errors.general" />
```

Per-field chyba → `Input.error`. Obecná (login failed, rate limit, …) → `ErrorAlert`.

### 5. Čištění při editaci

```typescript
const clearFieldError = (field: keyof ChangePasswordErrors): void => {
    // eslint-disable-next-line @typescript-eslint/no-dynamic-delete -- optional key removal is the intended API
    delete errors.value[field];
};
```

Uživatel opraví pole → chyba zmizí. `authFetch` na 401 automaticky refreshne token — o auth stav se formulář nestará.


## Proč ne validovat na frontendu

- **Single source of truth** je doména. Duplikát pravidel se dřív nebo později rozejde.
- **Bezpečnost.** Frontend validace je jen UX — útočník ji obejde. Backend validuje tak jako tak.
- **Konzistence hlášek.** Stejný handler volá HTTP, CLI i test. Všude stejná zpráva pro stejný případ.

**Nativní asistence prohlížeče** funguje jen přes atributy, které `<Input>` reálně propíše na DOM — dnes pouze `type` (takže `type="email"` dá nativní kontrolu formátu). Pozor: prop `required` na `<Input>` je **čistě vizuální** — vykreslí hvězdičku `*` u labelu, ale nepropisuje HTML atribut `required`, takže z něj žádný browser tooltip nepřijde; `minlength`/`pattern` `<Input>` zatím nepropisuje vůbec. Pravda tak jako tak přichází z backendu.


## Kam dál

| Téma | Odkaz |
|---|---|
| `ValidationError`, `AuthError`, `PermissionError` | [Errors & Events](/framework/domain/errors-events) |
| JSON response + `FieldError` interface | [HTTP Server](/framework/presentation/http-server) |
| `authFetch` + auto-refresh na 401 | [Frontend Utils](/guides/frontend-utils) |
| Login / refresh / logout tok | [Authentication](/guides/auth) |
| Permission check v handleru | [Permissions](/guides/permissions) |
