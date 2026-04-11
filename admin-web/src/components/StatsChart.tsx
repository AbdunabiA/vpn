import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

import { getStatsTimeseries } from "@/api/timeseries";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

// StatsChart renders a single area chart that overlays two series — new
// users per day and new connections per day — over the last 30 days.
// Kept to one chart (not two side-by-side) so a quick glance correlates
// signups with activity without the admin having to scan twice.
export function StatsChart() {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["admin", "stats", "timeseries", 30],
    queryFn: () => getStatsTimeseries(30),
    refetchInterval: 5 * 60_000,
  });

  // Zip the two series together so recharts can read from a single array.
  // The backend already guarantees both arrays are the same length and
  // date-aligned (see repository.GetTimeseries), so index-based merge is
  // safe and avoids a map lookup per point.
  const chartData = useMemo(() => {
    if (!data) return [];
    return data.signups.map((row, i) => ({
      date: row.date,
      signups: row.count,
      connections: data.connections[i]?.count ?? 0,
    }));
  }, [data]);

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Последние 30 дней</CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <Skeleton className="h-[260px] w-full" />
        ) : isError ? (
          <div className="flex h-[260px] items-center justify-center text-sm text-destructive">
            Не удалось загрузить график: {(error as Error).message}
          </div>
        ) : (
          <div className="h-[260px] w-full">
            <ResponsiveContainer>
              <AreaChart
                data={chartData}
                margin={{ top: 10, right: 12, left: 0, bottom: 0 }}
              >
                <defs>
                  <linearGradient id="sig" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#38bdf8" stopOpacity={0.5} />
                    <stop offset="95%" stopColor="#38bdf8" stopOpacity={0} />
                  </linearGradient>
                  <linearGradient id="con" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#a78bfa" stopOpacity={0.5} />
                    <stop offset="95%" stopColor="#a78bfa" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid
                  strokeDasharray="3 3"
                  stroke="currentColor"
                  className="text-border/40"
                />
                <XAxis
                  dataKey="date"
                  tickFormatter={(v: string) => v.slice(5)}
                  stroke="currentColor"
                  className="text-xs text-muted-foreground"
                />
                <YAxis
                  allowDecimals={false}
                  stroke="currentColor"
                  className="text-xs text-muted-foreground"
                  width={32}
                />
                <Tooltip
                  contentStyle={{
                    background: "hsl(240 6% 10%)",
                    border: "1px solid hsl(240 4% 22%)",
                    borderRadius: 8,
                    fontSize: 12,
                  }}
                  labelStyle={{ color: "#cbd5e1" }}
                />
                <Area
                  type="monotone"
                  dataKey="signups"
                  name="Новые пользователи"
                  stroke="#38bdf8"
                  fill="url(#sig)"
                  strokeWidth={2}
                />
                <Area
                  type="monotone"
                  dataKey="connections"
                  name="Подключения"
                  stroke="#a78bfa"
                  fill="url(#con)"
                  strokeWidth={2}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        )}
        <div className="mt-3 flex items-center gap-4 text-xs text-muted-foreground">
          <LegendDot color="#38bdf8" label="Новые пользователи" />
          <LegendDot color="#a78bfa" label="Подключения" />
        </div>
      </CardContent>
    </Card>
  );
}

function LegendDot({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-1.5">
      <span
        className="inline-block size-2 rounded-full"
        style={{ background: color }}
      />
      {label}
    </div>
  );
}
