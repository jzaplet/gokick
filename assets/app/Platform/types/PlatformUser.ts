// Mirrors the backend platformUserDTO (GET /api/v1/platform/users). last_login_at
// is null until the user has logged in at least once.
export type PlatformUser = {
    id: string;
    nickname: string;
    email: string;
    role: string;
    active: boolean;
    tenant_id: string;
    tenant_name: string;
    last_login_at: string | null;
};
