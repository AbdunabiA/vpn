import { api } from "@/api/client";

// Shape returned by GET /api/v1/admin/stats. Mirrors
// repository.GetGlobalStats in admin_repo.go — keep this in lockstep with
// the backend map keys. Fields are int64 on the backend which JSON-encodes
// as plain numbers (safe for values well below 2^53).
export interface AdminStats {
  total_users: number;
  active_subscriptions: number;
  server_count: number;
  active_server_count: number;
}

export async function getAdminStats(): Promise<AdminStats> {
  const resp = await api.get<AdminStats>("/api/v1/admin/stats");
  return resp.data;
}
