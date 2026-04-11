import { api } from "@/api/client";

// Mirror of model.User from server/api/internal/model/user.go. Keep the
// field names in exact lockstep with the Go JSON tags — any drift here
// means the UI silently shows blanks.
export interface AdminUser {
  id: string;
  full_name: string;
  subscription_tier: "free" | "premium" | "ultimate";
  subscription_expires_at: string | null;
  role: "user" | "admin";
  created_at: string;
}

export interface Pagination {
  page: number;
  limit: number;
  total: number;
  total_pages: number;
}

// GET /admin/users response payload after the global axios interceptor
// unwraps one level of `data`.
export interface ListUsersResponse {
  users: AdminUser[];
  pagination: Pagination;
}

export interface ListUsersParams {
  page: number;
  limit: number;
  search?: string;
}

export async function listUsers(
  params: ListUsersParams,
): Promise<ListUsersResponse> {
  const resp = await api.get<ListUsersResponse>("/api/v1/admin/users", {
    params: {
      page: params.page,
      limit: params.limit,
      // Skip the search param entirely when empty so the backend query
      // planner hits the fast path (no LIKEs).
      ...(params.search ? { search: params.search } : {}),
    },
  });
  return resp.data;
}

export async function getUser(id: string): Promise<AdminUser> {
  const resp = await api.get<AdminUser>(`/api/v1/admin/users/${id}`);
  return resp.data;
}

// adminUpdateUserRequest shape from admin.go:106. Only send fields that
// the caller actually wants to change; zero-valued fields are left
// out rather than sent as empty strings/zeros.
export interface UpdateUserInput {
  subscription_tier?: "free" | "premium" | "ultimate";
  role?: "user" | "admin";
  // RFC3339 timestamp, empty string clears the expiration, undefined
  // leaves the field alone. Matches the backend's *string pointer.
  subscription_expires_at?: string;
  // Positive integer; adds this many days to the current expiration
  // (or to now if none is set). Caller is responsible for sanity caps.
  extend_days?: number;
}

export interface UpdateUserResponse {
  id: string;
  updated: Record<string, unknown>;
}

export async function updateUser(
  id: string,
  input: UpdateUserInput,
): Promise<UpdateUserResponse> {
  const resp = await api.patch<UpdateUserResponse>(
    `/api/v1/admin/users/${id}`,
    input,
  );
  return resp.data;
}
