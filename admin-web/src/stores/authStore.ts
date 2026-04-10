import { create } from "zustand";
import { persist } from "zustand/middleware";

// Token bundle returned by /auth/admin-login and /auth/refresh. We persist
// it to localStorage so a page reload doesn't kick the admin to /login.
// Storing tokens in localStorage exposes them to XSS — mitigation: keep
// the admin origin tight (future CSP work) and honour the 5-minute access
// TTL so a stolen token has a short useful life.
export interface AuthTokens {
  accessToken: string;
  refreshToken: string;
  expiresIn: number; // seconds until access token expires
}

// Lightweight user payload; what admin-login returns under `user`. We only
// store what the UI actually displays to avoid drift with backend changes.
export interface AuthUser {
  id: string;
  full_name: string;
  role: string;
  subscription_tier: string;
}

interface AuthState {
  tokens: AuthTokens | null;
  user: AuthUser | null;
  setSession: (tokens: AuthTokens, user: AuthUser) => void;
  // updateTokens is used by the refresh interceptor — same tokens, same
  // user, new access/refresh pair.
  updateTokens: (tokens: AuthTokens) => void;
  clear: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      tokens: null,
      user: null,
      setSession: (tokens, user) => set({ tokens, user }),
      updateTokens: (tokens) => set((s) => ({ ...s, tokens })),
      clear: () => set({ tokens: null, user: null }),
    }),
    {
      name: "vpn-admin-auth",
      // Only persist tokens + user; everything else is ephemeral.
      partialize: (s) => ({ tokens: s.tokens, user: s.user }),
    },
  ),
);

// Non-reactive accessors for use outside React (e.g. axios interceptors).
// These read the same store instance the hooks use.
export const authSelectors = {
  getTokens: () => useAuthStore.getState().tokens,
  setTokens: (tokens: AuthTokens) => useAuthStore.getState().updateTokens(tokens),
  clear: () => useAuthStore.getState().clear(),
};
