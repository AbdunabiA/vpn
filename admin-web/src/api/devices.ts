import { api } from "@/api/client";

// Mirror of model.Device. Field names match the Go JSON tags exactly.
export interface AdminDevice {
  id: string;
  user_id: string;
  device_id: string;
  platform: string;
  model: string;
  first_seen_at: string;
  last_seen_at: string;
}

export async function listUserDevices(userID: string): Promise<AdminDevice[]> {
  const resp = await api.get<AdminDevice[]>(
    `/api/v1/admin/users/${userID}/devices`,
  );
  return resp.data;
}

// Delete a single device from any user's account (admin override).
// deviceRowID is the Device.id primary key, NOT the OS-issued device_id.
export async function deleteUserDevice(
  userID: string,
  deviceRowID: string,
): Promise<void> {
  await api.delete(
    `/api/v1/admin/users/${userID}/devices/${deviceRowID}`,
  );
}
