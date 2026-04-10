// Formatting helpers used across tables and detail views. Kept
// framework-agnostic so they can be unit tested without any React harness.

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

// Truncates a UUID to the first 8 chars — matches what admins paste in from
// Telegram after a user requests premium activation.
export function shortId(id: string): string {
  return id.slice(0, 8);
}

export function formatNumber(n: number | null | undefined): string {
  if (n == null) return "—";
  return new Intl.NumberFormat().format(n);
}
