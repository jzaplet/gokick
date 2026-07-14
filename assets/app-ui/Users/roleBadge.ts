import { Role } from '@/app/Auth/enums/roles';

// Role -> Tailwind badge classes, shared by the admin and platform user tables.
// The superadmin-aware variant is canonical (the platform plane lists superadmin
// rows). The param stays a bare `string` — it's fed codegen `user.role` (string)
// — but the literals compared against are the Role enum, not magic strings.
export const roleBadge = (role: string): string => {
    if (role === Role.SuperAdmin) {
        return 'bg-purple-100 text-purple-800';
    }
    if (role === Role.Admin) {
        return 'bg-orange-100 text-orange-800';
    }

    return 'bg-gray-100 text-gray-800';
};
