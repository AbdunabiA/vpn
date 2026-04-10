import { api } from "@/api/client";
import type { AuthTokens, AuthUser } from "@/stores/authStore";

// POST /auth/admin-login returns only the token triple. The user's identity
// is carried in the access token's JWT claims (sub, role, tier, name), so
// we decode client-side rather than asking for a /me endpoint. Keeping
// token shapes in sync with handler/auth.go generateTokens() (~line 400).
interface AdminLoginTokens {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

interface JwtClaims {
  sub: string;
  role: string;
  tier: string;
  name?: string;
  exp: number;
}

// Minimal base64url JWT decoder. We trust the backend's signature — this
// is a read-only inspection for UI display, never a security check.
function decodeJwtPayload(token: string): JwtClaims {
  const parts = token.split(".");
  if (parts.length !== 3) {
    throw new Error("malformed JWT");
  }
  const payload = parts[1].replace(/-/g, "+").replace(/_/g, "/");
  const padded = payload + "=".repeat((4 - (payload.length % 4)) % 4);
  const json = atob(padded);
  return JSON.parse(json) as JwtClaims;
}

export async function adminLogin(
  email: string,
  password: string,
): Promise<{ tokens: AuthTokens; user: AuthUser }> {
  const resp = await api.post<AdminLoginTokens>("/api/v1/auth/admin-login", {
    email,
    password,
  });
  const tokens: AuthTokens = {
    accessToken: resp.data.access_token,
    refreshToken: resp.data.refresh_token,
    expiresIn: resp.data.expires_in,
  };
  const claims = decodeJwtPayload(tokens.accessToken);
  const user: AuthUser = {
    id: claims.sub,
    full_name: claims.name ?? "Admin",
    role: claims.role,
    subscription_tier: claims.tier,
  };
  return { tokens, user };
}
