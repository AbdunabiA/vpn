interface PlaceholderProps {
  title: string;
  subtitle: string;
}

// Tiny shared stub for the B-2/B-3/B-4 routes so the sidebar links already
// work during the Phase B-1 smoke test without spewing 404s.
export function Placeholder({ title, subtitle }: PlaceholderProps) {
  return (
    <div className="space-y-2">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="text-sm text-muted-foreground">{subtitle}</p>
    </div>
  );
}
