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

// formatBytes renders a byte count as a compact human-readable string
// (KB/MB/GB/TB). Used on the traffic chart tooltip and the dashboard
// traffic KPIs. Uses 1024 rather than 1000 — standard for network
// bandwidth reporting.
export function formatBytes(n: number | null | undefined): string {
  if (n == null || !Number.isFinite(n)) return "—";
  if (n === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let i = 0;
  let v = Math.abs(n);
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  // One decimal place for KB and above, integer for bytes.
  const display = i === 0 ? v.toFixed(0) : v.toFixed(1);
  return `${display} ${units[i]}`;
}
