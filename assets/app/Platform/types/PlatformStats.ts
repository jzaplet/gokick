// Mirrors the backend platformStatsDTO (GET /api/v1/platform/stats).
export type PlatformStats = {
    tenant_count: number;
    user_count: number;
};
