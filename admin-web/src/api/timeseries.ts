import { api } from "@/api/client";

export interface TimeseriesBucket {
  date: string; // YYYY-MM-DD (UTC)
  count: number;
}

export interface StatsTimeseries {
  signups: TimeseriesBucket[];
  connections: TimeseriesBucket[];
}

export async function getStatsTimeseries(
  days = 30,
): Promise<StatsTimeseries> {
  const resp = await api.get<StatsTimeseries>(
    "/api/v1/admin/stats/timeseries",
    { params: { days } },
  );
  return resp.data;
}
