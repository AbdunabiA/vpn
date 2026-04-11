import { api } from "@/api/client";

// Mirror of model.VPNServer's JSON-visible fields only. Several columns
// (capacity, reality_public_key, reality_short_id, WS/AWG params) are
// marshalled as json:"-" in Go so they never reach the panel, even on
// admin reads. The PATCH endpoint still accepts those fields though —
// see UpdateServerInput below.
export interface AdminServer {
  id: string;
  hostname: string;
  ip_address: string;
  region: string;
  city: string;
  country: string;
  country_code: string;
  protocol: string;
  // load_percent is what the GORM struct serialises for "current_load".
  load_percent: number;
  is_active: boolean;
  created_at: string;
}

export async function listServers(): Promise<AdminServer[]> {
  const resp = await api.get<AdminServer[]>("/api/v1/admin/servers");
  return resp.data;
}

// Fields the backend PATCH handler will actually accept. Omit any
// field you don't want to change; empty strings are ignored server-
// side for the string fields (see handler/admin.go:346-367).
export interface UpdateServerInput {
  ip_address?: string;
  protocol?: string;
  capacity?: number;
  is_active?: boolean;
  reality_public_key?: string;
  reality_short_id?: string;
  current_load?: number;
}

export async function updateServer(
  id: string,
  input: UpdateServerInput,
): Promise<{ id: string; updated: Record<string, unknown> }> {
  const resp = await api.patch<{ id: string; updated: Record<string, unknown> }>(
    `/api/v1/admin/servers/${id}`,
    input,
  );
  return resp.data;
}

// Soft delete — backend sets is_active=false. The row is not physically
// removed, so a subsequent toggle back to active via updateServer works.
export async function deleteServer(id: string): Promise<void> {
  await api.delete(`/api/v1/admin/servers/${id}`);
}
