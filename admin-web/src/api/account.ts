import { api } from "@/api/client";

// POST /admin/change-password — rotates the authenticated admin's own
// password. The backend requires the current password so a stolen
// access token alone cannot rotate credentials.
export async function changeAdminPassword(
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  await api.post("/api/v1/admin/change-password", {
    current_password: currentPassword,
    new_password: newPassword,
  });
}
