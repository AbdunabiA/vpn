import { api } from "@/api/client";

export interface ConnectionEntry {
  id: string;
  user_id: string;
  server_id: string;
  connected_at: string;
  disconnected_at: string | null;
  bytes_up: number;
  bytes_down: number;
  status: string;
  last_heartbeat_at: string | null;
}

export async function listUserConnections(
  userID: string,
  limit = 50,
): Promise<ConnectionEntry[]> {
  const resp = await api.get<ConnectionEntry[]>(
    `/api/v1/admin/users/${userID}/connections`,
    { params: { limit } },
  );
  return resp.data;
}
