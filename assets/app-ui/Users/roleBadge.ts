// Role -> Tailwind badge classes, shared by the admin and platform user tables.
// The superadmin-aware variant is canonical (the platform plane lists superadmin
// rows). The param stays a bare `string` to match call sites — no Role enum
// exists yet (that is the separate F-095).
export const roleBadge = (role: string): string => {
    if (role === 'superadmin') {
        return 'bg-purple-100 text-purple-800';
    }
    if (role === 'admin') {
        return 'bg-orange-100 text-orange-800';
    }

    return 'bg-gray-100 text-gray-800';
};
