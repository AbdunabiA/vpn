import axios, {
  AxiosError,
  type AxiosRequestConfig,
  type InternalAxiosRequestConfig,
} from "axios";

import { authSelectors, type AuthTokens } from "@/stores/authStore";

// The production build is served from https://vpnapi.mydayai.uz:9443/admin/
// and the API lives at /api/v1 on the same origin, so an empty baseURL with
// a leading-slash path works for both dev (via the Vite proxy) and prod.
// VITE_API_URL can override for local-backend iterations.
const baseURL = (import.meta.env.VITE_API_URL as string | undefined) ?? "";

export const api = axios.create({
  baseURL,
  headers: {
    "Content-Type": "application/json",
  },
  // 15 s covers a slow DB query comfortably; anything longer should fail
  // visibly rather than let the UI hang.
  timeout: 15_000,
});

// --- Request interceptor: attach Authorization header ---------------------
api.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const tokens = authSelectors.getTokens();
  if (tokens?.accessToken) {
    config.headers = config.headers ?? {};
    (config.headers as Record<string, string>).Authorization =
      `Bearer ${tokens.accessToken}`;
  }
  return config;
});

// --- Response interceptor: unwrap { data } + single-flight refresh -------
//
// The Go backend wraps every success envelope in { data: ... }. Rather than
// teach every TanStack Query hook to reach into response.data.data, we
// unwrap it here so callers can treat response.data as the payload.
//
// The refresh logic must be single-flight: with a 5-minute access TTL and
// multiple tabs/queries in flight, parallel 401s would each call /refresh,
// and since the backend deletes the old session row on use (see
// handler/auth.go:143) all but the first would fail and log the admin
// out. We funnel all 401s through a shared promise.

interface RefreshResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

let refreshInFlight: Promise<AuthTokens> | null = null;

async function refreshAccessToken(): Promise<AuthTokens> {
  if (refreshInFlight) return refreshInFlight;

  refreshInFlight = (async () => {
    const current = authSelectors.getTokens();
    if (!current?.refreshToken) {
      throw new Error("no refresh token");
    }
    // Use a bare axios instance so we don't recurse through the interceptor
    // stack (and so the Authorization header is NOT attached — refresh is
    // authenticated by the refresh_token in the body only).
    const resp = await axios.post<{ data: RefreshResponse }>(
      `${baseURL}/api/v1/auth/refresh`,
      { refresh_token: current.refreshToken },
      { timeout: 15_000 },
    );
    const body = resp.data.data;
    const next: AuthTokens = {
      accessToken: body.access_token,
      refreshToken: body.refresh_token,
      expiresIn: body.expires_in,
    };
    authSelectors.setTokens(next);
    return next;
  })();

  try {
    return await refreshInFlight;
  } finally {
    // Clear the promise whether it resolved or rejected — a new 401 after
    // this point should be treated as a fresh refresh attempt.
    refreshInFlight = null;
  }
}

api.interceptors.response.use(
  (response) => {
    // Unwrap { data: ... } envelopes. Endpoints that don't use the envelope
    // (there shouldn't be any, but defensively) pass through untouched.
    const body = response.data;
    if (body && typeof body === "object" && "data" in body && Object.keys(body).length === 1) {
      response.data = (body as { data: unknown }).data;
    }
    return response;
  },
  async (error: AxiosError) => {
    const original = error.config as
      | (AxiosRequestConfig & { _retried?: boolean })
      | undefined;

    // Only attempt a single retry; give up on recursive 401s.
    if (
      error.response?.status === 401 &&
      original &&
      !original._retried &&
      // Never try to refresh the refresh endpoint itself.
      !original.url?.includes("/auth/refresh") &&
      !original.url?.includes("/auth/admin-login")
    ) {
      original._retried = true;
      try {
        const next = await refreshAccessToken();
        original.headers = original.headers ?? {};
        (original.headers as Record<string, string>).Authorization =
          `Bearer ${next.accessToken}`;
        return api.request(original);
      } catch (refreshErr) {
        // Refresh failed — the admin needs to log in again. Clearing the
        // store will trip the auth guard in <AdminLayout> on the next render.
        authSelectors.clear();
        return Promise.reject(refreshErr);
      }
    }

    return Promise.reject(error);
  },
);
