import { Suspense, lazy } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity, CreditCard, Server, Users } from "lucide-react";

import { getAdminStats, type AdminStats } from "@/api/stats";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { formatNumber } from "@/lib/format";

// Recharts is heavy (~100 KB gz). Splitting StatsChart into its own
// chunk keeps the initial dashboard render fast — users see the KPI
// cards immediately and the chart fades in a moment later.
const StatsChart = lazy(() =>
  import("@/components/StatsChart").then((m) => ({ default: m.StatsChart })),
);

interface KpiDef {
  key: keyof AdminStats;
  label: string;
  Icon: React.ComponentType<{ className?: string }>;
}

// KPI cards on the dashboard map straight to the four fields in AdminStats
// for Phase B-1. Charts and per-day timeseries arrive in B-3 once the
// backend /admin/stats/timeseries endpoint lands.
const kpis: KpiDef[] = [
  { key: "total_users", label: "Total users", Icon: Users },
  {
    key: "active_subscriptions",
    label: "Active subscriptions",
    Icon: CreditCard,
  },
  { key: "active_server_count", label: "Active servers", Icon: Activity },
  { key: "server_count", label: "Total servers", Icon: Server },
];

export function Dashboard() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["admin", "stats"],
    queryFn: getAdminStats,
    refetchInterval: 60_000,
  });

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Dashboard</h1>
        <p className="text-sm text-muted-foreground">
          Live snapshot of users, subscriptions and VPN servers.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {kpis.map(({ key, label, Icon }) => (
          <Card key={key}>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {label}
              </CardTitle>
              <Icon className="size-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-semibold">
                {isLoading
                  ? "…"
                  : isError
                    ? "—"
                    : formatNumber(data ? Number(data[key]) : undefined)}
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      {isError && (
        <Card className="border-destructive/40 bg-destructive/10">
          <CardContent className="p-4 text-sm text-destructive">
            Failed to load stats: {(error as Error).message}
          </CardContent>
        </Card>
      )}

      <Suspense fallback={<Skeleton className="h-[340px] w-full" />}>
        <StatsChart />
      </Suspense>
    </div>
  );
}
