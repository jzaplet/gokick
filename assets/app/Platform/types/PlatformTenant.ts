// Mirrors the backend platformTenantDTO (GET /api/v1/platform/tenants). Keys are
// snake_case to match the API convention (cf. access_token elsewhere).
export type PlatformTenant = {
    id: string;
    name: string;
    plan: string;
    user_count: number;
};
