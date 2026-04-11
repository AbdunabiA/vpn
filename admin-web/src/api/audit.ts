import { api } from "@/api/client";
import type { Pagination } from "@/api/users";

export interface AuditEntry {
  id: string;
  admin_id: string;
  action: string;
  target_id: string | null;
  details: Record<string, unknown> | null;
  ip: string;
  created_at: string;
}

export interface ListAuditResponse {
  entries: AuditEntry[];
  pagination: Pagination;
}

export async function listAuditEntries(
  page: number,
  limit = 50,
): Promise<ListAuditResponse> {
  const resp = await api.get<ListAuditResponse>("/api/v1/admin/audit-log", {
    params: { page, limit },
  });
  return resp.data;
}
