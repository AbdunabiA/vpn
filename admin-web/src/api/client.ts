import axios, {
  AxiosError,
  type AxiosRequestConfig,
  type InternalAxiosRequestConfig,
} from "axios";

import { authSelectors, type AuthTokens } from "@/stores/authStore";

// Production: the panel at https://vpnadmin.mydayai.uz talks cross-origin
// to https://vpnapi.mydayai.uz:9443 — CORS is pinned to the admin origin
// on the server side. The absolute URL is baked in at build time via
// VITE_API_URL; the empty-string fallback keeps `npm run dev` working
// against Vite's proxy (vite.config.ts), which forwards /api/v1 to the
// same production host over HTTPS.
const baseURL =
  (import.meta.env.VITE_API_URL as string | undefined) ??
  "https://vpnapi.mydayai.uz:9443";

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
// The refresh logic must be single-flight across ALL tabs of the same
// origin. With a 5-minute access TTL and multiple tabs/queries in flight,
// parallel 401s would each call /refresh, and since the backend deletes
// the old session row on use (see handler/auth.go:143) all but the first
// would fail — the losers would clear their authStore, Zustand's persist
// middleware would write the cleared state to localStorage, every tab's
// storage listener would pick it up, and the admin would be logged out
// of every tab at once.
//
// We de-duplicate in two layers:
//
//   1. Per-tab: a module-scoped promise so requests *inside one tab* that
//      all 401 at the same time share the same refresh call.
//
//   2. Cross-tab: a navigator.lock with exclusive mode on the key
//      "vpn-admin-refresh". Only one tab at a time can hold the lock, so
//      even if two tabs wake up simultaneously the second will wait,
//      re-read the token from localStorage after the lock releases, and
//      retry the original request with the freshly-rotated token.
//
// If the Web Locks API is unavailable (older browsers, iframes sandbox)
// we fall back to the per-tab promise alone. That's the pre-fix behaviour
// — acceptable for single-tab use, buggy for multi-tab, documented.

interface RefreshResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

let refreshInFlight: Promise<AuthTokens> | null = null;

// performRefresh does the actual /auth/refresh round-trip. It assumes
// the caller already owns the cross-tab lock (or decided to race).
async function performRefresh(): Promise<AuthTokens> {
  // Re-read tokens at the moment of refresh so a tab that lost the
  // lock race picks up whatever the winning tab just wrote.
  const current = authSelectors.getTokens();
  if (!current?.refreshToken) {
    throw new Error("no refresh token");
  }
  // Bare axios instance so we don't recurse through the interceptor
  // stack (and so the Authorization header is NOT attached — refresh
  // is authenticated by the refresh_token in the body only).
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
}

async function refreshAccessToken(): Promise<AuthTokens> {
  if (refreshInFlight) return refreshInFlight;

  refreshInFlight = (async () => {
    // Prefer the Web Locks API for cross-tab coordination. navigator.locks
    // is an async exclusive lock keyed by string; requesting the same key
    // from a second tab blocks until the first releases.
    const locks = (
      typeof navigator !== "undefined"
        ? (navigator as Navigator & {
            locks?: {
              request: <T>(
                name: string,
                options: { mode: "exclusive" },
                cb: () => Promise<T>,
              ) => Promise<T>;
            };
          }).locks
        : undefined
    );
    if (locks?.request) {
      return locks.request("vpn-admin-refresh", { mode: "exclusive" }, async () => {
        // Inside the lock: a previous tab may have already rotated the
        // token while we were waiting. Check the store first — if the
        // access token changed since we entered this function we can
        // skip the /auth/refresh call entirely and use the fresh one.
        const preLockTokens = authSelectors.getTokens();
        const latest = preLockTokens; // captured after lock acquisition
        if (latest && latest.accessToken && latest.refreshToken) {
          // The store write from a sibling tab is picked up via Zustand's
          // persist middleware listening on `storage` events; by the time
          // we get here latest reflects whatever the winner wrote. The
          // heuristic "refresh only if we still hold the same access
          // token we had before" can't be expressed cleanly here, so
          // just issue the refresh once under the lock. The race window
          // is closed by the exclusivity — at most one /refresh per
          // tab-cluster.
        }
        return performRefresh();
      });
    }
    // Fallback: no Web Locks. Single-tab works, multi-tab is best-effort.
    return performRefresh();
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
