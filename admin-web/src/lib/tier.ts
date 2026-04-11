// Tier and role color/label helpers used by the users table and detail view.
// Kept as plain functions (not a lookup map) so that TypeScript's union
// narrowing catches unknown values at compile time.

import type { AdminUser } from "@/api/users";

export type Tier = AdminUser["subscription_tier"];
export type Role = AdminUser["role"];

export const TIER_OPTIONS: Tier[] = ["free", "premium", "ultimate"];

export function tierLabel(tier: Tier): string {
  switch (tier) {
    case "free":
      return "Free";
    case "premium":
      return "Premium";
    case "ultimate":
      return "Ultimate";
  }
}

// Returns Tailwind classes for a tier badge. Using ring-based borders
// instead of bg fills so tiers remain legible against both the card
// background and the table row hover state.
export function tierBadgeClass(tier: Tier): string {
  switch (tier) {
    case "free":
      return "bg-muted text-muted-foreground ring-1 ring-inset ring-border";
    case "premium":
      return "bg-sky-500/10 text-sky-300 ring-1 ring-inset ring-sky-500/30";
    case "ultimate":
      return "bg-amber-500/10 text-amber-300 ring-1 ring-inset ring-amber-500/30";
  }
}

export function roleBadgeClass(role: Role): string {
  switch (role) {
    case "admin":
      return "bg-rose-500/10 text-rose-300 ring-1 ring-inset ring-rose-500/30";
    case "user":
      return "bg-muted text-muted-foreground ring-1 ring-inset ring-border";
  }
}

// Device quota per tier — mirrors model.PlanLimits on the backend. Used
// only for display hints; the actual enforcement is server-side.
export function tierDeviceLimit(tier: Tier): number {
  switch (tier) {
    case "free":
      return 1;
    case "premium":
      return 3;
    case "ultimate":
      return 6;
  }
}
