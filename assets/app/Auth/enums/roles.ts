// Mirrors the backend user.Role value object (app/domain/user/role.go): the three
// canonical role identifiers. Backend is authoritative — same discipline as the
// Permission enum. AuthUser.role / UserFormData.role are codegen `string` for now
// (widening those to Role is the BE<->FE codegen follow-up), so this enum is used
// for role LITERALS — comparisons and option values — instead of magic strings.

export const Role = {
    SuperAdmin: 'superadmin',
    Admin: 'admin',
    User: 'user',
} as const;

export type Role = typeof Role[keyof typeof Role];
