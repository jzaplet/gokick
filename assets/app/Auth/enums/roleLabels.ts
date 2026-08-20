import { Role } from '@/app/Auth/enums/roles';
import { t } from '@/app-ui/I18n';
import type { TranslationKey } from '@/app-ui/I18n';

// Display names for the wire Role values: raw enum values must
// never render as UI text. The map is hand-written next to the hand-written
// Permission enum — the generated roles.ts must stay codegen-only.
const roleLabelKeys: Record<Role, TranslationKey> = {
    [Role.SuperAdmin]: 'role.superadmin',
    [Role.Admin]: 'role.admin',
    [Role.User]: 'role.user',
};

export const roleLabel = (role: Role): string => t(roleLabelKeys[role]);
