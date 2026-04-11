// Formatting helpers used across tables and detail views. Kept
// framework-agnostic so they can be unit tested without any React harness.

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  // Explicit ru-RU locale so the admin panel always shows Russian
  // month abbreviations regardless of the browser's default. The admin
  // reads Russian and the whole UI is localised to ru, so there's no
  // reason to honour navigator language here.
  return d.toLocaleString("ru-RU", {
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
  // ru-RU uses a non-breaking space as the thousands separator, which
  // is the expected rendering in Russian-language interfaces.
  return new Intl.NumberFormat("ru-RU").format(n);
}
