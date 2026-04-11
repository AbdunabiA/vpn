import { useQuery } from "@tanstack/react-query";
import { Radio } from "lucide-react";

import { listUserConnections, type ConnectionEntry } from "@/api/connections";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatDate, shortId } from "@/lib/format";
import { cn } from "@/lib/utils";

export function ConnectionsSection({ userID }: { userID: string }) {
  const { data, isLoading, isError, error } = useQuery({
    queryKey: ["admin", "user", userID, "connections"],
    queryFn: () => listUserConnections(userID, 50),
    enabled: !!userID,
  });

  const conns = data ?? [];

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <div className="flex items-center gap-2">
          <Radio className="size-4 text-muted-foreground" />
          <CardTitle className="text-base">История подключений</CardTitle>
        </div>
        <span className="text-xs text-muted-foreground">
          {isLoading ? "…" : `последних: ${conns.length}`}
        </span>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : isError ? (
          <div className="text-sm text-destructive">
            Не удалось загрузить историю: {(error as Error).message}
          </div>
        ) : conns.length === 0 ? (
          <div className="rounded-md border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
            История подключений пока пуста.
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[180px]">Подключился</TableHead>
                <TableHead className="w-[180px]">Отключился</TableHead>
                <TableHead className="w-[120px]">Статус</TableHead>
                <TableHead className="w-[120px]">Сервер</TableHead>
                <TableHead className="text-right">Трафик</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {conns.map((c) => (
                <ConnectionRow key={c.id} entry={c} />
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

function ConnectionRow({ entry }: { entry: ConnectionEntry }) {
  const active = entry.status === "connected" && !entry.disconnected_at;
  return (
    <TableRow>
      <TableCell className="text-xs text-muted-foreground">
        {formatDate(entry.connected_at)}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {entry.disconnected_at ? formatDate(entry.disconnected_at) : "—"}
      </TableCell>
      <TableCell>
        <span
          className={cn(
            "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium capitalize",
            active
              ? "bg-emerald-500/10 text-emerald-300 ring-1 ring-inset ring-emerald-500/30"
              : "bg-muted text-muted-foreground ring-1 ring-inset ring-border",
          )}
        >
          {entry.status}
        </span>
      </TableCell>
      <TableCell className="font-mono text-xs text-muted-foreground">
        {shortId(entry.server_id)}
      </TableCell>
      <TableCell className="text-right text-xs tabular-nums text-muted-foreground">
        ↑ {formatBytes(entry.bytes_up)}
        <span className="mx-2 text-border">·</span>↓{" "}
        {formatBytes(entry.bytes_down)}
      </TableCell>
    </TableRow>
  );
}

function formatBytes(n: number): string {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}
